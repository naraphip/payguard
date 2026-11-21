package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

const matchingMailboxDigest = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"

func temporaryMailboxProposal() merchantPolicyProposal {
	value, _ := json.Marshal(matchingMailboxDigest)
	return merchantPolicyProposal{
		Scope: "MERCHANT", Name: "Temporary mailbox block", ProductTypes: []string{"flight_ticket"},
		Stages: []string{"BOOKING"}, Action: "BLOCK", DurationHours: 12,
		Conditions: merchantPolicyConditions{Logic: "AND", Rules: []merchantPolicyCondition{
			{Field: policyMailbox, Operator: "eq", Value: value},
		}},
	}
}

func TestTemporaryMailboxProposalValidationAndMatch(t *testing.T) {
	profile := merchantPolicyProfile{
		Products: []string{"flight_ticket"}, Stages: []string{"BOOKING"}, Signals: []string{policyMailbox, policyDomain},
	}
	for _, test := range []struct {
		name   string
		mutate func(*merchantPolicyProposal)
	}{
		{"wrong action", func(p *merchantPolicyProposal) { p.Action = "REVIEW" }},
		{"disjunction", func(p *merchantPolicyProposal) { p.Conditions.Logic = "OR" }},
		{"zero hours", func(p *merchantPolicyProposal) { p.DurationHours = 0 }},
		{"excessive hours", func(p *merchantPolicyProposal) { p.DurationHours = maximumMailboxBlockHours + 1 }},
		{"two units", func(p *merchantPolicyProposal) { p.DurationMinutes = 2 }},
		{"negative minutes", func(p *merchantPolicyProposal) { p.DurationHours, p.DurationMinutes = 0, -2 }},
		{"excessive minutes", func(p *merchantPolicyProposal) { p.DurationHours, p.DurationMinutes = 0, maximumMailboxBlockMinutes+1 }},
		{"invalid operator", func(p *merchantPolicyProposal) { p.Conditions.Rules[0].Operator = "ne" }},
		{"email in title", func(p *merchantPolicyProposal) { p.Name = "Block buyer@example.test" }},
		{"email as value", func(p *merchantPolicyProposal) { p.Conditions.Rules[0].Value = json.RawMessage(`"buyer@example.test"`) }},
		{"duration on domain rule", func(p *merchantPolicyProposal) {
			p.Conditions.Rules[0] = merchantPolicyCondition{Field: policyDomain, Operator: "eq", Value: json.RawMessage(`"example.test"`)}
		}},
		{"minute duration on domain rule", func(p *merchantPolicyProposal) {
			p.DurationHours, p.DurationMinutes = 0, 2
			p.Conditions.Rules[0] = merchantPolicyCondition{Field: policyDomain, Operator: "eq", Value: json.RawMessage(`"example.test"`)}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			proposal := temporaryMailboxProposal()
			test.mutate(&proposal)
			if err := proposal.validate(profile); err == nil {
				t.Fatal("Invalid temporary block accepted.")
			}
		})
	}
	minute := temporaryMailboxProposal()
	minute.DurationHours, minute.DurationMinutes = 0, 2
	if err := minute.validate(profile); err != nil {
		t.Fatal(err)
	}
	proposal := temporaryMailboxProposal()
	if err := proposal.validate(profile); err != nil {
		t.Fatal(err)
	}
	base := merchantPolicyEvidence{product: "flight_ticket", stage: "BOOKING", domain: "example.test"}
	for _, test := range []struct {
		ref     string
		matched bool
		missing bool
	}{
		{matchingMailboxDigest, true, false},
		{strings.Repeat("e", 64), false, false},
		{"", false, true},
	} {
		evidence := base
		evidence.mailboxRef = test.ref
		if matched, missing := proposal.evaluate(evidence); matched != test.matched || missing != test.missing {
			t.Errorf("mailbox evidence %q: matched=%v missing=%v", test.ref, matched, missing)
		}
	}
}

func TestTemporaryMailboxPolicyExpiryPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	ctx := t.Context()
	repo := repository{db}
	merchant, _, err := repo.createMockMerchant(ctx, mockMerchantInput{Name: "Temporary block test merchant",
		PolicyIntegration: &merchantPolicyIntegration{Products: []string{"flight_ticket"}}})
	if err != nil {
		t.Fatal(err)
	}
	profile, _, err := readMerchantPolicyContext(ctx, db, merchant.ID)
	if err != nil || profile.MerchantID != merchant.ID || !slices.Contains(profile.Signals, policyMailbox) {
		t.Fatalf("Mailbox capability missing: %+v, err=%v", profile, err)
	}
	proposal := temporaryMailboxProposal()
	mutate := func(input merchantPolicyMutation) merchantPolicyRule {
		t.Helper()
		rule, err := repo.mutateMerchantPolicy(ctx, merchant.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		return rule
	}
	first := mutate(merchantPolicyMutation{Kind: "save", Proposal: proposal})
	if first.Status != "ACTIVE" || first.ExpiresAt == nil || first.ExpiresAt.Sub(first.CreatedAt) != 12*time.Hour {
		t.Fatalf("First activation has an incorrect window: %+v", first)
	}
	updated := mutate(merchantPolicyMutation{Kind: "edit", ID: first.ID, ExpectedVersion: first.Version, Proposal: proposal})
	if updated.Version != 2 || updated.Status != "ACTIVE" || updated.ExpiresAt == nil {
		t.Fatalf("Expected a second active revision: %+v", updated)
	}
	// Advance the active revision to the expiration boundary while its predecessor is still within its window.
	if _, err := db.Exec(ctx, `UPDATE merchant_policy_rule_versions SET created_at=statement_timestamp()-interval '12 hours'
		WHERE merchant_id=$1::uuid AND rule_id=$2::uuid AND version=$3`, merchant.ID, updated.ID, updated.Version); err != nil {
		t.Fatal(err)
	}
	effective, err := readMerchantPolicies(ctx, db, merchant.ID, true)
	if err != nil || len(effective) != 0 {
		t.Fatalf("Expired rule or earlier revision remained effective: %+v, err=%v", effective, err)
	}
	retried := mutate(merchantPolicyMutation{Kind: "save", Proposal: proposal})
	if retried.ID != updated.ID || retried.Version != updated.Version || retried.Status != "EXPIRED" || retried.ActiveVersion != 0 {
		t.Fatalf("Retry after expiration must not reactivate the policy: %+v", retried)
	}
}
