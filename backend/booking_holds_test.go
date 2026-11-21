package main

import (
	"errors"
	"testing"
	"time"
)

func TestBookingHoldOwnershipAndResolution(t *testing.T) {
	now := time.Now().UTC()
	hold := bookingHold{State: holdOpen, DueAt: now.Add(time.Minute)}
	if changed, err := hold.transition("alice", "claim", "", now); !changed || err != nil {
		t.Fatalf("claim: %v %v", changed, err)
	}
	if changed, err := hold.transition("bob", "claim", "", now); changed || !errors.Is(err, errBookingHoldTransition) {
		t.Fatalf("competing claim: %v %v", changed, err)
	}
	if changed, err := hold.transition("bob", "resolve", "cleared", now); changed || !errors.Is(err, errBookingHoldTransition) {
		t.Fatalf("another analyst cannot resolve: %v %v", changed, err)
	}
	if changed, err := hold.transition("alice", "resolve", "cleared", now); !changed || err != nil || hold.ResolvedAt == nil {
		t.Fatalf("resolve: %+v %v %v", hold, changed, err)
	}
	if changed, err := hold.transition("alice", "resolve", "cleared", now); changed || err != nil {
		t.Fatalf("same resolution replay: %v %v", changed, err)
	}
	if changed, err := hold.transition("alice", "resolve", "rejected", now); changed || !errors.Is(err, errBookingHoldTransition) {
		t.Fatalf("changed resolution: %v %v", changed, err)
	}
}

func TestBookingHoldExplicitEscalation(t *testing.T) {
	now := time.Now().UTC()
	hold := bookingHold{State: holdClaimed, DueAt: now, AssignedActor: "alice"}
	hold.normalize(now)
	if !hold.Overdue {
		t.Fatal("HOLD should be overdue at the SLA boundary")
	}
	if changed, err := hold.transition("alice", "resolve", "cleared", now); changed || !errors.Is(err, errBookingHoldTransition) {
		t.Fatalf("an overdue HOLD requires explicit escalation: %v %v", changed, err)
	}
	if changed, err := hold.transition("bob", "escalate", "", now); !changed || err != nil ||
		hold.AssignedActor != "" || hold.EscalatedAt == nil || hold.State != holdEscalated {
		t.Fatalf("escalate: %+v %v %v", hold, changed, err)
	}
	if changed, err := hold.transition("bob", "claim", "", now); !changed || err != nil {
		t.Fatalf("claim after escalation: %v %v", changed, err)
	}
}
