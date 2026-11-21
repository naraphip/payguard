package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMerchantManagementPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	mux := http.NewServeMux()
	const admin = "merchant-management-test-admin"
	registerMerchantDemo(mux, repository{db}, admin)
	credentials := []string{admin}
	request := func(t *testing.T, method, path, token string, input any, status int, output any, idempotencyKey ...string) []byte {
		t.Helper()
		var body bytes.Buffer
		if input != nil {
			if err := json.NewEncoder(&body).Encode(input); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequestWithContext(t.Context(), method, path, &body)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if len(idempotencyKey) > 0 {
			req.Header.Set("Idempotency-Key", idempotencyKey[0])
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s %s: got HTTP %d, want %d: %s", method, path, response.Code, status, response.Body.String())
		}
		for _, credential := range credentials {
			if bytes.Contains(response.Body.Bytes(), []byte(credential)) {
				t.Fatalf("%s %s exposed an existing credential", method, path)
			}
		}
		if output != nil {
			if err := json.Unmarshal(response.Body.Bytes(), output); err != nil {
				t.Fatal(err)
			}
		}
		return response.Body.Bytes()
	}

	prefix, err := attemptID()
	if err != nil {
		t.Fatal(err)
	}
	createMerchant := func(suffix, description string) (mockMerchant, string) {
		t.Helper()
		var created struct {
			Merchant mockMerchant `json:"merchant"`
			Token    string       `json:"token"`
		}
		request(t, http.MethodPost, merchantDemoPath, admin, mockMerchantInput{
			Name: prefix + suffix, Description: &description,
			Setup:             &merchantSetup{SchemaVersion: 1, BusinessModel: "ota", Stage: "booking", AvailableSignals: []string{bookingTimestamp, bookingAmount}},
			PolicyIntegration: &merchantPolicyIntegration{Products: []string{"flight_ticket"}, EmailDomain: true},
		}, http.StatusCreated, &created)
		if created.Merchant.CreatedAt == nil || created.Merchant.Description != description || created.Token == "" {
			t.Fatal("new merchant lacks its creation time, description, or issued credential")
		}
		credentials = append(credentials, created.Token)
		return created.Merchant, created.Token
	}
	first, tokenA := createMerchant(" Alphabetic first", "Initial description")
	second, tokenB := createMerchant(" Z after first", "")
	firstPath, secondPath := merchantDemoPath+"/"+first.ID, merchantDemoPath+"/"+second.ID

	description := "Updated by the platform administrator"
	var update struct {
		Merchant mockMerchant `json:"merchant"`
	}
	request(t, http.MethodPut, firstPath, tokenA, mockMerchantInput{Name: first.Name, Description: &description}, http.StatusForbidden, nil)
	request(t, http.MethodPut, firstPath, admin, mockMerchantInput{Name: first.Name, Description: &description}, http.StatusOK, &update)
	request(t, http.MethodPut, firstPath, admin, mockMerchantInput{Name: first.Name}, http.StatusOK, &update)
	if update.Merchant.Description != description || !update.Merchant.CreatedAt.Equal(*first.CreatedAt) {
		t.Fatalf("legacy update lost existing metadata: %+v", update.Merchant)
	}

	createScenario := func(path string) mockScenario {
		t.Helper()
		var created struct {
			Scenario mockScenario `json:"scenario"`
		}
		request(t, http.MethodPost, path+"/scenarios", admin,
			map[string]any{"name": "Managed scenario", "thresholdMinor": 5000}, http.StatusCreated, &created)
		if created.Scenario.ID == "" {
			t.Fatal("scenario ID missing")
		}
		return created.Scenario
	}
	firstScenario, secondScenario := createScenario(firstPath), createScenario(secondPath)

	policy := exampleMerchantPolicy()
	var savedPolicy struct {
		Rule merchantPolicyRule `json:"rule"`
	}
	policyPath := merchantPolicyPath + "/rules"
	request(t, http.MethodPost, policyPath, tokenA, map[string]any{"proposal": policy}, http.StatusCreated, &savedPolicy)
	if savedPolicy.Rule.Status != "ACTIVE" {
		t.Fatalf("expected an active merchant policy, got %s", savedPolicy.Rule.Status)
	}
	request(t, http.MethodPost, policyPath, admin, map[string]any{"proposal": policy}, http.StatusForbidden, nil)
	var adminSaved struct {
		Rule merchantPolicyRule `json:"rule"`
	}
	request(t, http.MethodPost, firstPath+"/policies", admin, map[string]any{"proposal": policy}, http.StatusCreated, &adminSaved)
	if adminSaved.Rule.ID != savedPolicy.Rule.ID || adminSaved.Rule.Version != savedPolicy.Rule.Version {
		t.Fatalf("Platform Admin resaving the same policy should reuse the existing revision: %+v", adminSaved.Rule)
	}

	createTransaction := func(path string, scenario mockScenario, id string, amount int64, domain string) mockTransaction {
		t.Helper()
		var created struct {
			Transaction mockTransaction `json:"transaction"`
		}
		request(t, http.MethodPost, path+"/transactions", admin, mockTransactionInput{
			ID: id, ScenarioID: scenario.ID, Source: "checkout", Product: "flight_ticket", AmountMinor: &amount,
			CustomerEmailDomain: domain,
		}, http.StatusCreated, &created)
		return created.Transaction
	}
	const (
		allowedID  = "bdf1a974-f554-43b1-8f0d-6bc699354ec0"
		oldID      = "bdf1a974-f554-43b1-8f0d-6bc699354ec1"
		openID     = "bdf1a974-f554-43b1-8f0d-6bc699354ec2"
		resolvedID = "bdf1a974-f554-43b1-8f0d-6bc699354ec3"
		blockedID  = "bdf1a974-f554-43b1-8f0d-6bc699354ec4"
	)
	createTransaction(secondPath, secondScenario, allowedID, 50, "example.test")
	createTransaction(firstPath, firstScenario, oldID, 99, "example.test")
	if _, err := db.Exec(t.Context(), `UPDATE merchant_demo_transactions SET created_at=now()-interval '35 days'
		WHERE merchant_id=$1::text::uuid AND id=$2::text::uuid`, first.ID, oldID); err != nil {
		t.Fatal(err)
	}
	createTransaction(firstPath, firstScenario, allowedID, 50, "example.test")
	open := createTransaction(firstPath, firstScenario, openID, 7500, "example.test")
	createTransaction(firstPath, firstScenario, resolvedID, 7600, "example.test")
	blocked := createTransaction(firstPath, firstScenario, blockedID, 75, "skycannner.com")
	if open.ReviewState != "NEEDS_DECISION" || open.Status != "REVIEW" || blocked.Status != "REJECTED" ||
		blocked.PolicyDecision == nil || blocked.PolicyDecision.Final.Action != "BLOCK" {
		t.Fatalf("fixture decisions: open=%s blocked=%+v", open.ReviewState, blocked.PolicyDecision)
	}
	request(t, http.MethodPost, firstPath+"/transactions/"+blockedID+"/decision", admin, map[string]string{"decision": "cleared"}, http.StatusConflict, nil)
	request(t, http.MethodPost, firstPath+"/transactions/"+allowedID+"/decision", admin, map[string]string{"decision": "rejected"}, http.StatusConflict, nil)
	request(t, http.MethodPost, firstPath+"/transactions/"+resolvedID+"/decision", admin, map[string]string{"decision": "cleared"}, http.StatusOK, nil)

	t.Run("legacy directory and paginated search", func(t *testing.T) {
		var legacy struct {
			Role      string         `json:"role"`
			Merchants []mockMerchant `json:"merchants"`
		}
		raw := request(t, http.MethodGet, merchantDemoPath, admin, nil, http.StatusOK, &legacy)
		var legacyKeys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &legacyKeys); err != nil {
			t.Fatal(err)
		}
		index := slices.IndexFunc(legacy.Merchants, func(item mockMerchant) bool { return item.ID == first.ID })
		if legacy.Role != "admin" || legacyKeys["page"] != nil || legacyKeys["total"] != nil || index < 0 ||
			len(legacy.Merchants[index].Scenarios) != 1 || legacy.Merchants[index].Scenarios[0].ID != firstScenario.ID {
			t.Fatal("unpaged directory lost its scenario-bearing response")
		}
		request(t, http.MethodGet, merchantDemoPath, tokenB, nil, http.StatusOK, &legacy)
		if legacy.Role != "merchant" || len(legacy.Merchants) != 1 || legacy.Merchants[0].ID != second.ID {
			t.Fatal("merchant directory exposed another tenant")
		}

		type list struct {
			merchantPage
			Role      string                  `json:"role"`
			Merchants []merchantManagementRow `json:"merchants"`
		}
		search := "?q=" + url.QueryEscape(strings.ToUpper(prefix))
		var results list
		request(t, http.MethodGet, merchantDemoPath+search+"&page=1&limit=1&sort=name", admin, nil, http.StatusOK, &results)
		if results.Role != "admin" || results.Page != 1 || results.Limit != 1 || results.Total != 2 || len(results.Merchants) != 1 {
			t.Fatalf("unexpected pagination: %+v", results)
		}
		row := results.Merchants[0]
		if row.ID != first.ID || row.Description != description || row.TransactionCount != 5 || row.UnderReview != 1 ||
			row.RuleCount != 1 || row.LastActivity == nil || !row.LastActivity.Equal(blocked.CreatedAt) {
			t.Fatalf("unexpected merchant metrics: %+v", row)
		}
		request(t, http.MethodGet, merchantDemoPath+search+"&page=2&limit=1&sort=name", admin, nil, http.StatusOK, &results)
		if results.Total != 2 || results.Page != 2 || len(results.Merchants) != 1 || results.Merchants[0].ID != second.ID {
			t.Fatalf("incorrect second page: %+v", results)
		}
		for order, want := range map[string]string{"created": second.ID, "activity": first.ID} {
			request(t, http.MethodGet, merchantDemoPath+search+"&sort="+order+"&limit=1", admin, nil, http.StatusOK, &results)
			if len(results.Merchants) != 1 || results.Merchants[0].ID != want {
				t.Fatalf("unexpected %s sort: %+v", order, results)
			}
		}
		request(t, http.MethodGet, merchantDemoPath+"?q="+first.ID, admin, nil, http.StatusOK, &results)
		if results.Total != 1 || len(results.Merchants) != 1 || results.Merchants[0].ID != first.ID {
			t.Fatalf("merchant ID search failed: %+v", results)
		}
		request(t, http.MethodGet, merchantDemoPath+"?limit=101", admin, nil, http.StatusBadRequest, nil)
		request(t, http.MethodGet, merchantDemoPath+"?sort=unknown", admin, nil, http.StatusBadRequest, nil)
	})

	t.Run("stable merchant details and policy isolation", func(t *testing.T) {
		type detail struct {
			Merchant           mockMerchant          `json:"merchant"`
			Profile            merchantPolicyProfile `json:"profile"`
			Metrics            merchantMetrics       `json:"metrics"`
			RecentTransactions []mockTransaction     `json:"recentTransactions"`
		}
		var details, own detail
		request(t, http.MethodGet, firstPath, admin, nil, http.StatusOK, &details)
		request(t, http.MethodGet, firstPath, tokenA, nil, http.StatusOK, &own)
		if !reflect.DeepEqual(details, own) || details.Merchant.ID != first.ID || details.Merchant.Description != description ||
			details.Profile.BusinessName != first.Name || len(details.RecentTransactions) != 5 ||
			details.Metrics != (merchantMetrics{WindowDays: 30, Total: 4, Allowed: 1, UnderReview: 1, Blocked: 1, Approved: 2, Rejected: 1, Review: 1, ActivePolicies: 1}) {
			t.Fatalf("unexpected stable merchant detail: %+v", details)
		}
		for _, suffix := range []string{"", "/policies", "/policies/" + savedPolicy.Rule.ID, "/audit", "/transactions?limit=1", "/transactions/" + allowedID} {
			request(t, http.MethodGet, firstPath+suffix, tokenB, nil, http.StatusNotFound, nil)
		}
		request(t, http.MethodGet, firstPath, "", nil, http.StatusUnauthorized, nil)
		var policies struct {
			Rules []merchantPolicyRule `json:"rules"`
		}
		request(t, http.MethodGet, firstPath+"/policies", admin, nil, http.StatusOK, &policies)
		if len(policies.Rules) != 1 || policies.Rules[0].ID != savedPolicy.Rule.ID || policies.Rules[0].ActiveVersion != 1 {
			t.Fatalf("admin policies read differs from tenant rule: %+v", policies)
		}
		var history struct {
			Versions []merchantPolicyRule `json:"versions"`
		}
		request(t, http.MethodGet, firstPath+"/policies/"+savedPolicy.Rule.ID, admin, nil, http.StatusOK, &history)
		if len(history.Versions) != 1 || history.Versions[0].ID != savedPolicy.Rule.ID {
			t.Fatalf("admin policy history missing: %+v", history)
		}
		request(t, http.MethodGet, secondPath+"/policies/"+savedPolicy.Rule.ID, admin, nil, http.StatusNotFound, nil)
	})

	t.Run("transaction filters and scoped totals", func(t *testing.T) {
		type page struct {
			merchantPage
			Transactions []mockTransaction `json:"transactions"`
		}
		path := firstPath + "/transactions"
		var listed page
		request(t, http.MethodGet, path, tokenA, nil, http.StatusOK, &listed)
		if len(listed.Transactions) != 5 || listed.Total != 0 || listed.Transactions[0].ID != blockedID {
			t.Fatalf("legacy transaction list changed: %+v", listed)
		}
		request(t, http.MethodGet, path+"?page=2&limit=2", tokenA, nil, http.StatusOK, &listed)
		if listed.Page != 2 || listed.Total != 5 || len(listed.Transactions) != 2 || listed.Transactions[0].ID != openID {
			t.Fatalf("transaction page is incorrect: %+v", listed)
		}
		for _, filter := range []struct {
			query, id string
			amount    int64
		}{
			{"outcome=BLOCK", blockedID, 75},
			{"status=REJECTED", blockedID, 75},
			{"status=REVIEW&reviewState=NEEDS_DECISION", openID, 7500},
			{"status=APPROVED&reviewState=RESOLVED", resolvedID, 7600},
			{"outcome=REVIEW&reviewState=NEEDS_DECISION", openID, 7500},
			{"reviewState=RESOLVED&q=" + resolvedID, resolvedID, 7600},
			{"q=" + allowedID, allowedID, 50},
		} {
			request(t, http.MethodGet, path+"?"+filter.query, admin, nil, http.StatusOK, &listed)
			if listed.Total != 1 || len(listed.Transactions) != 1 || listed.Transactions[0].ID != filter.id ||
				listed.Transactions[0].AmountMinor == nil || *listed.Transactions[0].AmountMinor != filter.amount {
				t.Fatalf("filter %s returned wrong totals or amount: %+v", filter.query, listed)
			}
		}
		request(t, http.MethodGet, path+"?status=APPROVED&limit=1&page=2", tokenA, nil, http.StatusOK, &listed)
		if listed.Total != 3 || len(listed.Transactions) != 1 || listed.Transactions[0].ID != allowedID || listed.Transactions[0].Status != "APPROVED" {
			t.Fatalf("status pagination did not preserve total and stable page: %+v", listed)
		}
		from := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		to := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
		request(t, http.MethodGet, path+"?from="+from+"&to="+to, tokenA, nil, http.StatusOK, &listed)
		if listed.Total != 4 || slices.ContainsFunc(listed.Transactions, func(item mockTransaction) bool { return item.ID == oldID }) {
			t.Fatalf("UTC date filter returned wrong rows: %+v", listed)
		}
		request(t, http.MethodGet, path+"?from="+to+"&to="+from, admin, nil, http.StatusBadRequest, nil)
		request(t, http.MethodGet, path+"?outcome=UNKNOWN", admin, nil, http.StatusBadRequest, nil)
		request(t, http.MethodGet, path+"?status=UNKNOWN", admin, nil, http.StatusBadRequest, nil)
		request(t, http.MethodGet, secondPath+"/transactions?q="+allowedID, tokenB, nil, http.StatusOK, &listed)
		if listed.Total != 1 || len(listed.Transactions) != 1 || listed.Transactions[0].MerchantID != second.ID {
			t.Fatal("identical transaction IDs crossed tenant boundaries")
		}
	})

	t.Run("audit and credential revocation", func(t *testing.T) {
		var audit struct {
			Events []merchantAuditEntry `json:"events"`
		}
		request(t, http.MethodGet, firstPath+"/audit", admin, nil, http.StatusOK, &audit)
		actions := make(map[string]bool)
		for _, event := range audit.Events {
			actions[event.Action] = true
		}
		for _, needed := range []string{"MERCHANT_CREATED", "MERCHANT_CREDENTIAL_ISSUED", "MERCHANT_UPDATED", "POLICY_SAVE", "REVIEW_RESOLVED"} {
			if !actions[needed] {
				t.Fatalf("audit is missing %s: %+v", needed, actions)
			}
		}
		request(t, http.MethodDelete, firstPath, tokenA, nil, http.StatusForbidden, nil)
		request(t, http.MethodDelete, firstPath, admin, nil, http.StatusOK, nil)
		request(t, http.MethodDelete, firstPath, admin, nil, http.StatusOK, nil)
		request(t, http.MethodGet, merchantDemoPath, tokenA, nil, http.StatusUnauthorized, nil)
		request(t, http.MethodGet, firstPath, admin, nil, http.StatusNotFound, nil)
		request(t, http.MethodGet, firstPath+"/transactions?page=1", admin, nil, http.StatusNotFound, nil)
		var directory struct {
			Total int64 `json:"total"`
		}
		request(t, http.MethodGet, merchantDemoPath+"?q="+first.ID, admin, nil, http.StatusOK, &directory)
		if directory.Total != 0 {
			t.Fatal("deactivated merchant is still in the directory")
		}
		var deactivations, revocations int
		if err := db.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE action='MERCHANT_DEACTIVATED'),
			count(*) FILTER (WHERE action='MERCHANT_CREDENTIAL_REVOKED') FROM merchant_demo_audit
			WHERE merchant_id=$1::text::uuid`, first.ID).Scan(&deactivations, &revocations); err != nil {
			t.Fatal(err)
		}
		if deactivations != 1 || revocations != 1 {
			t.Fatal("deactivation and revocation must each be audited once")
		}
		request(t, http.MethodGet, secondPath, tokenB, nil, http.StatusOK, nil)
	})

	t.Run("create credential is issued once per idempotency key", func(t *testing.T) {
		name := "Idempotency test " + prefix
		key := "merchant-management-" + prefix
		var created struct {
			Merchant mockMerchant `json:"merchant"`
			Token    string       `json:"token"`
		}
		input := mockMerchantInput{Name: name}
		request(t, http.MethodPost, merchantDemoPath, admin, input, http.StatusBadRequest, nil, "invalid key")
		request(t, http.MethodPost, merchantDemoPath, admin, input, http.StatusCreated, &created, key)
		if created.Merchant.ID == "" || created.Token == "" {
			t.Fatal("the first creation did not return its merchant and one-time credential")
		}
		credentials = append(credentials, created.Token)
		request(t, http.MethodPost, merchantDemoPath, admin, input, http.StatusConflict, nil, key)
		request(t, http.MethodPost, merchantDemoPath, admin, mockMerchantInput{Name: name + " changed"}, http.StatusConflict, nil, key)
		var directory struct {
			Total     int64                   `json:"total"`
			Merchants []merchantManagementRow `json:"merchants"`
		}
		request(t, http.MethodGet, merchantDemoPath+"?q="+url.QueryEscape(name), admin, nil, http.StatusOK, &directory)
		if directory.Total != 1 || len(directory.Merchants) != 1 || directory.Merchants[0].ID != created.Merchant.ID {
			t.Fatal("replaying a creation created another merchant")
		}
	})
}
