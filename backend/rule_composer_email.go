package main

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const (
	policyDisposableEmail = "customer.email_disposable"
	policyRandomEmail     = "customer.email_local_part_random"
)

var (
	emailRiskMention      = regexp.MustCompile(`(?i)\b(?:random(?:ly generated|[- ]looking)? (?:e-?mails?|mailboxes?|local parts?)|disposable (?:e-?mails?|domains?)|temp(?:orary)?[ -]+(?:e-?mail|mail)s?|e-?mail\s+(?:is\s+)?(?:disposable|random)|local part looks randomly generated)\b|\btempmail(?:\s|$)|อีเมล(?:ดู)?สุ่ม|อีเมลชั่วคราว|ชื่อก่อน\s*@\s*ดูสุ่ม`)
	emailProductSeparator = regexp.MustCompile(`(?i)\s+and\s+|\s*และ\s*|\s*,\s*`)
)

func emailRiskFields(predicate string) []string {
	switch strings.ToLower(strings.Join(strings.Fields(predicate), " ")) {
	case "random email", "random emails", "the email is random", "อีเมลสุ่ม", "เป็นอีเมลสุ่ม":
		return []string{policyDisposableEmail, policyRandomEmail}
	case "disposable email", "disposable emails", "temporary email", "tempmail", "temp mail",
		"email is disposable", "the email is disposable", "อีเมลชั่วคราว", "เป็นอีเมลชั่วคราว":
		return []string{policyDisposableEmail}
	case "random-looking email", "random-looking local part", "random local part",
		"the local part looks randomly generated", "local part looks randomly generated",
		"ชื่อก่อน @ ดูสุ่ม":
		return []string{policyRandomEmail}
	}
	return nil
}

// Interpret recognized email risk on the actual email signals. If some part
// cannot be expressed faithfully, return a clarification instead of a partial rule.
func interpretEmailRiskRule(instruction string, profile merchantPolicyProfile) (composerInterpretation, bool) {
	if !emailRiskMention.MatchString(instruction) {
		return composerInterpretation{}, false
	}
	const example = "Block flight tickets and hotel rooms when the email is disposable or the local part looks randomly generated"
	message := "Specify the products and which email signal to check. For example: " + example + "."
	if strings.ContainsFunc(instruction, func(r rune) bool { return unicode.Is(unicode.Thai, r) }) {
		message = "ระบุสินค้าและเงื่อนไขอีเมล เช่น บล็อกตั๋วเครื่องบินและห้องพักเมื่อเป็นอีเมลชั่วคราวหรือชื่อก่อน @ ดูสุ่ม"
	}
	clarify := composerInterpretation{Status: "CLARIFICATION", Message: message}
	sentence := strings.TrimSpace(strings.TrimSuffix(unquoteRuleText(instruction), "."))
	match := localMerchantSentence.FindStringSubmatch(sentence)
	if match == nil {
		match = localMerchantThai.FindStringSubmatch(sentence)
	}
	if match == nil {
		return clarify, true
	}
	var products []string
	for _, label := range emailProductSeparator.Split(match[2], -1) {
		selected := localComposerProducts(label, profile)
		if len(selected) == 0 {
			return clarify, true
		}
		for _, product := range selected {
			if !slices.Contains(products, product) {
				products = append(products, product)
			}
		}
	}
	predicate := match[3]
	if !strings.ContainsAny(predicate, "\"“") {
		predicate = strings.NewReplacer("หรือ", " หรือ ", "และ", " และ ").Replace(predicate)
	}
	logic, clauses := splitCustomRuleConditions(predicate)
	group := merchantPolicyConditions{Logic: logic}
	for _, clause := range clauses {
		if fields := emailRiskFields(clause); len(fields) > 0 {
			if len(fields) > 1 {
				if len(clauses) > 1 && logic != "OR" {
					return clarify, true
				}
				group.Logic = "OR"
			}
			for _, field := range fields {
				group.Rules = append(group.Rules, merchantPolicyCondition{Field: field, Operator: "eq", Value: json.RawMessage("true")})
			}
			continue
		}
		field, operator, value, _, ok := localComposerCondition(clause)
		if !ok {
			return clarify, true
		}
		group.Rules = append(group.Rules, merchantPolicyCondition{Field: field, Operator: operator, Value: value})
	}
	if !slices.ContainsFunc(group.Rules, func(condition merchantPolicyCondition) bool {
		return condition.Field == policyDisposableEmail || condition.Field == policyRandomEmail
	}) {
		return clarify, true
	}
	action := localComposerActions[strings.ToLower(match[1])]
	return validateLocalRule(merchantPolicyProposal{
		Scope: composerMerchant, Name: localActionTitle(action) + " by email risk",
		ProductTypes: products, Stages: merchantComposerStages(products), Conditions: group, Action: action,
	}, profile), true
}

func emailRiskProposalHasSignal(proposal merchantPolicyProposal) bool {
	return slices.ContainsFunc(proposal.Conditions.Rules, func(condition merchantPolicyCondition) bool {
		switch condition.Field {
		case policyDisposableEmail, policyRandomEmail:
			return true
		}
		return false
	})
}

func composerPreservesEmailRisk(instruction string, profile merchantPolicyProfile, output composerInterpretation) bool {
	proposal, ready := output.Proposal.(merchantPolicyProposal)
	if !ready || !emailRiskMention.MatchString(instruction) {
		return true
	}
	if !emailRiskProposalHasSignal(proposal) {
		return false
	}
	// If the local interpreter can resolve the complete email policy, check
	// that the model preserved the product scope and the requested conditions.
	local, _ := interpretEmailRiskRule(instruction, profile)
	expected, ready := local.Proposal.(merchantPolicyProposal)
	if !ready {
		return true
	}
	if proposal.Action != expected.Action ||
		!slices.Equal(sortedPolicyValues(proposal.ProductTypes), sortedPolicyValues(expected.ProductTypes)) ||
		len(proposal.Conditions.Rules) != len(expected.Conditions.Rules) ||
		(len(expected.Conditions.Rules) > 1 && proposal.Conditions.Logic != expected.Conditions.Logic) {
		return false
	}
	for _, wanted := range expected.Conditions.Rules {
		if !slices.ContainsFunc(proposal.Conditions.Rules, func(condition merchantPolicyCondition) bool {
			return condition.Field == wanted.Field && condition.Operator == wanted.Operator && bytes.Equal(condition.Value, wanted.Value)
		}) {
			return false
		}
	}
	return true
}
