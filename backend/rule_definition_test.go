package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestRuleDefinitionByScope(t *testing.T) {
	for _, sample := range []struct {
		scope, input, field, value string
		profile                    merchantPolicyProfile
	}{
		{composerPlatform, validPlatformComposition, "device_orders", "3", merchantPolicyProfile{}},
		{composerMerchant, validThaiComposition, policyDomain, `"example.test"`, merchantPolicyProfile{
			Products: []string{"flight_ticket"}, Stages: []string{"BOOKING"}, Signals: []string{policyDomain, policyProduct},
		}},
	} {
		t.Run(sample.scope, func(t *testing.T) {
			parsed, err := parseComposerInterpretation([]byte(sample.input), sample.scope, sample.profile)
			if err != nil || !parsed.Validation.Valid {
				t.Fatalf("parse result: %+v, %v", parsed, err)
			}
			response := httptest.NewRecorder()
			respondRuleInterpretation(response, parsed)
			var result composerInterpretation
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			definition := result.Definition
			if result.Proposal == nil || definition == nil || definition.SchemaVersion != 1 ||
				definition.Scope != sample.scope || definition.Conditions.Logic != "AND" || len(definition.Conditions.Rules) != 1 {
				t.Fatalf("missing compatible definition: %s", response.Body.String())
			}
			condition := definition.Conditions.Rules[0]
			value, _ := json.Marshal(condition.Value)
			if condition.Field != sample.field || string(value) != sample.value {
				t.Fatalf("unexpected condition: %+v", condition)
			}
			if sample.scope == composerPlatform && (len(definition.ProductTypes) != 0 || definition.Stages[0] != "PAYMENT" ||
				definition.Description != "Review payment orders on a reused device.") {
				t.Fatalf("unexpected platform scope: %+v", definition)
			}
			if sample.scope == composerMerchant && (len(definition.ProductTypes) != 1 || definition.ProductTypes[0] != "flight_ticket" ||
				definition.Stages[0] != "BOOKING") {
				t.Fatalf("unexpected merchant scope: %+v", definition)
			}
		})
	}
}

func TestRuleDefinitionPreservesTypesAndMasksMailbox(t *testing.T) {
	proposal := merchantPolicyProposal{
		Name: "Private rule", Scope: composerMerchant, Action: "BLOCK", DurationMinutes: 2,
		Conditions: merchantPolicyConditions{Logic: "OR", Rules: []merchantPolicyCondition{
			{Field: policyMailbox, Operator: "eq", Value: json.RawMessage(`"private-mailbox-reference"`)},
			{Field: policyDisposableEmail, Operator: "eq", Value: json.RawMessage("false")},
		}},
	}
	definition := definitionFromProposal(proposal)
	if definition == nil || definition.Duration == nil || definition.Duration.Value != 2 || definition.Duration.Unit != "minute" ||
		definition.Conditions.Logic != "OR" || definition.Conditions.Rules[0].Value != nil {
		t.Fatalf("unexpected redacted definition: %+v", definition)
	}
	value, err := json.Marshal(definition.Conditions.Rules[1].Value)
	if err != nil || string(value) != "false" {
		t.Fatalf("expected a JSON boolean, got %s (%v)", value, err)
	}
	response := httptest.NewRecorder()
	respondRuleInterpretation(response, composerInterpretation{Status: "CLARIFICATION", Message: "Enter an amount."})
	var result composerInterpretation
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Definition != nil {
		t.Fatalf("clarification must omit the definition: %s (%v)", response.Body.String(), err)
	}
}
