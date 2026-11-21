package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	localMerchantSentence = regexp.MustCompile(`(?i)^(block|review|challenge) (.+?) when (.+)$`)
	localMerchantThai     = regexp.MustCompile(`^(บล็อก|ส่งตรวจสอบ|ตรวจสอบ|ยืนยันเพิ่มเติม)\s*(.+?)\s*เมื่อ\s*(.+)$`)
	localPolicyAmount     = regexp.MustCompile(`(?i)^amount (?:is )?(at least|at most|greater than|less than|equals|does not equal|>=|<=|>|<|=|!=) ?([0-9]+(?:\.[0-9]{1,2})?) (minor units?|satang|THB|baht)$`)
	localPolicyThaiAmount = regexp.MustCompile(`(?i)^ยอด(?:เงิน)?\s*(อย่างน้อย|ไม่เกิน|มากกว่า|น้อยกว่า|เท่ากับ|ไม่เท่ากับ|>=|<=|>|<|=|!=)\s*([0-9]+(?:\.[0-9]{1,2})?)\s*(สตางค์|บาท|THB)$`)
	localPolicyDomain     = regexp.MustCompile(`(?i)^(?:email domain equals |โดเมนอีเมล\s*(?:คือ|เท่ากับ)\s*)(\*?@?[\pL\pM\pN.-]+)$`)
	localPolicyFlight     = regexp.MustCompile(`(?i)^if a customer books a flight ticket using an email address from (\*?@?[\pL\pM\pN.-]+), block the booking$`)
	localPolicySemantic   = regexp.MustCompile(`(?i)^(transaction description|transaction\.description|รายละเอียดธุรกรรม|custom_[a-z][a-z0-9_]*)\s*(?: means |\s*หมายถึง\s*|\s*สื่อถึง\s*)"([^"\r\n]+)"$`)
)

var localComposerActions = map[string]string{
	"block": "BLOCK", "review": "REVIEW", "challenge": "CHALLENGE",
	"บล็อก": "BLOCK", "ส่งตรวจสอบ": "REVIEW", "ตรวจสอบ": "REVIEW", "ยืนยันเพิ่มเติม": "CHALLENGE",
}

var localComposerComparisons = map[string]string{
	"at least": "gte", "at most": "lte", "greater than": "gt", "less than": "lt", "equals": "eq", "does not equal": "ne",
	"อย่างน้อย": "gte", "ไม่เกิน": "lte", "มากกว่า": "gt", "น้อยกว่า": "lt", "เท่ากับ": "eq", "ไม่เท่ากับ": "ne",
	">=": "gte", "<=": "lte", ">": "gt", "<": "lt", "=": "eq", "!=": "ne",
}

func localComposerClarification(profile merchantPolicyProfile) composerInterpretation {
	result := composerInterpretation{
		Status:  "CLARIFICATION",
		Message: "Specify one action (Block, Review or Challenge) and a condition. Include the unit for any payment amount.",
	}
	for _, capability := range profile.FieldRegistry {
		if capability.Name == policyDescription && slices.Contains(capability.Operators, semanticOperator) {
			result.Message += " For a meaning-based rule, choose the transaction text Jev should compare."
			result.SemanticDraft = &composerSemanticDraft{Field: capability.Name}
			break
		}
	}
	return result
}

// Recognize complete, explicit sentences that fit the merchant's field catalogue.
func interpretLocalMerchantRule(instruction string, profile merchantPolicyProfile) (composerInterpretation, bool) {
	if strings.HasPrefix(instruction, "{") {
		var proposal merchantPolicyProposal
		if !validComposerJSON([]byte(instruction), &proposal) {
			return composerInterpretation{
				Status: "CLARIFICATION", Message: "Paste one complete AI Rule JSON object or use the rule fields.",
			}, true
		}
		if proposal.DurationHours != 0 || proposal.DurationMinutes != 0 || slices.ContainsFunc(proposal.Conditions.Rules, func(condition merchantPolicyCondition) bool {
			return condition.Field == policyMailbox
		}) {
			return composerInterpretation{
				Status: "CLARIFICATION", Message: "Use the temporary exact-email block format for a mailbox rule.",
			}, true
		}
		return validateLocalRule(proposal, profile), true
	}

	sentence := strings.TrimSpace(strings.TrimSuffix(instruction, "."))
	match := localMerchantSentence.FindStringSubmatch(sentence)
	if match == nil {
		match = localMerchantThai.FindStringSubmatch(sentence)
	}
	if match == nil {
		if flight := localPolicyFlight.FindStringSubmatch(sentence); flight != nil {
			match = []string{sentence, "block", "flight ticket", "email domain equals " + flight[1]}
		} else {
			return composerInterpretation{}, false
		}
	}
	products := localComposerProducts(match[2], profile)
	if len(products) == 0 {
		return composerInterpretation{}, false
	}

	field, operator, value, label, ok := localComposerCondition(match[3])
	if !ok || !composerConditionAvailable(profile, products, field, operator) {
		return composerInterpretation{}, false
	}
	condition := merchantPolicyCondition{Field: field, Operator: operator, Value: value}
	action := localComposerActions[strings.ToLower(match[1])]
	digest := sha256.Sum256([]byte(strings.Join(products, ",") + "|" + field + "|" + operator + "|" + string(value)))
	return validateLocalRule(merchantPolicyProposal{
		Name:  fmt.Sprintf("%s by %s (%x)", localActionTitle(action), label, digest[:4]),
		Scope: composerMerchant, ProductTypes: products, Stages: merchantComposerStages(products), Action: action,
		Conditions: merchantPolicyConditions{Logic: "AND", Rules: []merchantPolicyCondition{condition}},
	}, profile), true
}

func merchantComposerStages(products []string) []string {
	stages := make([]string, 0, len(products))
	for _, product := range products {
		if stage := merchantPolicyStage(product); !slices.Contains(stages, stage) {
			stages = append(stages, stage)
		}
	}
	return stages
}

func localComposerProducts(raw string, profile merchantPolicyProfile) []string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "all products", "สินค้าทั้งหมด", "ทุกสินค้า":
		return slices.Clone(profile.Products)
	case "bookings", "การจองทั้งหมด":
		return slices.DeleteFunc(slices.Clone(profile.Products), func(product string) bool {
			return merchantPolicyStage(product) != "BOOKING"
		})
	case "flight ticket", "flight tickets", "flight_ticket", "ตั๋วเครื่องบิน":
		return []string{"flight_ticket"}
	case "hotel room", "hotel rooms", "hotel_room", "ห้องพัก":
		return []string{"hotel_room"}
	case "physical goods", "physical", "สินค้าทั่วไป":
		return []string{"physical"}
	case "digital goods", "digital", "สินค้าดิจิทัล":
		return []string{"digital"}
	case "gateway payment", "gateway payments", "payment_gateway", "เกตเวย์ชำระเงิน":
		return []string{"payment_gateway"}
	}
	return nil
}

func localComposerCondition(raw string) (field, operator string, value json.RawMessage, label string, ok bool) {
	amount := localPolicyAmount.FindStringSubmatch(raw)
	if amount == nil {
		amount = localPolicyThaiAmount.FindStringSubmatch(raw)
	}
	if amount != nil {
		minor, valid := localComposerMinorAmount(amount[2], amount[3])
		if !valid {
			return "", "", nil, "", false
		}
		return policyAmount, localComposerComparisons[strings.ToLower(amount[1])], json.RawMessage(strconv.FormatInt(minor, 10)), "amount", true
	}
	if domain := localPolicyDomain.FindStringSubmatch(raw); domain != nil {
		host := strings.TrimPrefix(strings.TrimPrefix(domain[1], "*"), "@")
		value, _ := json.Marshal(host)
		return policyDomain, "eq", value, "email domain", true
	}
	if match := localPolicySemantic.FindStringSubmatch(raw); match != nil {
		field := strings.ToLower(match[1])
		if !strings.HasPrefix(field, "custom_") {
			field = policyDescription
		}
		value, _ := json.Marshal(strings.TrimSpace(match[2]))
		return field, semanticOperator, value, "meaning", true
	}
	return "", "", nil, "", false
}

func localComposerMinorAmount(raw, unit string) (int64, bool) {
	whole, fractional, decimal := strings.Cut(raw, ".")
	value, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || value > jevRuleMaxValue {
		return 0, false
	}
	switch strings.ToLower(unit) {
	case "thb", "baht", "บาท":
		if value > jevRuleMaxValue/100 {
			return 0, false
		}
		cents, _ := strconv.ParseInt((fractional + "00")[:2], 10, 64)
		value = value*100 + cents
	default:
		if decimal {
			return 0, false
		}
	}
	return value, validDemoAmount(&value)
}

func localActionTitle(action string) string {
	return strings.ToUpper(action[:1]) + strings.ToLower(action[1:])
}

func validateLocalRule(proposal merchantPolicyProposal, profile merchantPolicyProfile) composerInterpretation {
	if err := proposal.validate(profile); err != nil {
		return composerInterpretation{Status: "CLARIFICATION", Validation: composerValidation{Errors: []string{err.Error()}}}
	}
	return composerInterpretation{Status: "READY", Validation: composerValidation{Valid: true}, Proposal: proposal}
}
