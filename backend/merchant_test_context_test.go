package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestMerchantTestContextPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	ctx := t.Context()
	merchant, token, err := repo.createMockMerchant(ctx, mockMerchantInput{
		Name:              "Rule test merchant",
		PolicyIntegration: &merchantPolicyIntegration{Products: []string{"physical"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := repo.createMockMerchant(ctx, mockMerchantInput{Name: "Empty test merchant"})
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := repo.saveMockScenario(ctx, merchant.ID, "", "Initial version", 5000)
	if err != nil {
		t.Fatal(err)
	}
	scenario, err = repo.saveMockScenario(ctx, merchant.ID, scenario.ID, "Latest version", 6000)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.saveMockTransaction(ctx, merchant.ID, mockTransactionInput{
		ID: "379e4b7e-692f-45f4-bcf0-01334e8c8489", ScenarioID: scenario.ID, Source: "simulation", Product: "digital",
	}); err != nil {
		t.Fatal(err)
	}
	amount := int64(1234)
	jevSnapshot, err := repo.publishJevRule(ctx, merchant.ID, jevRuleDefinition{
		Name: "Jev amount review", Action: jevReviewAction,
		Conditions: []jevRuleCondition{{Signal: bookingAmount, Operator: "gte", Value: &amount}},
	})
	if err != nil {
		t.Fatal(err)
	}

	insertPolicy := func(ruleID string, version int, state, name, field, rawValue string, hours, ageHours int) {
		t.Helper()
		proposal := merchantPolicyProposal{
			Name: name, Action: "BLOCK", Scope: "MERCHANT", ProductTypes: []string{"physical"}, Stages: []string{"CHECKOUT"}, DurationHours: hours,
			Conditions: merchantPolicyConditions{Logic: "AND", Rules: []merchantPolicyCondition{{Field: field, Operator: "eq", Value: json.RawMessage(rawValue)}}},
		}
		_, err := db.Exec(ctx, `INSERT INTO merchant_policy_rule_versions
			(merchant_id, rule_id, version, status, proposal, preview, created_by, created_at, change_reason)
			VALUES ($1::text::uuid, $2::text::uuid, $3, $4, $5, '{}'::jsonb, 'fixture', statement_timestamp() - $6 * interval '1 hour', 'fixture')`,
			merchant.ID, ruleID, version, state, proposal, ageHours)
		if err != nil {
			t.Fatal(err)
		}
	}
	const (
		mailboxRule   = "379e4b7e-692f-45f4-bcf0-01334e8c8481"
		productRule   = "379e4b7e-692f-45f4-bcf0-01334e8c8482"
		expiredRule   = "379e4b7e-692f-45f4-bcf0-01334e8c8483"
		disabledRule  = "379e4b7e-692f-45f4-bcf0-01334e8c8484"
		nameReference = "abcdef.abcdef"
	)
	mailboxValue, err := json.Marshal(strings.Repeat("abcdef", 10) + "abcd")
	if err != nil {
		t.Fatal(err)
	}
	insertPolicy(mailboxRule, 1, "ACTIVE", "Email block "+nameReference+" (12 h)", policyMailbox, string(mailboxValue), 12, 1)
	insertPolicy(mailboxRule, 2, "PENDING_APPROVAL", "Pending change", policyProduct, `"physical"`, 0, 0)
	insertPolicy(productRule, 1, "ACTIVE", "Restricted product", policyProduct, `"physical"`, 0, 0)
	insertPolicy(expiredRule, 1, "ACTIVE", "Expired policy", policyMailbox, string(mailboxValue), 1, 2)
	insertPolicy(disabledRule, 1, "ACTIVE", "Former policy", policyProduct, `"physical"`, 0, 0)
	insertPolicy(disabledRule, 2, "DISABLED", "Disabled policy", policyProduct, `"physical"`, 0, 0)

	const admin = "merchant-test-context-admin"
	mux := http.NewServeMux()
	registerMerchantDemo(mux, repo, admin)
	type testContext struct {
		Products  []string             `json:"products"`
		Scenarios json.RawMessage      `json:"scenarios"`
		Fields    []merchantCapability `json:"fieldRegistry"`
		Policies  []merchantTestPolicy `json:"policies"`
		JevRules  jevRulesSnapshot     `json:"jevRules"`
		Platform  string               `json:"platformRuleVersion"`
		Missing   []string             `json:"unavailablePlatformSignals"`
	}
	request := func(method, id, credential string, expectedStatus int) testContext {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, method, merchantDemoPath+"/"+id+"/test-context", nil)
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		result := httptest.NewRecorder()
		mux.ServeHTTP(result, req)
		if result.Code != expectedStatus {
			t.Fatalf("test context returned HTTP %d, want %d", result.Code, expectedStatus)
		}
		if bytes.Contains(result.Body.Bytes(), mailboxValue) || bytes.Contains(result.Body.Bytes(), []byte(nameReference)) {
			t.Fatal("test context exposed a mailbox reference")
		}
		var response testContext
		if expectedStatus == http.StatusOK {
			if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}
		return response
	}
	request(http.MethodGet, merchant.ID, "", http.StatusUnauthorized)
	request(http.MethodGet, merchant.ID, token, http.StatusForbidden)
	request(http.MethodGet, other.ID, token, http.StatusForbidden)
	request(http.MethodPost, merchant.ID, admin, http.StatusMethodNotAllowed)
	request(http.MethodGet, "379e4b7e-692f-45f4-bcf0-01334e8c8480", admin, http.StatusNotFound)
	result := request(http.MethodGet, merchant.ID, admin, http.StatusOK)
	if !slices.Equal(result.Products, []string{"digital", "physical"}) {
		t.Fatalf("test context products = %v, want declared and observed products", result.Products)
	}
	if result.Scenarios != nil || len(result.Fields) == 0 || result.Platform != ruleVersion ||
		!slices.Contains(result.Missing, "ip_orders") || slices.Contains(result.Missing, merchantAccountAgeSignal) {
		t.Fatalf("test context should provide the shared fields and platform capabilities without scenarios: %+v", result)
	}
	if len(result.Policies) != 2 {
		t.Fatalf("test context returned %d policies, want two active policies", len(result.Policies))
	}
	mailbox, product := result.Policies[0], result.Policies[1]
	if mailbox.ID != mailboxRule || mailbox.Version != 1 || mailbox.ExpiresAt == nil || mailbox.Name != "Temporary email block" || len(mailbox.Conditions.Rules) != 1 || mailbox.Conditions.Rules[0].Field != policyMailbox || mailbox.Conditions.Rules[0].Value != nil {
		t.Fatal("test context did not show the effective mailbox rule with a redacted condition and expiry")
	}
	if product.ID != productRule || product.Action != "BLOCK" || product.Conditions.Logic != "AND" || len(product.Conditions.Rules) != 1 || string(product.Conditions.Rules[0].Value) != `"physical"` {
		t.Fatal("test context did not show the active product rule and its comparison")
	}
	if result.JevRules.Version != "booking-rules-v2" || result.JevRules.Version != jevSnapshot.Version || len(result.JevRules.Rules) != 1 {
		t.Fatal("test context did not show the published merchant Jev rule snapshot")
	}
	jevRule := result.JevRules.Rules[0]
	if jevRule.ID != jevSnapshot.Rules[0].ID || jevRule.Action != jevReviewAction || len(jevRule.Conditions) != 1 {
		t.Fatal("test context did not show the published merchant Jev rule")
	}
	condition := jevRule.Conditions[0]
	if condition.Signal != bookingAmount || condition.Operator != "gte" || condition.Value == nil || *condition.Value != amount {
		t.Fatal("test context did not show the published merchant Jev rule condition")
	}
	empty := request(http.MethodGet, other.ID, admin, http.StatusOK)
	if empty.Products == nil || empty.Scenarios != nil || empty.Policies == nil || len(empty.Products)+len(empty.Policies) != 0 {
		t.Fatal("test context for another merchant should have empty products and policies without scenarios")
	}
	if empty.JevRules.Version != bookingRuleVersion || empty.JevRules.Rules == nil || len(empty.JevRules.Rules) != 0 {
		t.Fatal("test context for another merchant should have the baseline Jev version and no published rules")
	}
}
