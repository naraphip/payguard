package main

import (
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
