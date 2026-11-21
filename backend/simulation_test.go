package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"
)

func TestSeededEventsAndTenantKeys(t *testing.T) {
	config := config{
		Seed: 42, Product: "flight_ticket", Scenario: "mixed", Accounts: 12, Rate: 30,
		Quantity: 1, Date: "2026-12-04", SharedIP: true, RotateIP: true,
		SharedDevice: true, SharedToken: true, Retry: true,
	}
	secret := []byte("test-key")
	events, truth := generate(config, "tenant-a", secret)
	again, truthAgain := generate(config, "tenant-a", secret)
	otherTenant, _ := generate(config, "tenant-b", secret)
	raw, _ := json.Marshal(events)
	replayed, _ := json.Marshal(again)
	if !bytes.Equal(raw, replayed) || !slices.Equal(truth, truthAgain) {
		t.Fatal("replay changed events or labels")
	}
	if bytes.Contains(raw, []byte("ringId")) || bytes.Contains(raw, []byte("fraud")) {
		t.Fatal("generator leaked labels into events")
	}
	for i, e := range events {
		for j, entity := range e.Entities {
			if entity.Key == otherTenant[i].Entities[j].Key {
				t.Fatalf("entity %s shared across tenants", entity.Kind)
			}
		}
	}
}

type recordedVelocity struct {
	events []event
}

func (v *recordedVelocity) count(_ context.Context, e event) ([]feature, error) {
	v.events = append(v.events, e)
	return []feature{{Kind: "ip", Orders: 18}}, nil
}

func TestEvaluationOrdersAndDeduplicates(t *testing.T) {
	config := config{
		Seed: 5, Product: "physical", Scenario: "normal", Accounts: 5, Rate: 60,
		Quantity: 4, Delivery: "express", SharedIP: true, Retry: true,
	}
	events, _ := generate(config, "tenant-a", []byte("test-key"))
	counter := &recordedVelocity{}
	decisions, err := evaluate(context.Background(), counter, events, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 10 || len(counter.events) != 10 {
		t.Fatalf("wanted ten unique evaluations, got %d", len(decisions))
	}
	for i, d := range decisions {
		if i > 0 && decisions[i-1].Event.At > d.Event.At {
			t.Fatal("events processed outside event-time order")
		}
		if d.Action == "BLOCK" {
			t.Fatal("shared IP alone produced a block")
		}
	}
	payment := event{ID: "retry", Stage: "PAYMENT", At: 200, Product: "physical", Quantity: 1}
	conflict := payment
	conflict.Quantity = 2
	if _, err := evaluate(context.Background(), counter, []event{payment, conflict}, false, false); err == nil {
		t.Fatal("conflicting duplicate was accepted")
	}
}

func TestPolicyAndTimeoutKeepSeparateEvidence(t *testing.T) {
	e := event{Stage: "PRE_PAYMENT", Product: "flight_ticket", Domain: "restricted.test"}
	d := assess(e, []feature{{Kind: "token", Orders: 1}}, true, false)
	if d.Action != "BLOCK" || d.Source != "MERCHANT_POLICY" || d.Platform != "ALLOW" {
		t.Fatalf("merchant policy changed platform decision: %+v", d)
	}
	e.Domain = "restricted.test.other.test"
	d = assess(e, []feature{{Kind: "token", Orders: 1}}, true, false)
	if d.Action != "ALLOW" || d.Merchant != "PASS" {
		t.Fatalf("unexpected suffix policy match: %+v", d)
	}
	d = assess(e, nil, true, true)
	if d.Action != "UNSUCCESSFUL" || d.JevStatus != "TIMEOUT" || !slices.Contains(d.Missing, "payment_token") {
		t.Fatalf("missing timeout or token evidence: %+v", d)
	}
}
