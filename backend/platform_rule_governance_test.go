package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestPlatformCheckerRequiresIndependentConfiguration(t *testing.T) {
	for _, check := range []struct {
		name, configuredChecker, bearer string
		expected                        int
	}{
		{"admin cannot approve", "second-reviewer-token", "maker-token", http.StatusForbidden},
		{"checker can reach approval validation", "second-reviewer-token", "second-reviewer-token", http.StatusUnprocessableEntity},
		{"same credential fails closed", "maker-token", "maker-token", http.StatusForbidden},
		{"no checker configured", "", "maker-token", http.StatusForbidden},
		{"unknown checker", "second-reviewer-token", "someone-else", http.StatusUnauthorized},
	} {
		t.Run(check.name, func(t *testing.T) {
			mux := http.NewServeMux()
			registerPlatformRuleGovernance(mux, repository{}, "maker-token", check.configuredChecker)
			req := httptest.NewRequest(http.MethodPost, composerPath+"/platform/00000000-0000-4000-8000-000000000001/approve", strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer "+check.bearer)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != check.expected {
				t.Fatalf("got %d, want %d: %s", recorder.Code, check.expected, recorder.Body.String())
			}
		})
	}
}

func TestGovernedPlatformRulesPreserveDecisionPrecedence(t *testing.T) {
	req := event{Stage: "PAYMENT", Product: "flight_ticket", Domain: "restricted.test", AgeDays: 100}
	conditions := []ruleCondition{{Signal: "device_orders", Operator: "gte", Value: float64(2)}}
	rules := []activePlatformRule{
		{ID: "challenge", Version: 2, Rule: ruleInput{Stage: "PAYMENT", Action: "CHALLENGE", Conditions: conditions}},
		{ID: "review", Version: 5, Rule: ruleInput{Stage: "PAYMENT", Action: "REVIEW", Conditions: conditions}},
		{ID: "wrong-stage", Version: 2, Rule: ruleInput{Stage: "BOOKING", Action: "REVIEW", Conditions: conditions}},
		{ID: "untrusted-action", Version: 2, Rule: ruleInput{Stage: "PAYMENT", Action: "BLOCK", Conditions: conditions}},
	}
	observed := []feature{{Kind: "device", Orders: 2}, {Kind: "token", Orders: 1}}
	for _, check := range []struct {
		name, action, source string
		merchant, timeout    bool
	}{
		{"escalate", "REVIEW", "PLATFORM", false, false},
		{"keep merchant block", "BLOCK", "MERCHANT_POLICY", true, false},
		{"keep timeout", "UNSUCCESSFUL", "JEV_TIMEOUT", true, true},
	} {
		t.Run(check.name, func(t *testing.T) {
			result := assess(req, observed, check.merchant, check.timeout)
			applyGovernedPlatformRules(&result, rules)
			if result.Action != check.action || result.Source != check.source || result.Platform != "REVIEW" ||
				!slices.Equal(result.RuleHits, []string{"challenge@2", "review@5"}) {
				t.Fatalf("unexpected evaluated decision: %+v", result)
			}
		})
	}

	result := assess(req, []feature{{Kind: "token", Orders: 1}}, false, false)
	applyGovernedPlatformRules(&result, rules)
	if result.Platform != "ALLOW" || len(result.RuleHits) != 0 {
		t.Fatalf("missing signal should not match: %+v", result)
	}
	result = assess(req, observed, false, false)
	result.Platform, result.Action = "BLOCK", "BLOCK"
	applyGovernedPlatformRules(&result, rules)
	if result.Platform != "BLOCK" || result.Action != "BLOCK" {
		t.Fatalf("governed rule weakened an existing platform block: %+v", result)
	}
}

func TestPlatformGovernanceAcceptsExistingDraftIDFormat(t *testing.T) {
	id, err := attemptID()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.SetPathValue("id", id)
	if accepted, ok := platformPathID(httptest.NewRecorder(), request); !ok || accepted != id {
		t.Fatal("existing system draft ID format was rejected")
	}
}

func TestPlatformRulesetVersionPinsActiveRevision(t *testing.T) {
	if platformRulesetVersion(nil) != ruleVersion {
		t.Fatal("unconfigured platform rules must preserve the builtin version")
	}
	before := platformRulesetVersion([]activePlatformRule{{ID: "same-rule", Version: 2}})
	after := platformRulesetVersion([]activePlatformRule{{ID: "same-rule", Version: 4}})
	if before == ruleVersion || after == before || !strings.HasPrefix(after, ruleVersion+"+governed-") {
		t.Fatalf("missing revision lineage: %q, %q", before, after)
	}
}
