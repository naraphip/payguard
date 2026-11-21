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

func TestMockMerchantJevRuleRejectsUnavailableSignal(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{
		"name":"Velocity","action":"REVIEW",
		"conditions":[{"signal":"bookingVelocity60s","operator":"gte","value":5}]
	}`))
	var definition jevRuleDefinition
	if decodeMockJevRule(w, r, &definition) || w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported signal accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestMockMerchantJevRulesPinRevisionAndPreserveScenario(t *testing.T) {
	threshold := int64(0)
	scenario := mockScenario{ID: "everyday", ThresholdMinor: 100, ScenarioVersion: 2, RuleVersion: "rules-v2"}
	snapshot := jevRulesSnapshot{
		Version: "booking-rules-v3",
		Rules: []publishedJevRule{
			{ID: "jev-rule-zero", jevRuleDefinition: jevRuleDefinition{
				Name: "Zero booking amount", Action: "REVIEW",
				Conditions: []jevRuleCondition{{Signal: bookingAmount, Operator: "eq", Value: &threshold}},
			}},
		},
	}
	for _, test := range []struct {
		name    string
		amount  *int64
		timeout bool
		want    string
		hits    []string
	}{
		{"zero counts as observed", &threshold, false, "REVIEW", []string{"jev-rule-zero"}},
		{"scenario threshold still applies", &scenario.ThresholdMinor, false, "REVIEW", []string{"AMOUNT_REVIEW_THRESHOLD"}},
		{"missing remains unresolved", nil, false, "UNRESOLVED", nil},
		{"timeout remains unresolved", &threshold, true, "UNRESOLVED", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := evaluateMockJev(mockTransactionInput{AmountMinor: test.amount, SimulateTimeout: test.timeout}, scenario)
			result.applyMerchantJevRules(snapshot)
			if result.Outcome != test.want || !slices.Equal(result.RuleHits, test.hits) {
				t.Fatalf("unexpected decision: %+v", result)
			}
			if result.RuleVersion != scenario.RuleVersion || result.JevRuleVersion != snapshot.Version {
				t.Fatalf("scenario and Jev versions not independently pinned: %+v", result)
			}
		})
	}
}

func TestMerchantJevRuleRevisionsPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db: db}
	merchant, merchantToken, err := repo.createMockMerchant(t.Context(), mockMerchantInput{
		Name: "Jev revision test", PolicyIntegration: &merchantPolicyIntegration{Products: []string{"physical"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := repo.createMockMerchant(t.Context(), mockMerchantInput{Name: "Other Jev revision test"})
	if err != nil {
		t.Fatal(err)
	}
	admin := "merchant-jev-test-admin"
	mux := http.NewServeMux()
	registerMerchantDemo(mux, repo, admin)
	call := func(method, path, token string, data any, status int, output any) {
		t.Helper()
		var body bytes.Buffer
		if data != nil {
			if err := json.NewEncoder(&body).Encode(data); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &body)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, w.Code, status, w.Body)
		}
		if output != nil {
			if err := json.Unmarshal(w.Body.Bytes(), output); err != nil {
				t.Fatal(err)
			}
		}
	}
	root := merchantDemoPath + "/" + merchant.ID
	rules := root + "/jev/rules"
	makeDefinition := func(name string, threshold int64) jevRuleDefinition {
		return jevRuleDefinition{
			Name: name, Action: jevReviewAction, Conditions: []jevRuleCondition{
				{Signal: bookingAmount, Operator: "gte", Value: &threshold},
			},
		}
	}
	first := makeDefinition("Review amounts of 200 or more", 200)
	var original jevRulesSnapshot
	call(http.MethodPost, rules, admin, first, http.StatusOK, &original)
	if original.Version != "booking-rules-v2" || len(original.Rules) != 1 {
		t.Fatalf("unexpected first publication: %+v", original)
	}
	id := original.Rules[0].ID
	rule := rules + "/" + id
	amount := int64(300)
	input := mockTransactionInput{ID: "946d7166-0c77-47e9-a918-43ab90fd0ee8", Source: "checkout", Product: "physical", AmountMinor: &amount}
	var evaluated struct{ Transaction mockTransaction }
	call(http.MethodPost, root+"/transactions", admin, input, http.StatusCreated, &evaluated)
	if evaluated.Transaction.JevRuleVersion != original.Version || !slices.Contains(evaluated.Transaction.RuleHits, id) {
		t.Fatalf("transaction failed to pin the original Jev ruleset: %+v", evaluated.Transaction)
	}

	revised := makeDefinition("Review amounts of 400 or more", 400)
	edit := map[string]any{"definition": revised, "expectedVersion": original.Version}
	remove := map[string]string{"expectedVersion": original.Version}
	call(http.MethodPut, rule, "", edit, http.StatusUnauthorized, nil)
	call(http.MethodDelete, rule, merchantToken, remove, http.StatusForbidden, nil)
	call(http.MethodPut, rule, admin, map[string]any{"definition": revised}, http.StatusUnprocessableEntity, nil)
	call(http.MethodDelete, merchantDemoPath+"/"+other.ID+"/jev/rules/"+id, admin,
		map[string]string{"expectedVersion": bookingRuleVersion}, http.StatusNotFound, nil)
	var updated jevRulesSnapshot
	call(http.MethodPut, rule, admin, edit, http.StatusOK, &updated)
	if updated.Version != "booking-rules-v3" || len(updated.Rules) != 1 || updated.Rules[0].ID != id ||
		updated.Rules[0].Name != revised.Name || *updated.Rules[0].Conditions[0].Value != 400 {
		t.Fatalf("update did not preserve the expected rule identity: %+v", updated)
	}
	afterEdit := input
	afterEdit.ID = "946d7166-0c77-47e9-a918-43ab90fd0ee9"
	var evaluatedAfterEdit struct{ Transaction mockTransaction }
	call(http.MethodPost, root+"/transactions", admin, afterEdit, http.StatusCreated, &evaluatedAfterEdit)
	if evaluatedAfterEdit.Transaction.JevRuleVersion != updated.Version || slices.Contains(evaluatedAfterEdit.Transaction.RuleHits, id) {
		t.Fatalf("a new transaction failed to apply the updated threshold: %+v", evaluatedAfterEdit.Transaction)
	}
	call(http.MethodPut, rule, admin, edit, http.StatusConflict, nil)
	call(http.MethodDelete, rule, admin, remove, http.StatusConflict, nil)
	call(http.MethodDelete, rule, admin, map[string]string{"expectedVersion": updated.Version}, http.StatusOK, &updated)
	if updated.Version != "booking-rules-v4" || len(updated.Rules) != 0 {
		t.Fatalf("removal did not produce a new empty ruleset: %+v", updated)
	}
	call(http.MethodDelete, rule, admin, map[string]string{"expectedVersion": updated.Version}, http.StatusNotFound, nil)

	var replayed struct{ Transaction mockTransaction }
	call(http.MethodPost, root+"/transactions", admin, input, http.StatusOK, &replayed)
	if replayed.Transaction.JevRuleVersion != original.Version || !slices.Equal(replayed.Transaction.RuleHits, evaluated.Transaction.RuleHits) {
		t.Fatalf("editing and removing a rule changed a saved transaction: %+v", replayed.Transaction)
	}
	call(http.MethodPost, rules, admin, first, http.StatusOK, &updated)
	if updated.Version != "booking-rules-v5" || len(updated.Rules) != 1 || updated.Rules[0].ID == id {
		t.Fatalf("recreating a removed definition did not receive a distinct rule identity: %+v", updated)
	}
	call(http.MethodPost, rules, admin, first, http.StatusOK, &updated)
	if updated.Version != "booking-rules-v5" || len(updated.Rules) != 1 {
		t.Fatalf("publishing the same active definition should be idempotent: %+v", updated)
	}
	var historicalName string
	err = db.QueryRow(t.Context(), `SELECT definition->>'name' FROM booking_jev_rules
		WHERE merchant_id=$1 AND revision=2 AND rule_id=$2`, merchant.ID, id).Scan(&historicalName)
	if err != nil || historicalName != first.Name {
		t.Fatalf("publication history was rewritten: name=%q err=%v", historicalName, err)
	}
}
