package main

import (
	"encoding/json"
	"testing"
)

func exampleMerchantPolicy() merchantPolicyProposal {
	return merchantPolicyProposal{
		Scope: "MERCHANT", Name: "Restricted flight email", ProductTypes: []string{"flight_ticket"},
		Stages: []string{"BOOKING"}, Action: "BLOCK", Conditions: merchantPolicyConditions{
			Logic: "AND", Rules: []merchantPolicyCondition{
				{Field: policyDomain, Operator: "eq", Value: json.RawMessage(`"SKYCANNNER.COM"`)},
			},
		},
	}
}

func exampleMerchantPolicyProfile() merchantPolicyProfile {
	return merchantPolicyProfile{Products: []string{"flight_ticket", "hotel_room"}, Stages: []string{"BOOKING"},
		Signals: []string{policyProduct, policyDomain, policyAmount}}
}

func TestMerchantPolicyExactDomain(t *testing.T) {
	policy := exampleMerchantPolicy()
	if err := policy.validate(exampleMerchantPolicyProfile()); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, domain, product, stage string
		matched, missing             bool
	}{
		{"exact", "skycannner.com", "flight_ticket", "BOOKING", true, false},
		{"case", "SKYCANNNER.COM", "flight_ticket", "BOOKING", true, false},
		{"subdomain", "sub.skycannner.com", "flight_ticket", "BOOKING", false, false},
		{"spoofedSuffix", "skycannner.com.evil.test", "flight_ticket", "BOOKING", false, false},
		{"spoofedPrefix", "not-skycannner.com", "flight_ticket", "BOOKING", false, false},
		{"wrongStage", "skycannner.com", "flight_ticket", "PRE_PAYMENT", false, false},
		{"wrongProduct", "skycannner.com", "hotel_room", "BOOKING", false, false},
		{"missing", "", "flight_ticket", "BOOKING", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := ""
			if tt.domain != "" {
				var err error
				domain, err = normalizePolicyDomain(tt.domain)
				if err != nil {
					t.Fatal(err)
				}
			}
			matched, missing := policy.evaluate(merchantPolicyEvidence{product: tt.product, stage: tt.stage, domain: domain})
			if matched != tt.matched || missing != tt.missing {
				t.Fatalf("matched=%v missing=%v", matched, missing)
			}
		})
	}
	if domain, err := normalizePolicyDomain("BÜCHER.de"); err != nil || domain != "xn--bcher-kva.de" {
		t.Fatalf("IDNA domain=%q err=%v", domain, err)
	}
	for _, invalid := range []string{"*@skycannner.com", "user@skycannner.com", "not-a-domain", "address.test/evil"} {
		if _, err := normalizePolicyDomain(invalid); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestMerchantPolicyValidationAndMissingAmount(t *testing.T) {
	profile := exampleMerchantPolicyProfile()
	profile.Signals = []string{policyProduct, policyAmount}
	p := exampleMerchantPolicy()
	if err := p.validate(profile); err == nil {
		t.Fatal("accepted unavailable email")
	}
	p.Conditions.Rules = []merchantPolicyCondition{{Field: policyAmount, Operator: "eq", Value: json.RawMessage("0")}}
	if err := p.validate(profile); err != nil {
		t.Fatal(err)
	}
	item := merchantPolicyEvidence{product: "flight_ticket", stage: "BOOKING"}
	if matched, missing := p.evaluate(item); matched || !missing {
		t.Fatal("unavailable amount matched")
	}
	zero := int64(0)
	item.amount = &zero
	if matched, missing := p.evaluate(item); !matched || missing {
		t.Fatal("observed zero did not match")
	}
	p.Conditions.Rules[0].Operator = "exec"
	if err := p.validate(profile); err == nil {
		t.Fatal("accepted arbitrary operator")
	}
}

func TestMerchantPolicyDecisionPrecedence(t *testing.T) {
	for _, tt := range []struct{ platform, merchant, action, source string }{
		{"ALLOW", "BLOCK", "BLOCK", "MERCHANT_POLICY"},
		{"BLOCK", "ALLOW_EXCEPTION", "BLOCK", "PLATFORM"},
		{"BLOCK", "BLOCK", "BLOCK", "PLATFORM"},
		{"REVIEW", "NOT_APPLICABLE", "REVIEW", "PLATFORM"},
		{"ALLOW", "NOT_APPLICABLE", "ALLOW", "NONE"},
	} {
		t.Run(tt.platform+"_"+tt.merchant, func(t *testing.T) {
			decision := resolvePolicyDecision(policyDecisionPart{Action: tt.platform}, policyDecisionPart{Action: tt.merchant})
			if decision.Final.Action != tt.action || decision.Final.Source != tt.source {
				t.Fatalf("got %+v", decision.Final)
			}
		})
	}
}

func BenchmarkMerchantPolicyEvaluation(b *testing.B) {
	p := exampleMerchantPolicy()
	if err := p.validate(exampleMerchantPolicyProfile()); err != nil {
		b.Fatal(err)
	}
	rules := []merchantPolicyRule{{ID: "test-rule", Version: 1, Proposal: p}}
	item := merchantPolicyEvidence{product: "flight_ticket", stage: "BOOKING", domain: "skycannner.com"}
	b.ReportAllocs()
	for b.Loop() {
		_ = evaluateMerchantPolicies(rules, item)
	}
}

func TestPolicySnapshotKeepsMissingSignals(t *testing.T) {
	proposal := exampleMerchantPolicy()
	if err := proposal.validate(exampleMerchantPolicyProfile()); err != nil {
		t.Fatal(err)
	}
	rules := []merchantPolicyRule{{ID: "email", Version: 1, Proposal: proposal}}
	result := evaluateMerchantPolicies(rules, merchantPolicyEvidence{product: "flight_ticket", stage: "BOOKING"})
	if result.Action != "UNAVAILABLE" || len(result.MissingRuleIDs) != 1 ||
		len(result.MissingSignals) != 1 || result.MissingSignals[0] != policyDomain || len(result.MatchedRuleIDs) != 0 {
		t.Fatalf("missing evidence was not preserved: %+v", result)
	}
}
