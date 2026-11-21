package main

import "slices"

const merchantRiskScoreVersion = "merchant-risk-v1"

type merchantRiskFactor struct {
	Code   string `json:"code"`
	Points int    `json:"points"`
}

type merchantRiskScore struct {
	Version string               `json:"version"`
	Value   *int                 `json:"value"`
	Partial bool                 `json:"partial"`
	Factors []merchantRiskFactor `json:"factors"`
}

func calculateMerchantRiskScore(input mockTransactionInput, result mockTransaction) *merchantRiskScore {
	score := &merchantRiskScore{Version: merchantRiskScoreVersion}
	total := 0
	observed := false
	add := func(code string, points int) {
		total += points
		score.Factors = append(score.Factors, merchantRiskFactor{Code: code, Points: points})
	}
	missing := func(code string) {
		score.Partial = true
		add(code, 0)
	}
	// Each action raises the score to its floor. A later rule or Jev answer
	// can confirm the same evidence without awarding the same points twice.
	floor := func(code string, minimum int) {
		observed = true
		add(code, max(0, minimum-total))
	}
	action := func(source string, part policyDecisionPart) {
		if len(part.MatchedRuleIDs) == 0 {
			return
		}
		switch part.Action {
		case "CHALLENGE":
			floor(source+"_CHALLENGE", 40)
		case "REVIEW":
			floor(source+"_REVIEW", 55)
		case "BLOCK":
			floor(source+"_BLOCK", 85)
		}
	}

	add("BASE", 20)
	switch {
	case input.AccountAgeDays == nil:
		missing("ACCOUNT_AGE_MISSING")
	case *input.AccountAgeDays <= 7:
		observed = true
		add("ACCOUNT_AGE_NEW", 10)
	default:
		observed = true
		add("ACCOUNT_AGE_OBSERVED", 0)
	}
	if input.CustomerEmailDomain == "" {
		missing("EMAIL_DOMAIN_MISSING")
	} else if domains := disposableEmailDomains(); domains != nil {
		observed = true
		if matched, _ := domains.Lookup(input.CustomerEmailDomain); matched != "" {
			add("DISPOSABLE_EMAIL_DOMAIN", 15)
		} else {
			add("EMAIL_DOMAIN_NOT_LISTED", 0)
		}
	} else {
		missing("EMAIL_DOMAIN_UNAVAILABLE")
	}

	policy := result.PolicyDecision
	action("PLATFORM", policy.Platform)
	if len(policy.Platform.MissingRuleIDs) > 0 {
		missing("PLATFORM_EVIDENCE_INCOMPLETE")
	}
	action("MERCHANT", policy.Merchant)
	if len(policy.Merchant.MissingRuleIDs) > 0 || slices.Contains(policy.Merchant.ReasonCodes, merchantJevUnresolved) {
		missing("MERCHANT_EVIDENCE_INCOMPLETE")
	}
	if len(result.RuleHits) > 0 {
		floor("AMOUNT_RULE_REVIEW", 55)
	}
	switch result.JevStatus {
	case "LIVE", "MOCKED":
		switch result.JevAnswer {
		case "REVIEW":
			floor("JEV_"+result.JevStatus+"_REVIEW", 55)
		case "LOW_RISK":
			observed = true
			add("JEV_"+result.JevStatus+"_LOW_RISK", 0)
		default:
			missing("JEV_UNAVAILABLE")
		}
	case "TIMEOUT":
		missing("JEV_TIMEOUT")
	case "NOT_RUN":
		missing("JEV_NOT_RUN")
	default:
		missing("JEV_UNAVAILABLE")
	}
	if observed {
		value := min(total, 100)
		score.Value = &value
	}
	return score
}
