package main

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestMerchantEmailShape(t *testing.T) {
	for _, item := range []struct {
		name   string
		shape  *merchantEmailShape
		known  bool
		random bool
	}{
		{"unavailable", nil, false, false},
		{"interleaved handle", &merchantEmailShape{Length: 12, Letters: 6, Digits: 6, Transitions: 11}, true, true},
		{"typical numeric suffix", &merchantEmailShape{Length: 14, Letters: 8, Digits: 6, Transitions: 1}, true, false},
		{"punctuated address", &merchantEmailShape{Length: 13, Letters: 6, Digits: 6, Transitions: 9}, true, false},
		{"punctuation only", &merchantEmailShape{Length: 3}, true, false},
		{"short handle", &merchantEmailShape{Length: 8, Letters: 4, Digits: 4, Transitions: 7}, true, false},
		{"impossible count", &merchantEmailShape{Length: 12, Letters: 5, Digits: 8, Transitions: 1}, false, false},
		{"impossible transitions", &merchantEmailShape{Length: 16, Letters: 4, Digits: 12, Transitions: 9}, false, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			got := item.shape.randomLocalPart()
			if (got != nil) != item.known || (got != nil && *got != item.random) {
				t.Fatalf("classification should be known=%v and random=%v, got %v", item.known, item.random, got)
			}
		})
	}
}

func TestMerchantEmailRiskReplayPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	ctx := t.Context()
	repo := repository{db}
	merchant, _, err := repo.createMockMerchant(ctx, mockMerchantInput{Name: "Email evidence test merchant",
		PolicyIntegration: &merchantPolicyIntegration{Products: []string{"flight_ticket", "hotel_room"}, EmailDomain: true}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := merchantPolicyProposal{
		Scope: "MERCHANT", Name: "Email evidence test", ProductTypes: []string{"flight_ticket", "hotel_room"},
		Stages: []string{"BOOKING"}, Action: "BLOCK", Conditions: merchantPolicyConditions{Logic: "OR", Rules: []merchantPolicyCondition{
			{Field: policyDisposableEmail, Operator: "eq", Value: json.RawMessage(`true`)},
			{Field: policyRandomEmail, Operator: "eq", Value: json.RawMessage(`true`)},
		}},
	}
	rule, err := repo.mutateMerchantPolicy(ctx, merchant.ID, merchantPolicyMutation{Kind: "save", Proposal: proposal})
	if err != nil || rule.Status != "ACTIVE" {
		t.Fatalf("save rule: %+v %v", rule, err)
	}
	input := mockTransactionInput{ID: "7bd5864e-852b-11ee-b9d1-0242ac120002", Source: "checkout", Product: "hotel_room",
		CustomerEmailDomain: "gmail.com", CustomerMailboxDigest: strings.Repeat("a", 64),
		CustomerEmailShape: &merchantEmailShape{Length: 12, Letters: 6, Digits: 6, Transitions: 11}}
	first, created, err := repo.saveMockTransaction(ctx, merchant.ID, input)
	if err != nil || !created || first.PolicyDecision == nil || first.PolicyDecision.Final.Action != "BLOCK" {
		t.Fatalf("expected new block: %+v %v", first, err)
	}
	_, evidence, err := readMerchantPolicyContext(ctx, db, merchant.ID)
	if err != nil || len(evidence) != 1 || previewMerchantPolicy(rule.Proposal, evidence).Matched != 1 {
		t.Fatalf("historical preview should match the stored email classification: %v", err)
	}
	_, err = repo.mutateMerchantPolicy(ctx, merchant.ID, merchantPolicyMutation{Kind: "disable", ID: rule.ID, ExpectedVersion: rule.Version})
	if err != nil {
		t.Fatal(err)
	}
	replayed, created, err := repo.saveMockTransaction(ctx, merchant.ID, input)
	if err != nil || created || replayed.PolicyDecision == nil || replayed.PolicyDecision.Final.Action != "BLOCK" {
		t.Fatalf("expected original block on identical replay: %+v %v", replayed, err)
	}
	input.CustomerEmailShape = &merchantEmailShape{Length: 14, Letters: 8, Digits: 6, Transitions: 1}
	if _, _, err := repo.saveMockTransaction(ctx, merchant.ID, input); !errors.Is(err, errDemoReplayConflict) {
		t.Fatalf("expected conflict for changed email classification, got %v", err)
	}
}

func TestMerchantPolicyEmailRiskMatchesEitherSignal(t *testing.T) {
	profile := exampleMerchantPolicyProfile()
	profile.Signals = append(profile.Signals, policyDisposableEmail, policyRandomEmail)
	policy := merchantPolicyProposal{
		Scope: "MERCHANT", Name: "Email risk", ProductTypes: []string{"flight_ticket", "hotel_room"},
		Stages: []string{"BOOKING"}, Action: "BLOCK", Conditions: merchantPolicyConditions{Logic: "OR", Rules: []merchantPolicyCondition{
			{Field: policyDisposableEmail, Operator: "eq", Value: json.RawMessage(`true`)},
			{Field: policyRandomEmail, Operator: "eq", Value: json.RawMessage(`true`)},
		}},
	}
	if err := policy.validate(profile); err != nil {
		t.Fatal(err)
	}
	random, ordinary := true, false
	for _, item := range []struct {
		name, domain, product string
		random                *bool
		wantAction, missing   string
	}{
		{"ordinary hotel", "gmail.com", "hotel_room", &ordinary, "NOT_APPLICABLE", ""},
		{"ordinary flight", "gmail.com", "flight_ticket", &ordinary, "NOT_APPLICABLE", ""},
		{"random Gmail hotel", "gmail.com", "hotel_room", &random, "BLOCK", ""},
		{"random Gmail flight", "gmail.com", "flight_ticket", &random, "BLOCK", ""},
		{"disposable hotel", "mailinator.com", "hotel_room", nil, "BLOCK", ""},
		{"disposable subdomain flight", "test.mailinator.com", "flight_ticket", &ordinary, "BLOCK", ""},
		{"normal domain without mailbox", "gmail.com", "hotel_room", nil, "UNAVAILABLE", policyRandomEmail},
		{"out-of-scope product", "mailinator.com", "digital", &random, "NOT_APPLICABLE", ""},
	} {
		t.Run(item.name, func(t *testing.T) {
			decision := evaluateMerchantPolicies([]merchantPolicyRule{{ID: "email-rule", Version: 1, Proposal: policy}}, merchantPolicyEvidence{
				product: item.product, stage: merchantPolicyStage(item.product), domain: item.domain, randomLocalPart: item.random,
			})
			if decision.Action != item.wantAction || (item.missing != "" && !slices.Contains(decision.MissingSignals, item.missing)) {
				t.Fatalf("expected %s with missing=%q, got %+v", item.wantAction, item.missing, decision)
			}
		})
	}
	for _, invalid := range []string{`"true"`, `null`, `1`} {
		policy.Conditions.Rules[0].Value = json.RawMessage(invalid)
		if err := policy.validate(profile); err == nil {
			t.Fatalf("accepted non-boolean %q", invalid)
		}
	}
}
