package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestJevRuleValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		condition string
		valid     bool
	}{
		{"zero", `{"signal":"amountMinor","operator":"eq","value":0}`, true},
		{"velocity", `{"signal":"bookingVelocity60s","operator":"gte","value":2}`, true},
		{"missing value", `{"signal":"amountMinor","operator":"eq"}`, false},
		{"null value", `{"signal":"amountMinor","operator":"eq","value":null}`, false},
		{"string value", `{"signal":"amountMinor","operator":"eq","value":"1"}`, false},
		{"fraction", `{"signal":"amountMinor","operator":"eq","value":0.1}`, false},
		{"negative", `{"signal":"amountMinor","operator":"eq","value":-1}`, false},
		{"too large", `{"signal":"amountMinor","operator":"eq","value":9007199254740992}`, false},
		{"future signal", `{"signal":"paymentToken","operator":"eq","value":1}`, false},
		{"unsupported operator", `{"signal":"amountMinor","operator":"in","value":1}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"  Rule  ","action":"REVIEW","conditions":[`+test.condition+`]}`))
			response := httptest.NewRecorder()
			var definition jevRuleDefinition
			if valid := decodeJevRule(response, request, &definition); valid != test.valid {
				t.Fatalf("valid = %v, want %v: %s", valid, test.valid, response.Body)
			}
			if test.valid && definition.Name != "Rule" {
				t.Fatalf("rule name = %q, want trimmed name", definition.Name)
			}
		})
	}
}

func TestJevRuleAssessmentAndPinnedEvidence(t *testing.T) {
	var definition jevRuleDefinition
	if err := json.Unmarshal([]byte(`{"name":"Zero amount and repeated booking","action":"REVIEW","conditions":[
		{"signal":"bookingVelocity60s","operator":"gte","value":2},
		{"signal":"amountMinor","operator":"eq","value":0}]}`), &definition); err != nil {
		t.Fatal(err)
	}
	if err := definition.validate(); err != nil {
		t.Fatal(err)
	}
	if definition.Conditions[0].Signal != bookingAmount {
		t.Fatal("AND conditions must have a canonical order")
	}
	const ruleID = "jev-rule-fixture"
	snapshot := jevRulesSnapshot{Version: "booking-rules-v2", Rules: []publishedJevRule{{ID: ruleID, jevRuleDefinition: definition}}}
	var zero int64
	for _, test := range []struct {
		name     string
		amount   *int64
		signals  []string
		velocity int64
		mode     bookingJevMode
		outcome  string
		matched  bool
		missing  bool
	}{
		{"threshold hit", &zero, []string{bookingAmount}, 2, bookingJevMock, "REVIEW", true, false},
		{"below threshold", &zero, []string{bookingAmount}, 1, bookingJevMock, "LOW_RISK", false, false},
		{"omitted amount", nil, []string{bookingAmount}, 2, bookingJevMock, "LOW_RISK", false, true},
		{"amount outside profile", &zero, nil, 2, bookingJevMock, "LOW_RISK", false, true},
		{"timeout priority", &zero, []string{bookingAmount}, 2, bookingJevTimeout, "UNRESOLVED", true, false},
		{"built-in still matches", nil, []string{bookingAmount}, 5, bookingJevMock, "REVIEW", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			booking := bookingEvent{bookingInput: bookingInput{AmountMinor: test.amount}}
			profile := profileCandidate{Signals: test.signals}
			assessment := evaluateBooking(booking, "profile-test", profile, test.velocity, test.mode)
			assessment.applyJevRules(snapshot)
			if assessment.RuleVersion != snapshot.Version || assessment.Outcome != test.outcome || slices.Contains(assessment.RuleHits, ruleID) != test.matched {
				t.Fatalf("unexpected assessment: %+v", assessment)
			}
			raw, err := json.Marshal(assessment.Features)
			if err != nil {
				t.Fatal(err)
			}
			var features []bookingFeature
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			if err := decoder.Decode(&features); err != nil {
				t.Fatal(err)
			}
			if matched, missing := definition.matches(jevRuleSignals(features)); matched != test.matched || missing != test.missing {
				t.Fatalf("pinned evidence yielded (%v, %v), want (%v, %v)", matched, missing, test.matched, test.missing)
			}
		})
	}
}

func TestJevRulesPreserveGovernedPolicyVersions(t *testing.T) {
	assessment := evaluateBooking(bookingEvent{}, "profile-test", profileCandidate{}, 2, bookingJevMock)
	policy := bookingGovernancePolicy{
		rule: bookingChangeRule{ReviewThreshold: 3}, ruleVersion: "governed-rule",
		rubric: bookingChangeRubric{ElevatedVelocity: 4}, rubricVersion: "governed-rubric",
	}
	policy.apply(&assessment, "", 2, bookingJevMock)
	threshold := int64(2)
	snapshot := jevRulesSnapshot{Version: "booking-rules-v2", Rules: []publishedJevRule{{
		ID: "jev-rule-velocity",
		jevRuleDefinition: jevRuleDefinition{
			Action: jevReviewAction, Conditions: []jevRuleCondition{{Signal: bookingVelocitySignal, Operator: "gte", Value: &threshold}},
		},
	}}}
	assessment.applyJevRules(snapshot)
	if assessment.RuleVersion != policy.ruleVersion || assessment.JevRuleVersion != snapshot.Version ||
		assessment.Jev.RubricVersion != policy.rubricVersion || assessment.Jev.Answer != bookingJevLow ||
		assessment.Outcome != "REVIEW" || !slices.Contains(assessment.RuleHits, snapshot.Rules[0].ID) {
		t.Fatalf("unexpected combined assessment: %+v", assessment)
	}
}
