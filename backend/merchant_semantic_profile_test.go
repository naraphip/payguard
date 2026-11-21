package main

import (
	"encoding/json"
	"testing"
)

func TestSemanticPolicyPreviewUsesOnlyObservedFields(t *testing.T) {
	profile := merchantPolicyProfile{
		Products: []string{"physical"}, Stages: []string{"CHECKOUT"},
		Signals: []string{policyDescription, policyAmount, policyDomain},
	}
	rule := merchantPolicyProposal{
		Scope: "MERCHANT", Name: "Park admission", ProductTypes: profile.Products, Stages: profile.Stages, Action: "BLOCK",
		Conditions: merchantPolicyConditions{Logic: "AND", Rules: []merchantPolicyCondition{
			{Field: policyDescription, Operator: semanticOperator, Value: json.RawMessage(`"theme park entry"`)},
			{Field: policyAmount, Operator: "gte", Value: json.RawMessage(`100`)},
		}},
	}
	if err := rule.validate(profile); err != nil {
		t.Fatal(err)
	}
	observed := map[string]json.RawMessage{policyDescription: json.RawMessage(`"One-day entry"`)}
	hi, lo := int64(200), int64(10)
	samples := []merchantPolicyEvidence{
		{product: "physical", stage: "CHECKOUT", amount: &hi, custom: observed},
		{product: "physical", stage: "CHECKOUT", amount: &lo, custom: observed},
		{product: "physical", stage: "CHECKOUT", amount: &hi},
	}
	preview := previewMerchantPolicy(rule, samples)
	if preview.RequiresJev != 1 || preview.Matched != 0 || preview.Missing != 1 || preview.Evaluated != 3 {
		t.Fatalf("AND: %+v", preview)
	}
	rule.Conditions.Logic = "OR"
	preview = previewMerchantPolicy(rule, samples)
	if preview.RequiresJev != 1 || preview.Matched != 2 || preview.Missing != 0 {
		t.Fatalf("OR: %+v", preview)
	}
	for _, field := range []string{policyAmount, policyDomain, policyMailbox, "custom_unregistered"} {
		rule.Conditions.Rules = []merchantPolicyCondition{{Field: field, Operator: semanticOperator, Value: json.RawMessage(`"theme park"`)}}
		if err := rule.validate(profile); err == nil {
			t.Errorf("unexpected semantic operator for %s", field)
		}
	}
}

func TestDescriptionPrivacyAndLiteralMatch(t *testing.T) {
	for _, value := range []string{`"buyer@example.test"`, `"192.0.2.99"`, `"4242 4242 4242 4242"`, `false`} {
		if validCustomValue("text", json.RawMessage(value)) {
			t.Errorf("unexpected description accepted: %s", value)
		}
	}
	c := merchantPolicyCondition{Field: policyDescription, Operator: "contains", Value: json.RawMessage(`"entry"`)}
	e := merchantPolicyEvidence{custom: map[string]json.RawMessage{policyDescription: json.RawMessage(`"Park ENTRY pass"`)}}
	if yes, missing := c.evaluate(e); !yes || missing {
		t.Fatal("literal contains did not run locally")
	}
}
