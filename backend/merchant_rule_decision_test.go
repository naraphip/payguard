package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestMerchantRuleDecisionUsesObservedEvidence(t *testing.T) {
	zero := int64(0)
	platform := activePlatformRule{ID: "age-check", Version: 2, Rule: ruleInput{
		Stage: "CHECKOUT", Action: "REVIEW",
		Conditions: []ruleCondition{{Signal: merchantAccountAgeSignal, Operator: "lte", Value: float64(4)}},
	}}
	var run mockTransactionInput
	run.Product, run.AmountMinor = "physical", &zero
	withoutAge := evaluateMerchantRuleDecision(t.Context(), run, []activePlatformRule{platform}, nil, merchantPolicyEvidence{}, jevRulesSnapshot{}, merchantJev{})
	if withoutAge.Outcome != "LOW_RISK" || !slices.Contains(withoutAge.PolicyDecision.Platform.MissingRuleIDs, platform.ID) {
		t.Fatalf("omitted age was not treated as unavailable: %+v", withoutAge)
	}
	run.AccountAgeDays = &zero
	observedZero := evaluateMerchantRuleDecision(t.Context(), run, []activePlatformRule{platform}, nil, merchantPolicyEvidence{}, jevRulesSnapshot{}, merchantJev{})
	if observedZero.Outcome != "REVIEW" || observedZero.PolicyDecision.Final.Source != "PLATFORM" ||
		observedZero.PolicyDecision.Platform.RuleVersions[platform.ID] != 2 {
		t.Fatalf("observed zero did not apply the governed rule: %+v", observedZero)
	}
	run.Product = "flight_ticket"
	differentStage := evaluateMerchantRuleDecision(t.Context(), run, []activePlatformRule{platform}, nil, merchantPolicyEvidence{}, jevRulesSnapshot{}, merchantJev{})
	if differentStage.Outcome != "LOW_RISK" || slices.Contains(differentStage.PolicyDecision.Platform.MissingRuleIDs, platform.ID) {
		t.Fatalf("checkout rule affected a booking: %+v", differentStage)
	}
}

func TestMerchantRuleDecisionKeepsJevEvidenceSeparate(t *testing.T) {
	amount := int64(123)
	input := mockTransactionInput{
		AmountMinor: &amount, Product: "physical", CustomerEmailDomain: "example.test", AccountAgeDays: &amount,
		CustomSignals: map[string]json.RawMessage{"custom_title": json.RawMessage(`"Ocean Park ticket"`)},
	}
	jev := testMerchantJev(t, func(r *http.Request) (*http.Response, error) {
		var request struct {
			State map[string]any `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		expected := map[string]any{"booking_amount_minor": float64(amount), "configured_review_rule_matched": false}
		if !reflect.DeepEqual(request.State, expected) {
			t.Fatalf("provider state = %v, want private rule evidence omitted", request.State)
		}
		return testJevResponse(200, `{"id":"gen-rule-check","model":"typesafe/jev-1.14","answers":{"booking_risk":{"type":"choice","choice":"LOW_RISK"}}}`), nil
	})
	result := resolveMerchantRuleDecision(t.Context(), input, policyDecisionPart{Action: "ALLOW"}, "rules-v1", policyDecisionPart{Action: "BLOCK"}, jevRulesSnapshot{}, jev)
	if result.Outcome != "HIGH_RISK" || result.ReviewState != "NONE" || result.PolicyDecision.Final.Source != "MERCHANT_POLICY" ||
		result.JevStatus != "LIVE" || result.JevAnswer != "LOW_RISK" || result.JevDecisionID != "gen-rule-check" ||
		result.PolicyDecision.Jev.Action != "ALLOW" || !strings.Contains(result.JevReason, "low risk") {
		t.Fatalf("merchant block obscured Jev evidence: %+v", result)
	}
	input.SimulateTimeout = true
	timeout := evaluateMerchantRuleDecision(t.Context(), input, nil, nil, merchantPolicyEvidence{}, jevRulesSnapshot{}, jev)
	if timeout.Outcome != "UNRESOLVED" || timeout.ReviewState != "NEEDS_DECISION" || timeout.PolicyDecision.Final.Source != "JEV" {
		t.Fatalf("missing Jev decision did not open manual review: %+v", timeout)
	}
	input.AmountMinor, input.SimulateTimeout = nil, false
	missing := evaluateMerchantRuleDecision(t.Context(), input, nil, nil, merchantPolicyEvidence{}, jevRulesSnapshot{}, jev)
	if missing.Outcome != "UNRESOLVED" || missing.JevStatus != "NOT_RUN" || !slices.Contains(missing.PolicyDecision.Jev.MissingSignals, policyAmount) {
		t.Fatalf("missing amount did not show incomplete Jev evidence: %+v", missing)
	}
}

func TestMerchantRuleDecisionPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	ctx := t.Context()
	merchant, token, err := repo.createMockMerchant(ctx, mockMerchantInput{
		Name:              "Rule-based transaction without a scenario",
		PolicyIntegration: &merchantPolicyIntegration{Products: []string{"physical"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := repo.createMockMerchant(ctx, mockMerchantInput{Name: "Another merchant without scenarios"})
	if err != nil {
		t.Fatal(err)
	}
	ruleID := strings.ReplaceAll(merchant.ID, "-", "")
	approved := ruleInput{Name: "Young account", Stage: "CHECKOUT", Action: "CHALLENGE",
		Conditions: []ruleCondition{{Signal: merchantAccountAgeSignal, Operator: "lte", Value: float64(7)}}}
	if _, err := db.Exec(ctx, `INSERT INTO platform_rule_revisions
		(rule_id,version,draft_version,action,rule_input,preview,reason,actor)
		VALUES ($1,1,1,'APPROVED',$2,'{}','Test fixture','platform-checker')`, ruleID, approved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := db.Exec(context.Background(), "DELETE FROM platform_rule_revisions WHERE rule_id=$1", ruleID)
		if err != nil {
			t.Error(err)
		}
	})

	const admin = "merchant-rule-decision-test-admin"
	mux := http.NewServeMux()
	registerMerchantDemo(mux, repo, admin)
	path := merchantDemoPath + "/" + merchant.ID + "/transactions"
	call := func(method, path, credential string, input any, want int) []byte {
		t.Helper()
		var body bytes.Buffer
		if input != nil {
			if err := json.NewEncoder(&body).Encode(input); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequestWithContext(ctx, method, path, &body)
		req.Header.Set("Authorization", "Bearer "+credential)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s %s gave %d (want %d): %s", method, path, response.Code, want, response.Body)
		}
		return response.Body.Bytes()
	}
	decode := func(body []byte) mockTransaction {
		t.Helper()
		var response struct {
			Transaction mockTransaction `json:"transaction"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		return response.Transaction
	}
	amount, age := int64(jevRuleMaxValue), int64(1)
	makeInput := func(source string) mockTransactionInput {
		t.Helper()
		id := pgtype.UUID{}
		if err := db.QueryRow(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
			t.Fatal(err)
		}
		return mockTransactionInput{ID: id.String(), Product: "physical", Source: source, AmountMinor: &amount}
	}
	call(http.MethodPost, merchantDemoPath+"/"+other.ID+"/transactions", admin, makeInput("checkout"), http.StatusUnprocessableEntity)
	undeclared := makeInput("checkout")
	undeclared.Product = "digital"
	call(http.MethodPost, path, admin, undeclared, http.StatusUnprocessableEntity)
	profile, _, err := readMerchantPolicyContext(ctx, db, merchant.ID)
	if err != nil || profile.ScoringProfileVersion != merchantRuleProfileVersion {
		t.Fatalf("merchant profile has stale version %q: %v", profile.ScoringProfileVersion, err)
	}
	for _, source := range []string{"checkout", "simulation"} {
		input := makeInput(source)
		first := call(http.MethodPost, path, admin, input, http.StatusCreated)
		result := decode(first)
		if result.Outcome != "LOW_RISK" || result.JevAnswer != "LOW_RISK" || result.ThresholdMinor != nil ||
			bytes.Contains(first, []byte(`"scenarioId"`)) || !strings.Contains(result.RuleVersion, "+governed-") {
			t.Fatalf("unexpected threshold or venue for merchant-only %s result: %s", source, first)
		}
		replayed := call(http.MethodPost, path, admin, input, http.StatusOK)
		if !bytes.Equal(first, replayed) || !bytes.Equal(first, call(http.MethodGet, path+"/"+input.ID, token, nil, http.StatusOK)) {
			t.Fatalf("saved %s result or replay did not round-trip", source)
		}
		input.AccountAgeDays = &age
		call(http.MethodPost, path, admin, input, http.StatusConflict)
		call(http.MethodPost, path, token, input, http.StatusForbidden)
		call(http.MethodGet, merchantDemoPath+"/"+other.ID+"/transactions", token, nil, http.StatusNotFound)
	}
	input := makeInput("checkout")
	input.AccountAgeDays = &age
	challenged := decode(call(http.MethodPost, path, admin, input, http.StatusCreated))
	if challenged.Outcome != "REVIEW" || challenged.ReviewState != "NONE" ||
		challenged.PolicyDecision.Final != (policyFinal{Action: "CHALLENGE", Source: "PLATFORM"}) ||
		challenged.PolicyDecision.Platform.RuleVersions[ruleID] != 1 {
		t.Fatalf("approved platform challenge was not applied: %+v", challenged)
	}
	call(http.MethodPost, path+"/"+input.ID+"/decision", admin, map[string]any{"decision": "cleared"}, http.StatusConflict)
	if read := decode(call(http.MethodGet, path+"/"+input.ID, token, nil, http.StatusOK)); read.ReviewState != challenged.ReviewState {
		t.Fatalf("persisted challenge changed review state: %+v", read)
	}
	legacyScenario, err := repo.saveMockScenario(ctx, merchant.ID, "", "Legacy threshold", 0)
	if err != nil {
		t.Fatal(err)
	}
	input = makeInput("checkout")
	input.ScenarioID = legacyScenario.ID
	input.Product = "digital"
	legacy := call(http.MethodPost, path, admin, input, http.StatusCreated)
	if result := decode(legacy); result.ScenarioID != legacyScenario.ID || result.ThresholdMinor == nil || *result.ThresholdMinor != 0 ||
		result.Outcome != "REVIEW" || result.PolicyDecision.Jev != nil || result.PolicyDecision.ScoringProfileVersion != merchantScenarioProfileVersion {
		t.Fatalf("legacy zero threshold or review lineage changed: %s", legacy)
	}
	if !bytes.Equal(legacy, call(http.MethodPost, path, admin, input, http.StatusOK)) {
		t.Fatal("legacy request did not replay its pinned assessment")
	}
	observed := decode(call(http.MethodPost, path, admin, undeclared, http.StatusCreated))
	if observed.Product != "digital" || observed.PolicyDecision.ScoringProfileVersion != merchantRuleProfileVersion {
		t.Fatalf("observed product did not enter merchant's effective catalogue: %+v", observed)
	}
}
