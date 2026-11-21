package main

import (
	"context"
	"slices"
)

const merchantRuleProfileVersion = "merchant-rules-v1"

// Single-transaction tools accept account age. The velocity and token signals use
// the event stream from batch Simulation Lab and remain unknown here.
const merchantAccountAgeSignal = "account_age_days"

func merchantUnavailablePlatformSignals() []string {
	var unavailable []string
	for _, item := range ruleSignals {
		if item.ID != merchantAccountAgeSignal {
			unavailable = append(unavailable, item.ID)
		}
	}
	return unavailable
}

func evaluateMerchantPlatformRules(input mockTransactionInput, approved []activePlatformRule) policyDecisionPart {
	part := policyDecisionPart{Action: "ALLOW", ReasonCodes: []string{}}
	stage := merchantPolicyStage(input.Product)
	valueOf := func(signal string) (any, bool) {
		if signal == merchantAccountAgeSignal && input.AccountAgeDays != nil {
			return float64(*input.AccountAgeDays), true
		}
		return nil, false
	}
	check := func(id string, version int64, reasonCode string, rule ruleInput) {
		if !rule.includesStage(stage) {
			return
		}
		matched, available := rule.matchSignalValues(valueOf)
		if !available {
			part.MissingRuleIDs = append(part.MissingRuleIDs, id)
			for _, condition := range rule.Conditions {
				_, available := valueOf(condition.Signal)
				if !available && !slices.Contains(part.MissingSignals, condition.Signal) {
					part.MissingSignals = append(part.MissingSignals, condition.Signal)
				}
			}
		}
		if !matched {
			return
		}
		if part.RuleVersions == nil {
			part.RuleVersions = map[string]int{}
		}
		part.MatchedRuleIDs = append(part.MatchedRuleIDs, id)
		part.RuleVersions[id] = int(version)
		part.ReasonCodes = append(part.ReasonCodes, reasonCode)
		if policyActionRank(rule.Action) > policyActionRank(part.Action) {
			part.Action = rule.Action
		}
	}
	for _, rule := range builtinRules {
		check(rule.ID, rule.Version, rule.Code, rule.ruleInput)
	}
	for _, rule := range approved {
		check(rule.ID, rule.Version, "PLATFORM_RULE_"+rule.Rule.Action, rule.Rule)
	}
	if len(part.MissingRuleIDs) > 0 {
		part.ReasonCodes = append(part.ReasonCodes, "PLATFORM_SIGNAL_UNAVAILABLE")
	}
	return part
}

func evaluateMerchantRuleDecision(ctx context.Context, input mockTransactionInput, platformRules []activePlatformRule, merchantRules []merchantPolicyRule, evidence merchantPolicyEvidence, rules jevRulesSnapshot, jev merchantJev, comparison *merchantBookingComparison) mockTransaction {
	platform := evaluateMerchantPlatformRules(input, platformRules)
	policies := evaluateMerchantPoliciesWithJev(ctx, merchantRules, evidence, jev, input.SimulateTimeout, platform.Action == "BLOCK")
	return resolveMerchantRuleDecision(ctx, input, platform, platformRulesetVersion(platformRules), policies, rules, jev, comparison)
}

func resolveMerchantRuleDecision(ctx context.Context, input mockTransactionInput, platform policyDecisionPart, platformVersion string, policies policyDecisionPart, rules jevRulesSnapshot, jev merchantJev, comparison *merchantBookingComparison) mockTransaction {
	baseReview := platform.Action == "REVIEW" || policies.Action == "REVIEW"
	result := evaluateJevWithoutScenario(ctx, input, rules, baseReview, jev, comparison)
	result.RuleVersion = platformVersion

	jevDecision := policyDecisionPart{Action: result.Outcome, ReasonCodes: []string{}}
	if jevDecision.Action == "LOW_RISK" {
		jevDecision.Action = "ALLOW"
	}
	jevDecision.MatchedRuleIDs = slices.Clone(result.RuleHits)
	jevDecision.ReasonCodes = append(jevDecision.ReasonCodes, "JEV_"+result.JevStatus)
	if len(jevDecision.MatchedRuleIDs) > 0 {
		jevDecision.ReasonCodes = append(jevDecision.ReasonCodes, "BOOKING_REVIEW_RULE")
	}
	if input.AmountMinor == nil {
		jevDecision.MissingSignals = []string{policyAmount}
		switch {
		case comparison != nil && (len(rules.Rules) > 0 || slices.Contains(policies.MissingSignals, policyAmount)):
			jevDecision.Action = "UNRESOLVED"
			result.Reason = "Manual review is required because an applicable rule needs the transaction amount."
		case comparison != nil && comparison.Similarity == "INSUFFICIENT" && result.JevAnswer == "LOW_RISK":
			jevDecision.Action = "UNRESOLVED"
			result.Reason = "Manual review is required because the amount is missing and the bookings could not be compared."
		}
	}
	policy := resolvePolicyDecision(platform, policies)
	policy.ScoringProfileVersion = merchantRuleProfileVersion
	policy.Jev = &jevDecision
	policy.BookingComparison = comparison
	if policyActionRank(jevDecision.Action) > policyActionRank(policy.Final.Action) {
		policy.Final = policyFinal{Action: jevDecision.Action, Source: "JEV"}
	}
	policy.requireReviewForUnresolvedMerchantPolicy()
	result.PolicyDecision = &policy
	result.applyMerchantRuleOutcome()
	return result
}

func (policy *transactionPolicyDecision) requireReviewForUnresolvedMerchantPolicy() {
	if policy.Final.Action != "BLOCK" && slices.Contains(policy.Merchant.ReasonCodes, merchantJevUnresolved) {
		policy.Final = policyFinal{Action: "UNRESOLVED", Source: "MERCHANT_POLICY"}
	}
}

func (result *mockTransaction) applyMerchantRuleOutcome() {
	final := result.PolicyDecision.Final
	switch final.Source {
	case "PLATFORM":
		result.Reason = "A platform rule requires the " + final.Action + " action."
	case "MERCHANT_POLICY":
		result.Reason = "A merchant rule requires the " + final.Action + " action."
		if final.Action == "UNRESOLVED" && slices.Contains(result.PolicyDecision.Merchant.ReasonCodes, merchantJevUnresolved) {
			result.Reason = "A merchant rule could not be resolved with the available evidence and Jev response. Manual review is required."
		}
	}
	switch final.Action {
	case "BLOCK":
		result.Outcome, result.ReviewState = "HIGH_RISK", "NONE"
	case "CHALLENGE":
		result.Outcome, result.ReviewState = "REVIEW", "NONE"
	case "REVIEW", "UNRESOLVED":
		result.Outcome, result.ReviewState = final.Action, "NEEDS_DECISION"
	default:
		result.Outcome, result.ReviewState = "LOW_RISK", "NONE"
	}
}
