package main

import (
	"slices"
	"testing"
)

func TestComposeEmailRisk(t *testing.T) {
	profile := merchantPolicyProfile{
		Products: []string{"flight_ticket", "hotel_room", "payment_gateway"},
		Stages:   []string{"BOOKING", "CHECKOUT"},
		Signals:  []string{policyProduct, policyAmount, policyDisposableEmail, policyRandomEmail},
	}
	for _, instruction := range []string{
		"Block flight tickets and hotel rooms when the email is disposable or the local part looks randomly generated",
		"Block flight tickets and hotel rooms when the email is random",
		"บล็อกตั๋วเครื่องบินและห้องพักเมื่อเป็นอีเมลชั่วคราวหรือชื่อก่อน @ ดูสุ่ม",
	} {
		t.Run(instruction, func(t *testing.T) {
			result, handled := interpretEmailRiskRule(instruction, profile)
			proposal, ok := result.Proposal.(merchantPolicyProposal)
			if !handled || !ok || result.Status != "READY" || !result.Validation.Valid {
				t.Fatalf("expected ready proposal, got %+v", result)
			}
			if !slices.Equal(proposal.ProductTypes, []string{"flight_ticket", "hotel_room"}) ||
				!slices.Equal(proposal.Stages, []string{"BOOKING"}) || proposal.Action != "BLOCK" ||
				proposal.Conditions.Logic != "OR" || len(proposal.Conditions.Rules) != 2 {
				t.Fatalf("incorrect scope or conditions: %+v", proposal)
			}
			for i, field := range []string{policyDisposableEmail, policyRandomEmail} {
				condition := proposal.Conditions.Rules[i]
				if condition.Field != field || condition.Operator != "eq" || string(condition.Value) != "true" {
					t.Fatalf("email condition was changed or omitted: %+v", condition)
				}
			}
		})
	}
	for _, instruction := range []string{
		"Block random email",
		"Block hotel rooms when the email is random and the booking is refundable",
		"Block hotel rooms when not a random email",
	} {
		result, handled := interpretEmailRiskRule(instruction, profile)
		if !handled || result.Status != "CLARIFICATION" || result.Proposal != nil {
			t.Fatalf("expected clarification for %q, got %+v", instruction, result)
		}
	}
	if _, handled := interpretEmailRiskRule("Block hotel rooms when email domain equals tempmail.com", profile); handled {
		t.Fatal("the exact domain parser should handle an explicit host")
	}
}
