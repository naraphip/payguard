package main

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

type ruleCondition struct {
	Signal   string `json:"signal"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type ruleInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Stage       string          `json:"stage"`
	Action      string          `json:"action"`
	Conditions  []ruleCondition `json:"conditions"`
}

type signal struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Type   string   `json:"type"`
	Stages []string `json:"stages"`
}

var allRuleStages = []string{"ANY", "BOOKING", "PRE_PAYMENT", "CHECKOUT", "PAYMENT", "PAYMENT_STAGES"}
var paymentRuleStages = []string{"PRE_PAYMENT", "PAYMENT", "PAYMENT_STAGES"}

var ruleSignals = []signal{
	{"account_age_days", "Account age (days)", "number", allRuleStages},
	{"device_orders", "Orders on device (60 seconds)", "number", allRuleStages},
	{"ip_orders", "Orders on IP (60 seconds)", "number", allRuleStages},
	{"payment_token_orders", "Orders on payment token (60 seconds)", "number", paymentRuleStages},
	{"payment_token_missing", "Payment token missing", "boolean", paymentRuleStages},
	{"shared_device_or_token_orders", "Highest order count on device or token (60 seconds)", "number", allRuleStages},
}

type systemRule struct {
	ruleInput
	ID        string `json:"id"`
	Status    string `json:"status"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Code      string `json:"code,omitempty"`
}

type builtinRule struct {
	systemRule
	jevAnswer string
}

var builtinRules = []builtinRule{
	{systemRule: systemRule{
		ID: "builtin-shared-device-and-token", Status: "ACTIVE", Version: 1, Code: "SHARED_DEVICE_AND_TOKEN",
		ruleInput: ruleInput{
			Name: "Shared device and payment token", Description: "Reviews payment events with repeated orders on both the device and the payment token.",
			Stage: "PAYMENT_STAGES", Action: "REVIEW", Conditions: []ruleCondition{
				{"device_orders", "gte", float64(2)},
				{"payment_token_orders", "gte", float64(2)},
			},
		},
	}, jevAnswer: "HIGH"},
	{systemRule: systemRule{
		ID: "builtin-new-account-shared-instrument-or-device", Status: "ACTIVE", Version: 1, Code: "NEW_ACCOUNT_SHARED_INSTRUMENT_OR_DEVICE",
		ruleInput: ruleInput{
			Name: "New account with a shared device or token", Description: "Challenges accounts seven days old or younger with repeated orders on the device or payment token.",
			Stage: "ANY", Action: "CHALLENGE", Conditions: []ruleCondition{
				{"shared_device_or_token_orders", "gte", float64(2)},
				{"account_age_days", "lte", float64(7)},
			},
		},
	}, jevAnswer: "MEDIUM"},
	{systemRule: systemRule{
		ID: "builtin-payment-token-missing", Status: "ACTIVE", Version: 1, Code: "PAYMENT_TOKEN_MISSING",
		ruleInput: ruleInput{
			Name: "Missing payment token", Description: "Reviews payment events without a usable payment token signal.",
			Stage: "PAYMENT_STAGES", Action: "REVIEW", Conditions: []ruleCondition{
				{"payment_token_missing", "eq", true},
			},
		},
	}, jevAnswer: "INSUFFICIENT_EVIDENCE"},
}

func (input *ruleInput) validate() error {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 80 {
		return errors.New("Rule name must be 1 to 80 characters.")
	}
	if utf8.RuneCountInString(input.Description) > 500 {
		return errors.New("Rule description must be at most 500 characters.")
	}
	if !slices.Contains(allRuleStages, input.Stage) {
		return errors.New("Choose a valid rule stage.")
	}
	if input.Action != "REVIEW" && input.Action != "CHALLENGE" {
		return errors.New("Rule action must be REVIEW or CHALLENGE.")
	}
	if len(input.Conditions) < 1 || len(input.Conditions) > 5 {
		return errors.New("A rule must have 1 to 5 conditions.")
	}
	for i, condition := range input.Conditions {
		index := slices.IndexFunc(ruleSignals, func(s signal) bool { return s.ID == condition.Signal })
		if index < 0 {
			return fmt.Errorf("Condition %d uses an unknown signal.", i+1)
		}
		available := ruleSignals[index]
		if !slices.Contains(available.Stages, input.Stage) {
			return fmt.Errorf("Condition %d uses a signal unavailable at this stage.", i+1)
		}
		if available.Type == "boolean" {
			if _, ok := condition.Value.(bool); !ok || (condition.Operator != "eq" && condition.Operator != "ne") {
				return fmt.Errorf("Condition %d requires a boolean with eq or ne.", i+1)
			}
			continue
		}
		value, ok := condition.Value.(float64)
		if !ok || math.IsNaN(value) || math.Abs(value) > 9007199254740991 {
			return fmt.Errorf("Condition %d requires a finite number in the supported range.", i+1)
		}
		switch condition.Operator {
		case "gte", "gt", "lte", "lt", "eq", "ne":
		default:
			return fmt.Errorf("Condition %d uses an unsupported numeric operator.", i+1)
		}
	}
	return nil
}

func (input ruleInput) includesStage(stage string) bool {
	return input.Stage == "ANY" || input.Stage == stage || (input.Stage == "PAYMENT_STAGES" && paymentStage(stage))
}

func observedOrders(features []feature, kind string) (float64, bool) {
	for _, f := range features {
		if f.Kind == kind {
			return float64(f.Orders), true
		}
	}
	return 0, false
}

func paymentTokenMissing(e event, features []feature) bool {
	token, _ := observedOrders(features, "token")
	return paymentStage(e.Stage) && token == 0
}

func signalValue(identifier string, e event, features []feature) (any, bool) {
	switch identifier {
	case "account_age_days":
		return float64(e.AgeDays), true
	case "device_orders":
		return observedOrders(features, "device")
	case "ip_orders":
		return observedOrders(features, "ip")
	case "payment_token_orders":
		if paymentStage(e.Stage) {
			return observedOrders(features, "token")
		}
	case "payment_token_missing":
		return paymentTokenMissing(e, features), paymentStage(e.Stage)
	case "shared_device_or_token_orders":
		device, hasDevice := observedOrders(features, "device")
		token, hasToken := observedOrders(features, "token")
		return max(device, token), hasDevice || hasToken
	}
	return nil, false
}

func (condition ruleCondition) matches(actual any) bool {
	switch target := condition.Value.(type) {
	case bool:
		value, ok := actual.(bool)
		return ok && ((condition.Operator == "eq" && value == target) || (condition.Operator == "ne" && value != target))
	case float64:
		value, ok := actual.(float64)
		if !ok {
			return false
		}
		switch condition.Operator {
		case "gte":
			return value >= target
		case "gt":
			return value > target
		case "lte":
			return value <= target
		case "lt":
			return value < target
		case "eq":
			return value == target
		case "ne":
			return value != target
		}
	}
	return false
}

// Match all conditions and track whether all required signals were observed independently of the match.
func (input ruleInput) matchSignals(e event, features []feature) (matched, eligible bool) {
	matched = true
	for _, condition := range input.Conditions {
		value, found := signalValue(condition.Signal, e, features)
		if !found {
			return false, false
		}
		matched = condition.matches(value) && matched
	}
	return matched, true
}

type rulePreview struct {
	RunID           string   `json:"runId"`
	RuleVersion     string   `json:"ruleVersion"`
	Eligible        int      `json:"eligible"`
	Matched         int      `json:"matched"`
	Skipped         int      `json:"skipped"`
	MatchedEventIDs []string `json:"matchedEventIds"`
}

func previewRule(saved run, input ruleInput) rulePreview {
	result := rulePreview{RunID: saved.ID, RuleVersion: saved.RuleVersion, MatchedEventIDs: []string{}}
	for _, d := range saved.Decisions {
		if !input.includesStage(d.Event.Stage) {
			continue
		}
		matched, eligible := input.matchSignals(d.Event, d.Features)
		if !eligible {
			result.Skipped++
			continue
		}
		result.Eligible++
		if matched {
			result.Matched++
			result.MatchedEventIDs = append(result.MatchedEventIDs, d.Event.ID)
		}
	}
	return result
}
