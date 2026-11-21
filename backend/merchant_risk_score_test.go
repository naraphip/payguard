package main

import (
	"slices"
	"testing"
)

func TestMerchantRiskScoreObservedSignals(t *testing.T) {
	age := int64(7)
	input := mockTransactionInput{AccountAgeDays: &age, CustomerEmailDomain: "mailinator.com"}
	result := mockTransaction{PolicyDecision: &transactionPolicyDecision{}, JevStatus: "LIVE", JevAnswer: "LOW_RISK"}
	score := calculateMerchantRiskScore(input, result, false)
	if score.Value == nil || *score.Value != 45 || score.Partial || score.Version != merchantRiskScoreVersion {
		t.Fatalf("score with observed account age and disposable domain: %+v", score)
	}
	want := []merchantRiskFactor{
		{Code: "BASE", Points: 20},
		{Code: "ACCOUNT_AGE_NEW", Points: 10},
		{Code: "DISPOSABLE_EMAIL_DOMAIN", Points: 15},
		{Code: "JEV_LIVE_LOW_RISK", Points: 0},
	}
	if !slices.Equal(score.Factors, want) {
		t.Fatalf("score factors = %v, want %v", score.Factors, want)
	}
	age++
	input.CustomerEmailDomain = "gmail.com"
	score = calculateMerchantRiskScore(input, result, false)
	if score.Value == nil || *score.Value != 20 || score.Partial ||
		!slices.Contains(score.Factors, merchantRiskFactor{Code: "EMAIL_DOMAIN_NOT_LISTED"}) {
		t.Fatalf("a non-listed domain must be neutral: %+v", score)
	}
}

func TestMerchantRiskScoreRuleFloorsAndJev(t *testing.T) {
	age := int64(0)
	input := mockTransactionInput{AccountAgeDays: &age, CustomerEmailDomain: "mailinator.com"}
	result := mockTransaction{
		Outcome: "HIGH_RISK", RuleHits: []string{"amount-1", "amount-2"}, JevStatus: "LIVE", JevAnswer: "REVIEW",
		PolicyDecision: &transactionPolicyDecision{
			Platform: policyDecisionPart{Action: "CHALLENGE", MatchedRuleIDs: []string{"platform-1"}},
			Merchant: policyDecisionPart{Action: "BLOCK", MatchedRuleIDs: []string{"merchant-1"}},
		},
	}
	score := calculateMerchantRiskScore(input, result, false)
	for _, factor := range []merchantRiskFactor{
		{Code: "PLATFORM_CHALLENGE"},
		{Code: "MERCHANT_BLOCK", Points: 40},
		{Code: "AMOUNT_RULE_REVIEW"},
		{Code: "JEV_LIVE_REVIEW"},
	} {
		if !slices.Contains(score.Factors, factor) {
			t.Fatalf("missing factor %v in %v", factor, score.Factors)
		}
	}
	if score.Value == nil || *score.Value != 85 || score.Partial {
		t.Fatalf("strongest rule did not determine the floor: %+v", score)
	}
	result.PolicyDecision = &transactionPolicyDecision{}
	result.RuleHits = nil
	for _, status := range []string{"LIVE", "MOCKED"} {
		result.JevStatus = status
		score = calculateMerchantRiskScore(input, result, false)
		if score.Value == nil || *score.Value != 55 || score.Partial ||
			!slices.Contains(score.Factors, merchantRiskFactor{Code: "JEV_" + status + "_REVIEW", Points: 10}) {
			t.Fatalf("%s review did not contribute its floor: %+v", status, score)
		}
	}
}

func TestMerchantRiskScoreIncompleteEvidence(t *testing.T) {
	result := mockTransaction{PolicyDecision: &transactionPolicyDecision{}, JevStatus: "TIMEOUT", JevAnswer: "LOW_RISK"}
	missing := calculateMerchantRiskScore(mockTransactionInput{}, result, false)
	if missing.Value != nil || !missing.Partial ||
		!slices.Contains(missing.Factors, merchantRiskFactor{Code: "JEV_TIMEOUT"}) {
		t.Fatalf("an unanswered transaction without evidence had a numerical score: %+v", missing)
	}
	result.PolicyDecision.Merchant.ReasonCodes = []string{merchantJevUnresolved}
	score := calculateMerchantRiskScore(mockTransactionInput{CustomerEmailDomain: "mailinator.com"}, result, false)
	if score.Value == nil || *score.Value != 35 || !score.Partial ||
		!slices.Contains(score.Factors, merchantRiskFactor{Code: "MERCHANT_EVIDENCE_INCOMPLETE"}) {
		t.Fatalf("available evidence should be scored and marked partial: %+v", score)
	}
}
