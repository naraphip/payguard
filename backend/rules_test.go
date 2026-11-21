package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestRuleInputValidation(t *testing.T) {
	input := ruleInput{
		Name: " Token count ", Description: " Example ", Stage: "PAYMENT_STAGES", Action: "REVIEW",
		Conditions: []ruleCondition{{"payment_token_orders", "gte", float64(2)}, {"payment_token_missing", "eq", false}},
	}
	if err := input.validate(); err != nil || input.Name != "Token count" || input.Description != "Example" {
		t.Fatalf("expected normalized valid rule, got %+v: %v", input, err)
	}
	for _, test := range []struct {
		name, stage, action string
		conditions          []ruleCondition
	}{
		{"unknown stage", "DELIVERY", "REVIEW", input.Conditions},
		{"token on booking", "BOOKING", "REVIEW", input.Conditions},
		{"token on any", "ANY", "REVIEW", input.Conditions},
		{"blocking action", "PAYMENT_STAGES", "BLOCK", input.Conditions},
		{"too few conditions", "PAYMENT_STAGES", "REVIEW", nil},
		{"too many conditions", "PAYMENT_STAGES", "REVIEW", slices.Concat(input.Conditions, input.Conditions, input.Conditions)},
		{"unknown signal", "ANY", "REVIEW", []ruleCondition{{"ground_truth", "eq", true}}},
		{"boolean as number", "PAYMENT", "REVIEW", []ruleCondition{{"payment_token_orders", "gte", true}}},
		{"null number", "PAYMENT", "REVIEW", []ruleCondition{{"payment_token_orders", "gte", nil}}},
		{"numeric operator on boolean", "PRE_PAYMENT", "CHALLENGE", []ruleCondition{{"payment_token_missing", "gte", false}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := ruleInput{Name: input.Name, Stage: test.stage, Action: test.action, Conditions: test.conditions}
			if err := invalid.validate(); err == nil {
				t.Fatal("accepted invalid rule")
			}
		})
	}
}

func TestSharedBuiltinRulesPreservePrecedence(t *testing.T) {
	for _, test := range []struct {
		name, stage, action, jev, hit string
		age                           int
		features                      []feature
		missing                       bool
	}{
		{"both shared", "PRE_PAYMENT", "REVIEW", "HIGH", "SHARED_DEVICE_AND_TOKEN", 1, []feature{{"device", 2}, {"token", 2}}, false},
		{"challenge before missing", "PAYMENT", "CHALLENGE", "MEDIUM", "NEW_ACCOUNT_SHARED_INSTRUMENT_OR_DEVICE", 7, []feature{{"device", 2}}, true},
		{"shared token only", "PAYMENT", "CHALLENGE", "MEDIUM", "NEW_ACCOUNT_SHARED_INSTRUMENT_OR_DEVICE", 1, []feature{{"device", 1}, {"token", 2}}, false},
		{"booking device signal", "BOOKING", "CHALLENGE", "MEDIUM", "NEW_ACCOUNT_SHARED_INSTRUMENT_OR_DEVICE", 1, []feature{{"device", 2}}, false},
		{"missing token", "PRE_PAYMENT", "REVIEW", "INSUFFICIENT_EVIDENCE", "PAYMENT_TOKEN_MISSING", 8, []feature{{"device", 2}}, true},
		{"zero token", "PAYMENT", "REVIEW", "INSUFFICIENT_EVIDENCE", "PAYMENT_TOKEN_MISSING", 8, []feature{{"token", 0}}, true},
		{"clean", "PAYMENT", "ALLOW", "LOW", "", 8, []feature{{"device", 1}, {"token", 1}}, false},
		{"checkout without token", "CHECKOUT", "ALLOW", "LOW", "", 1, []feature{{"device", 1}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := assess(event{Stage: test.stage, AgeDays: test.age}, test.features, false, false)
			expectedHits := []string{}
			if test.hit != "" {
				expectedHits = append(expectedHits, test.hit)
			}
			if d.Platform != test.action || d.JevAnswer != test.jev || !slices.Equal(d.RuleHits, expectedHits) ||
				slices.Contains(d.Missing, "payment_token") != test.missing {
				t.Fatalf("unexpected decision: %+v", d)
			}
		})
	}
}

func TestDraftPreviewSkipsAbsentSignalsAndFiltersByStage(t *testing.T) {
	saved := run{ID: "saved", RuleVersion: "prior-rules-v1", Decisions: []decision{
		{Event: event{ID: "booking", Stage: "BOOKING", AgeDays: 2}},
		{Event: event{ID: "no-token", Stage: "PAYMENT", AgeDays: 2}},
		{Event: event{ID: "old-no-token", Stage: "PRE_PAYMENT", AgeDays: 40}},
		{Event: event{ID: "zero-token", Stage: "PRE_PAYMENT", AgeDays: 2}, Features: []feature{{"token", 0}}},
		{Event: event{ID: "young-token", Stage: "PAYMENT", AgeDays: 2}, Features: []feature{{"token", 1}}},
		{Event: event{ID: "old-token", Stage: "PAYMENT", AgeDays: 40}, Features: []feature{{"token", 1}}},
	}}
	input := ruleInput{Stage: "PAYMENT_STAGES", Conditions: []ruleCondition{
		{"account_age_days", "lte", float64(7)},
		{"payment_token_orders", "eq", float64(0)},
	}}
	got := previewRule(saved, input)
	if got.RunID != saved.ID || got.RuleVersion != saved.RuleVersion || got.Eligible != 3 || got.Matched != 1 ||
		got.Skipped != 2 || !slices.Equal(got.MatchedEventIDs, []string{"zero-token"}) {
		t.Fatalf("unexpected preview: %+v", got)
	}
	input.Conditions[1] = ruleCondition{"payment_token_missing", "eq", true}
	got = previewRule(saved, input)
	if got.Eligible != 5 || got.Matched != 2 || got.Skipped != 0 ||
		!slices.Equal(got.MatchedEventIDs, []string{"no-token", "zero-token"}) {
		t.Fatalf("unexpected explicit-missing preview: %+v", got)
	}
}

func TestDraftRoutesRejectUntrustedFieldsAndActiveEdits(t *testing.T) {
	app := app{}
	post := func(body string) int {
		w := httptest.NewRecorder()
		app.createRule(w, httptest.NewRequest(http.MethodPost, "/api/v1/system/rules", strings.NewReader(body)))
		return w.Code
	}
	if got := post(`{"name":"An active rule","status":"ACTIVE"}`); got != http.StatusBadRequest {
		t.Fatalf("accepted client-supplied active status: %d", got)
	}
	if got := post(`{} {}`); got != http.StatusBadRequest {
		t.Fatalf("accepted trailing document: %d", got)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/rules/"+builtinRules[0].ID, nil)
	req.SetPathValue("id", builtinRules[0].ID)
	app.updateRule(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("active rule edit did not return forbidden: %d", w.Code)
	}
}
