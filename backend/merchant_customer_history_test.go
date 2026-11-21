package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestMerchantCustomerEmailChangePostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	makeMerchant := func(name string) mockMerchant {
		t.Helper()
		merchant, _, err := repo.createMockMerchant(t.Context(), mockMerchantInput{
			Name: name, PolicyIntegration: &merchantPolicyIntegration{Products: []string{"physical"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return merchant
	}
	first := makeMerchant("Customer history one")
	second := makeMerchant("Customer history two")
	const admin = "merchant-customer-history-admin"
	const customer = "order-customer:1"
	mux := http.NewServeMux()
	registerMerchantDemo(mux, repo, admin)
	post := func(merchant mockMerchant, input mockTransactionInput, status int) mockTransaction {
		t.Helper()
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, merchantDemoPath+"/"+merchant.ID+"/transactions", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+admin)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("got %d, want %d: %s", response.Code, status, response.Body.String())
		}
		if response.Code != http.StatusCreated && response.Code != http.StatusOK {
			return mockTransaction{}
		}
		if strings.Contains(response.Body.String(), `"customerId"`) || strings.Contains(response.Body.String(), customer) {
			t.Fatalf("customer identifier leaked: %s", response.Body.String())
		}
		var output struct {
			Transaction mockTransaction `json:"transaction"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		return output.Transaction
	}
	amount, age := int64(100), int64(45)
	transaction := func(customerID, domain, mailbox, source string) mockTransactionInput {
		t.Helper()
		var id pgtype.UUID
		if err := db.QueryRow(t.Context(), "SELECT gen_random_uuid()").Scan(&id); err != nil {
			t.Fatal(err)
		}
		var digest string
		if mailbox != "" {
			hash := sha256.Sum256([]byte(mailbox))
			digest = hex.EncodeToString(hash[:])
		}
		return mockTransactionInput{ID: id.String(), CustomerID: customerID, Product: "physical", Source: source,
			CustomerEmailDomain: domain, CustomerMailboxDigest: digest, AmountMinor: &amount, AccountAgeDays: &age}
	}
	assertEmailTransition := func(result mockTransaction, want bool, value int) {
		t.Helper()
		score := result.PolicyDecision.RiskScore
		if result.Outcome != "LOW_RISK" || score == nil || score.Value == nil || *score.Value != value ||
			slices.Contains(score.Factors, merchantRiskFactor{Code: "CUSTOMER_EMAIL_CHANGED_AFTER_DISPOSABLE", Points: 15}) != want {
			t.Fatalf("unexpected assessment: outcome=%s score=%+v", result.Outcome, score)
		}
	}

	post(first, transaction("buyer@example.test", "", "", "checkout"), http.StatusUnprocessableEntity)
	initial := transaction(" "+customer+" ", "mailinator.com", "first@mailinator.com", "checkout")
	assertEmailTransition(post(first, initial, http.StatusCreated), false, 35)
	var storedRef, mailboxRef string
	if err := db.QueryRow(t.Context(), "SELECT customer_id_ref, customer_mailbox_ref FROM merchant_demo_transactions WHERE merchant_id=$1 AND id=$2",
		first.ID, initial.ID).Scan(&storedRef, &mailboxRef); err != nil {
		t.Fatal(err)
	}
	if !validMailboxDigest(storedRef) || storedRef == mailboxRef || storedRef == initial.CustomerMailboxDigest {
		t.Fatal("customer ID must use a separate opaque merchant-scoped reference")
	}
	otherMerchant := transaction(customer, "gmail.com", "buyer@gmail.com", "simulation")
	assertEmailTransition(post(second, otherMerchant, http.StatusCreated), false, 20)
	var otherRef string
	if err := db.QueryRow(t.Context(), "SELECT customer_id_ref FROM merchant_demo_transactions WHERE merchant_id=$1 AND id=$2",
		second.ID, otherMerchant.ID).Scan(&otherRef); err != nil {
		t.Fatal(err)
	}
	if !validMailboxDigest(otherRef) || otherRef == storedRef {
		t.Fatal("each merchant must have a different reference for the same identifier")
	}

	assertEmailTransition(post(first, transaction(customer, "", "", "simulation"), http.StatusCreated), false, 20)
	assertEmailTransition(post(first, transaction("", "gmail.com", "buyer@gmail.com", "checkout"), http.StatusCreated), false, 20)
	assertEmailTransition(post(first, transaction(strings.ToUpper(customer), "gmail.com", "", "checkout"), http.StatusCreated), false, 20)
	change := transaction(customer, "gmail.com", "buyer@gmail.com", "simulation")
	scored := post(first, change, http.StatusCreated)
	assertEmailTransition(scored, true, 35)
	assertEmailTransition(post(first, transaction(customer, "gmail.com", "buyer@gmail.com", "checkout"), http.StatusCreated), false, 20)
	if replayed := post(first, change, http.StatusOK); !reflect.DeepEqual(replayed, scored) {
		t.Fatal("replay did not retain the original history evidence and score")
	}
	change.CustomerID = "another-customer"
	post(first, change, http.StatusConflict)

	assertEmailTransition(post(first, transaction("same-domain", "mailinator.com", "", "checkout"), http.StatusCreated), false, 35)
	assertEmailTransition(post(first, transaction("same-domain", "mailinator.com", "first@mailinator.com", "simulation"), http.StatusCreated), false, 35)
	assertEmailTransition(post(first, transaction("same-domain", "mailinator.com", "second@mailinator.com", "simulation"), http.StatusCreated), true, 50)
	assertEmailTransition(post(first, transaction("same-domain", "gmail.com", "", "checkout"), http.StatusCreated), true, 35)
}
