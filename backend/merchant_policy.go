package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

const (
	policyDomain                   = "customer.email_domain"
	policyAmount                   = "order.amount_minor"
	policyProduct                  = "product.type"
	merchantScenarioProfileVersion = "merchant-scenario-amount-v1"
)

type merchantPolicyProfile struct {
	BusinessName          string               `json:"businessName"`
	BusinessModel         string               `json:"businessModel,omitempty"`
	Products              []string             `json:"products"`
	Stages                []string             `json:"stages"`
	Signals               []string             `json:"signals"`
	FieldRegistry         []merchantCapability `json:"fieldRegistry,omitempty"`
	ScoringProfileVersion string               `json:"scoringProfileVersion,omitempty"`
}

type merchantPolicyCondition struct {
	Field    string          `json:"field"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value"`
}

type merchantPolicyConditions struct {
	Logic string                    `json:"logic"`
	Rules []merchantPolicyCondition `json:"rules"`
}

type merchantPolicyProposal struct {
	Scope        string                   `json:"scope"`
	Name         string                   `json:"name"`
	ProductTypes []string                 `json:"product_types"`
	Stages       []string                 `json:"stages"`
	Conditions   merchantPolicyConditions `json:"conditions"`
	Action       string                   `json:"action"`
}

type policyField struct {
	DataType           string   `json:"dataType"`
	Operators          []string `json:"allowedOperators"`
	DataSource         string   `json:"dataSource"`
	Privacy            string   `json:"privacyClassification"`
	MissingValuePolicy string   `json:"missingValuePolicy"`
}

type merchantCapability struct {
	Name string `json:"fieldName"`
	policyField
	Products []string `json:"supportedProductTypes"`
	Stages   []string `json:"availableStages"`
}

var merchantPolicyFields = map[string]policyField{
	policyDomain:  {"string", []string{"eq"}, "merchant transaction email domain", "normalized-domain", "no_match"},
	policyAmount:  {"number", []string{"eq", "ne", "gt", "gte", "lt", "lte"}, "merchant transaction amount", "transaction-amount", "no_match"},
	policyProduct: {"enum", []string{"eq"}, "merchant transaction product", "catalog-metadata", "no_match"},
}

// Only stages that the merchant transaction intake actually evaluates belong here.
func merchantPolicyStage(product string) string {
	if module, ok := simulationProducts[product]; ok {
		return module.firstStage
	}
	return ""
}

func normalizePolicyDomain(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 1024 {
		return "", errors.New("Enter an email domain such as example.com.")
	}
	domain, err := idna.Lookup.ToASCII(raw)
	if err == nil {
		domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	}
	if err != nil || len(domain) > 253 || !strings.Contains(domain, ".") {
		return "", errors.New("Enter a valid email domain without a username or wildcard.")
	}
	if _, err := netip.ParseAddr(domain); err == nil {
		return "", errors.New("Enter an email domain rather than an IP address.")
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' ||
			strings.IndexFunc(label, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') }) >= 0 {
			return "", errors.New("Enter a valid email domain without a username or wildcard.")
		}
	}
	return domain, nil
}

func policyList(input, available []string, label string) error {
	if len(input) == 0 || len(input) > len(available) {
		return fmt.Errorf("Choose at least one supported %s from the merchant profile.", label)
	}
	seen := map[string]bool{}
	for _, value := range input {
		if seen[value] || !slices.Contains(available, value) {
			return fmt.Errorf("Unsupported or duplicate %s: %s.", label, value)
		}
		seen[value] = true
	}
	return nil
}

func (p *merchantPolicyProposal) validate(profile merchantPolicyProfile) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Scope != "MERCHANT" || p.Name == "" || utf8.RuneCountInString(p.Name) > 80 || strings.IndexFunc(p.Name, unicode.IsControl) >= 0 {
		return errors.New("A Merchant Policy requires a name of 1 to 80 characters.")
	}
	if !slices.Contains([]string{"BLOCK", "REVIEW", "CHALLENGE"}, p.Action) {
		return errors.New("Merchant Policy actions are BLOCK, REVIEW, and CHALLENGE.")
	}
	if err := policyList(p.ProductTypes, profile.Products, "product"); err != nil {
		return err
	}
	if err := policyList(p.Stages, profile.Stages, "stage"); err != nil {
		return err
	}
	expectedStages := make([]string, 0, len(p.ProductTypes))
	for _, product := range p.ProductTypes {
		stage := merchantPolicyStage(product)
		if !slices.Contains(expectedStages, stage) {
			expectedStages = append(expectedStages, stage)
		}
	}
	if len(expectedStages) != len(p.Stages) || !slices.Equal(sortedPolicyValues(expectedStages), sortedPolicyValues(p.Stages)) {
		return errors.New("The selected products must match their supported booking or checkout stages.")
	}
	if (p.Conditions.Logic != "AND" && p.Conditions.Logic != "OR") || len(p.Conditions.Rules) < 1 || len(p.Conditions.Rules) > 5 {
		return errors.New("Use an AND or OR group containing 1 to 5 conditions.")
	}
	for i := range p.Conditions.Rules {
		condition := &p.Conditions.Rules[i]
		field, ok := merchantPolicyFields[condition.Field]
		if !ok || !slices.Contains(profile.Signals, condition.Field) {
			return fmt.Errorf("Required signal unavailable: %s.", condition.Field)
		}
		if !slices.Contains(field.Operators, condition.Operator) {
			return fmt.Errorf("Operator %s is unsupported for %s.", condition.Operator, condition.Field)
		}
		for _, available := range profile.FieldRegistry {
			if available.Name != condition.Field {
				continue
			}
			for _, product := range p.ProductTypes {
				if !slices.Contains(available.Products, product) {
					return fmt.Errorf("Required signal unavailable: %s for %s.", condition.Field, product)
				}
			}
		}
		switch field.DataType {
		case "number":
			amount, err := strconv.ParseInt(strings.TrimSpace(string(condition.Value)), 10, 64)
			if err != nil || !validDemoAmount(&amount) {
				return errors.New("Order amount must be a nonnegative whole number in minor units.")
			}
			condition.Value = json.RawMessage(strconv.FormatInt(amount, 10))
		case "string", "enum":
			var value string
			if err := json.Unmarshal(condition.Value, &value); err != nil || value == "" {
				return fmt.Errorf("A text value is required for %s.", condition.Field)
			}
			if condition.Field == policyDomain {
				domain, err := normalizePolicyDomain(value)
				if err != nil {
					return err
				}
				value = domain
			}
			if condition.Field == policyProduct && !slices.Contains(p.ProductTypes, value) {
				return errors.New("A product condition must refer to a selected product.")
			}
			condition.Value, _ = json.Marshal(value)
		}
	}
	p.ProductTypes = sortedPolicyValues(p.ProductTypes)
	p.Stages = sortedPolicyValues(p.Stages)
	slices.SortFunc(p.Conditions.Rules, func(a, b merchantPolicyCondition) int {
		if diff := strings.Compare(a.Field, b.Field); diff != 0 {
			return diff
		}
		if diff := strings.Compare(a.Operator, b.Operator); diff != 0 {
			return diff
		}
		return bytes.Compare(a.Value, b.Value)
	})
	p.Conditions.Rules = slices.CompactFunc(p.Conditions.Rules, func(a, b merchantPolicyCondition) bool {
		return a.Field == b.Field && a.Operator == b.Operator && bytes.Equal(a.Value, b.Value)
	})
	return nil
}

func sortedPolicyValues(values []string) []string {
	copyValues := slices.Clone(values)
	slices.Sort(copyValues)
	return copyValues
}

type merchantPolicyEvidence struct {
	product string
	stage   string
	domain  string
	amount  *int64
}

func (p merchantPolicyProposal) evaluate(evidence merchantPolicyEvidence) (matched, missing bool) {
	if !slices.Contains(p.ProductTypes, evidence.product) || !slices.Contains(p.Stages, evidence.stage) {
		return false, false
	}
	all, any := true, false
	for _, condition := range p.Conditions.Rules {
		match, unavailable := condition.evaluate(evidence)
		missing = missing || unavailable
		all = all && match
		any = any || match
	}
	if p.Conditions.Logic == "OR" {
		return any, missing && !any
	}
	return all, missing
}

func (condition merchantPolicyCondition) evaluate(evidence merchantPolicyEvidence) (bool, bool) {
	switch condition.Field {
	case policyProduct, policyDomain:
		actual := evidence.product
		if condition.Field == policyDomain {
			actual = evidence.domain
		}
		if actual == "" {
			return false, true
		}
		var expected string
		return json.Unmarshal(condition.Value, &expected) == nil && condition.Operator == "eq" && actual == expected, false
	case policyAmount:
		if evidence.amount == nil {
			return false, true
		}
		expected, err := strconv.ParseInt(string(condition.Value), 10, 64)
		if err != nil {
			return false, false
		}
		actual := *evidence.amount
		switch condition.Operator {
		case "eq":
			return actual == expected, false
		case "ne":
			return actual != expected, false
		case "gt":
			return actual > expected, false
		case "gte":
			return actual >= expected, false
		case "lt":
			return actual < expected, false
		case "lte":
			return actual <= expected, false
		}
	}
	return false, false
}

type policyDecisionPart struct {
	Action         string         `json:"action"`
	MatchedRuleIDs []string       `json:"matchedRuleIds,omitempty"`
	MissingRuleIDs []string       `json:"missingRuleIds,omitempty"`
	MissingSignals []string       `json:"missingSignals,omitempty"`
	ReasonCodes    []string       `json:"reasonCodes"`
	RuleVersions   map[string]int `json:"ruleVersions,omitempty"`
}

type policyFinal struct {
	Action string `json:"action"`
	Source string `json:"source"`
}

type transactionPolicyDecision struct {
	Platform              policyDecisionPart `json:"platformDecision"`
	Merchant              policyDecisionPart `json:"merchantPolicyDecision"`
	Final                 policyFinal        `json:"finalDecision"`
	ScoringProfileVersion string             `json:"scoringProfileVersion,omitempty"`
}

func policyActionRank(action string) int {
	switch action {
	case "BLOCK":
		return 4
	case "REVIEW":
		return 3
	case "CHALLENGE":
		return 2
	case "UNRESOLVED", "UNSUCCESSFUL":
		return 1
	default:
		return 0
	}
}

func resolvePolicyDecision(platform, merchant policyDecisionPart) transactionPolicyDecision {
	final := policyFinal{Action: platform.Action, Source: "PLATFORM"}
	if platform.Action == "ALLOW" && policyActionRank(merchant.Action) == 0 {
		final.Source = "NONE"
	}
	if policyActionRank(merchant.Action) > policyActionRank(platform.Action) {
		final = policyFinal{Action: merchant.Action, Source: "MERCHANT_POLICY"}
	}
	return transactionPolicyDecision{Platform: platform, Merchant: merchant, Final: final}
}

func evaluateMerchantPolicies(rules []merchantPolicyRule, evidence merchantPolicyEvidence) policyDecisionPart {
	part := policyDecisionPart{Action: "NOT_APPLICABLE", ReasonCodes: []string{}}
	for _, rule := range rules {
		matched, missing := rule.Proposal.evaluate(evidence)
		if missing {
			part.MissingRuleIDs = append(part.MissingRuleIDs, rule.ID)
			for _, condition := range rule.Proposal.Conditions.Rules {
				_, unavailable := condition.evaluate(evidence)
				if unavailable && !slices.Contains(part.MissingSignals, condition.Field) {
					part.MissingSignals = append(part.MissingSignals, condition.Field)
				}
			}
		}
		if !matched {
			continue
		}
		if part.RuleVersions == nil {
			part.RuleVersions = make(map[string]int)
		}
		part.MatchedRuleIDs = append(part.MatchedRuleIDs, rule.ID)
		part.RuleVersions[rule.ID] = rule.Version
		part.ReasonCodes = append(part.ReasonCodes, "MERCHANT_POLICY_"+rule.Proposal.Action)
		if policyActionRank(rule.Proposal.Action) > policyActionRank(part.Action) {
			part.Action = rule.Proposal.Action
		}
	}
	if part.Action == "NOT_APPLICABLE" && len(part.MissingSignals) > 0 {
		part.Action = "UNAVAILABLE"
		part.ReasonCodes = append(part.ReasonCodes, "MERCHANT_POLICY_SIGNAL_UNAVAILABLE")
	}
	return part
}
