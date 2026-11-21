package main

import "testing"

func TestBookingGovernanceCandidateValidation(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		raw       string
		canonical string
	}{
		{"rule", "rule", `{"reviewThreshold":3}`, `{"reviewThreshold":3}`},
		{"typed rule", "rule", `{"reviewThreshold":"3"}`, ""},
		{"positive rule", "rule", `{"reviewThreshold":0}`, ""},
		{"rubric", "rubric", `{"elevatedVelocity":4}`, `{"elevatedVelocity":4}`},
		{"distinct list", "list", `{"reviewProducts":["physical","digital"]}`, `{"reviewProducts":["digital","physical"]}`},
		{"duplicate list product", "list", `{"reviewProducts":["physical","physical"]}`, ""},
		{"unknown list product", "list", `{"reviewProducts":["unknown"]}`, ""},
		{"unknown field", "rule", `{"reviewThreshold":3,"label":"fraud"}`, ""},
		{"null candidate", "list", `null`, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, err := decodeBookingChangeCandidate(test.kind, []byte(test.raw))
			if test.canonical == "" {
				if err == nil {
					t.Errorf("accepted invalid candidate %s", test.raw)
				}
			} else if err != nil || string(got) != test.canonical {
				t.Errorf("candidate = %s, %v; want %s", got, err, test.canonical)
			}
		})
	}
}

func TestBookingGovernanceAppliesPinnedRevisions(t *testing.T) {
	policy := bookingGovernancePolicy{
		rule: bookingChangeRule{ReviewThreshold: 10}, ruleVersion: "rule-new",
		rubric: bookingChangeRubric{ElevatedVelocity: 3}, rubricVersion: "rubric-new",
		list: bookingChangeList{ReviewProducts: []string{"digital"}}, listVersion: "list-new",
	}
	evaluate := func(product string, velocity int64, mode bookingJevMode) bookingAssessment {
		result := evaluateBooking(bookingEvent{}, "profile-new", profileCandidate{}, velocity, mode)
		policy.apply(&result, product, velocity, mode)
		return result
	}
	result := evaluate("physical", 5, bookingJevMock)
	if result.Outcome != "LOW_RISK" || len(result.RuleHits) != 0 || result.Jev.Answer != bookingJevElevated {
		t.Fatalf("governed velocity rule and rubric should act independently: %+v", result)
	}
	result = evaluate("digital", 2, bookingJevMock)
	if result.Outcome != "REVIEW" || len(result.RuleHits) != 1 || result.RuleHits[0] != bookingProductListRuleCode ||
		result.Jev.Answer != bookingJevLow || result.ProfileVersion != "profile-new" ||
		result.RuleVersion != "rule-new" || result.Jev.RubricVersion != "rubric-new" || result.ListVersion != "list-new" {
		t.Fatalf("product list and lineage not applied: %+v", result)
	}
	result = evaluate("digital", 12, bookingJevTimeout)
	if result.Outcome != "UNRESOLVED" || result.Handoff != "MANUAL_REVIEW" || result.Jev.Answer != bookingJevUnavailable ||
		len(result.RuleHits) != 2 {
		t.Fatalf("governance changed mock timeout behavior: %+v", result)
	}
}
