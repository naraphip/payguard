package main

import (
	"slices"
	"testing"
)

func TestCustomMerchantRuleKeepsEveryCondition(t *testing.T) {
	profile := localComposerTestProfile()
	profile.Signals = append(profile.Signals, "custom_ip_country")
	profile.FieldRegistry = append(profile.FieldRegistry, merchantCapability{
		Name: "custom_ip_country", Products: profile.Products,
		policyField: policyField{DataType: "country", Operators: []string{"eq", "ne"}},
	})
	for _, test := range []struct {
		name, prompt, action, logic string
		withCountry                 bool
		fields, operators, values   []string
	}{
		{
			name:   "unknown country field preserves the entire predicate",
			prompt: "Block ticket ที่เป็น *disney* land hongkong และ ip มาจาก india",
			action: "BLOCK", logic: "AND",
			fields: []string{policyDescription}, operators: []string{semanticOperator},
			values: []string{`"ticket ที่เป็น *disney* land hongkong และ ip มาจาก india"`},
		},
		{
			name:   "country and ticket evaluate independently",
			prompt: "Block ticket ที่เป็น *disney* land hongkong และ ip มาจาก india",
			action: "BLOCK", logic: "AND", withCountry: true,
			fields: []string{"custom_ip_country", policyDescription}, operators: []string{"eq", semanticOperator},
			values: []string{`"IN"`, `"ticket ที่เป็น *disney* land hongkong"`},
		},
		{
			name:   "keep native amount beside explicitly assigned Jev field",
			prompt: `Review physical goods when amount is at least 50000 minor units or custom_ticket_title refers to "hockey and skating admission"`,
			action: "REVIEW", logic: "OR",
			fields: []string{"custom_ticket_title", policyAmount}, operators: []string{semanticOperator, "gte"},
			values: []string{`"hockey and skating admission"`, "50000"},
		},
		{
			name:   "registered typed fields stay native",
			prompt: "Challenge physical goods when custom_refundable equals false and ip from อินเดีย",
			action: "CHALLENGE", logic: "AND", withCountry: true,
			fields: []string{"custom_ip_country", "custom_refundable"}, operators: []string{"eq", "eq"},
			values: []string{`"IN"`, "false"},
		},
		{
			name:   "English trailing action",
			prompt: "If it looks like a nonrefundable park ticket, then block the transaction.",
			action: "BLOCK", logic: "AND",
			fields: []string{policyDescription}, operators: []string{semanticOperator},
			values: []string{`"it looks like a nonrefundable park ticket"`},
		},
		{
			name:   "Thai trailing action",
			prompt: "ถ้าเป็นตั๋วเข้าสวนสนุกให้บล็อกธุรกรรม",
			action: "BLOCK", logic: "AND",
			fields: []string{policyDescription}, operators: []string{semanticOperator},
			values: []string{`"เป็นตั๋วเข้าสวนสนุก"`},
		},
		{
			name:   "time window describes evidence",
			prompt: "Review bookings when a repeat booking was made in the last 24 hours",
			action: "REVIEW", logic: "AND",
			fields: []string{policyDescription}, operators: []string{semanticOperator},
			values: []string{`"a repeat booking was made in the last 24 hours"`},
		},
		{
			name:   "account age describes evidence",
			prompt: "Review when account age is 3 days",
			action: "REVIEW", logic: "AND",
			fields: []string{policyDescription}, operators: []string{semanticOperator},
			values: []string{`"account age is 3 days"`},
		},
		{
			name:   "nested logic remains intact",
			prompt: "Block physical goods when (amount is at least 100 minor units or order is a voucher) and pickup is urgent",
			action: "BLOCK", logic: "AND",
			fields: []string{policyDescription}, operators: []string{semanticOperator},
			values: []string{`"(amount is at least 100 minor units or order is a voucher) and pickup is urgent"`},
		},
		{
			name:   "an undeclared field is compared only against available text",
			prompt: `Block physical goods when custom_unregistered means "ticket for an event"`,
			action: "BLOCK", logic: "AND",
			fields: []string{policyDescription}, operators: []string{semanticOperator},
			values: []string{`"custom_unregistered means \"ticket for an event\""`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			actualProfile := profile
			if !test.withCountry {
				actualProfile = localComposerTestProfile()
			}
			result, handled := interpretCustomMerchantRule(test.prompt, actualProfile)
			proposal, ready := result.Proposal.(merchantPolicyProposal)
			if !handled || !ready || !result.Validation.Valid || proposal.Action != test.action || proposal.Conditions.Logic != test.logic {
				t.Fatalf("expected a validated %s %s-group: %+v", test.action, test.logic, result)
			}
			var fields, operators, values []string
			for _, condition := range proposal.Conditions.Rules {
				fields = append(fields, condition.Field)
				operators = append(operators, condition.Operator)
				values = append(values, string(condition.Value))
			}
			if !slices.Equal(fields, test.fields) || !slices.Equal(operators, test.operators) || !slices.Equal(values, test.values) {
				t.Fatalf("conditions changed meaning: %+v", proposal.Conditions.Rules)
			}
		})
	}

	profile = localComposerTestProfile()
	profile.Signals = slices.DeleteFunc(profile.Signals, func(field string) bool { return field == policyAmount })
	const unavailableAmount = "Review physical goods when amount is at least 100 minor units"
	if _, handled := interpretLocalMerchantRule(unavailableAmount, profile); handled {
		t.Fatal("an unavailable native field must fall through for custom interpretation")
	}
	fallback, handled := interpretCustomMerchantRule(unavailableAmount, profile)
	proposal, ready := fallback.Proposal.(merchantPolicyProposal)
	if !handled || !ready || proposal.Conditions.Rules[0].Field != policyDescription ||
		string(proposal.Conditions.Rules[0].Value) != `"amount is at least 100 minor units"` {
		t.Fatalf("unavailable native field must use explicitly previewed transaction text: %+v", fallback)
	}
}

func TestCustomMerchantRuleRejectsAmbiguousOrPrivateInput(t *testing.T) {
	for _, instruction := range []string{
		"Offer a voucher when an otherwise eligible ticket costs too much",
		"Do not block theme park tickets",
		"If the order is for a concert, then block or review",
		"Maybe block theme park tickets",
		"Block physical goods when the buyer seems suspicious and review other orders",
		"Block theme park tickets for 2 minutes",
		"Block theme park tickets for two minutes",
		"Review physical goods when amount is at least 100",
		"Review physical goods when amount is at least 12.34 minor units",
		"Block tickets from ip 198.51.100.17",
		"Block buyer@example.test 2min",
		"Review CVV 123",
		"Block all products",
	} {
		if proposal, handled := interpretCustomMerchantRule(instruction, localComposerTestProfile()); handled {
			t.Errorf("%q did not require clarification: %+v", instruction, proposal)
		}
	}
}
