package main

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMerchantTemporaryAccessTokenPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	ctx := t.Context()
	admin := "merchant-temporary-token-test-admin"
	mux := http.NewServeMux()
	registerMerchantDemo(mux, repo, admin)

	// This merchant was provisioned with the original hash-only credentials.
	original, originalHash, err := newMerchantToken("merchant-demo-")
	if err != nil {
		t.Fatal(err)
	}
	var merchantID string
	if err := db.QueryRow(ctx, `INSERT INTO merchant_demo_merchants (name, token_hash)
		VALUES ('Legacy Sandbox Merchant', $1) RETURNING id::text`, originalHash[:]).Scan(&merchantID); err != nil {
		t.Fatal(err)
	}
	other, otherToken, err := repo.createMockMerchant(ctx, mockMerchantInput{Name: "Other Sandbox Merchant"})
	if err != nil {
		t.Fatal(err)
	}
	merchantPath := merchantDemoPath + "/" + merchantID
	otherPath := merchantDemoPath + "/" + other.ID
	issuePath := merchantPath + "/access-token"
	request := func(method, path, token string, wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != wantStatus {
			t.Fatalf("%s %s returned HTTP %d, want %d", method, path, response.Code, wantStatus)
		}
		return response
	}

	request(http.MethodPost, issuePath, "", http.StatusUnauthorized)
	request(http.MethodPost, issuePath, original, http.StatusForbidden)
	request(http.MethodPost, issuePath, otherToken, http.StatusForbidden)
	request(http.MethodGet, issuePath, admin, http.StatusMethodNotAllowed)
	request(http.MethodPost, merchantDemoPath+"/invalid/access-token", admin, http.StatusNotFound)

	var before time.Time
	if err := db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	issue := func() merchantAccessToken {
		t.Helper()
		issued := request(http.MethodPost, issuePath, admin, http.StatusCreated)
		if issued.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("The temporary token response must not be cached")
		}
		var access merchantAccessToken
		if err := json.Unmarshal(issued.Body.Bytes(), &access); err != nil {
			t.Fatal("Invalid temporary token response")
		}
		if !bookingTokenPattern.MatchString(access.Token) || !strings.HasPrefix(access.Token, merchantAccessTokenPrefix) {
			t.Fatal("Temporary token cannot be used as a bearer credential")
		}
		return access
	}
	first, second := issue(), issue()
	if first.Token == second.Token || first.Token == original || second.Token == original {
		t.Fatal("Issuing a temporary token must create a distinct credential")
	}
	var after time.Time
	if err := db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if first.ExpiresAt.Before(before.Add(time.Hour)) || first.ExpiresAt.After(after.Add(time.Hour)) {
		t.Fatal("Temporary token expiry must be one hour after issuance")
	}

	for _, token := range []string{original, first.Token, second.Token} {
		var directory struct {
			Merchants []mockMerchant `json:"merchants"`
			Role      string         `json:"role"`
		}
		response := request(http.MethodGet, merchantDemoPath, token, http.StatusOK)
		if err := json.Unmarshal(response.Body.Bytes(), &directory); err != nil ||
			directory.Role != "merchant" || len(directory.Merchants) != 1 || directory.Merchants[0].ID != merchantID {
			t.Fatal("Credential must grant access only to the intended merchant")
		}
		request(http.MethodGet, merchantPath+"/transactions", token, http.StatusOK)
		request(http.MethodGet, merchantPolicyPath+"/profile", token, http.StatusOK)
		request(http.MethodGet, otherPath+"/transactions", token, http.StatusNotFound)
		request(http.MethodPost, issuePath, token, http.StatusForbidden)
	}

	for _, path := range []string{merchantDemoPath, merchantPath, merchantPath + "/audit"} {
		response := request(http.MethodGet, path, admin, http.StatusOK)
		for _, token := range []string{original, first.Token, second.Token} {
			if strings.Contains(response.Body.String(), token) {
				t.Fatalf("GET %s exposes a credential", path)
			}
		}
	}
	var issuedCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM merchant_demo_audit
		WHERE merchant_id=$1::text::uuid AND action='MERCHANT_TEMPORARY_CREDENTIAL_ISSUED' AND actor='platform-admin'`, merchantID).
		Scan(&issuedCount); err != nil || issuedCount != 2 {
		t.Fatalf("Expected two audited temporary token issuances, got %d (err=%v)", issuedCount, err)
	}

	firstHash := sha256.Sum256([]byte(first.Token))
	if _, err := db.Exec(ctx, `UPDATE merchant_demo_access_tokens SET expires_at=clock_timestamp() WHERE token_hash=$1`, firstHash[:]); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, merchantDemoPath, first.Token, http.StatusUnauthorized)
	request(http.MethodGet, merchantDemoPath, second.Token, http.StatusOK)
	third := issue()
	var expiredRemains bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM merchant_demo_access_tokens WHERE token_hash=$1)`, firstHash[:]).Scan(&expiredRemains); err != nil || expiredRemains {
		t.Fatalf("Issuing a credential must remove expired hashes (err=%v)", err)
	}

	request(http.MethodDelete, merchantPath, admin, http.StatusOK)
	for _, token := range []string{original, second.Token, third.Token} {
		request(http.MethodGet, merchantDemoPath, token, http.StatusUnauthorized)
	}
	request(http.MethodPost, issuePath, admin, http.StatusNotFound)
	request(http.MethodGet, merchantDemoPath, otherToken, http.StatusOK)
}
