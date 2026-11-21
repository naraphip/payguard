package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const bookingHoldSchema = `
CREATE TABLE IF NOT EXISTS booking_holds (
  merchant_id text NOT NULL,
  event_id uuid NOT NULL,
  state text NOT NULL CHECK (state IN ('OPEN', 'CLAIMED', 'ESCALATED', 'RESOLVED')),
  due_at timestamptz NOT NULL,
  assigned_actor text NOT NULL DEFAULT '',
  resolution text NOT NULL DEFAULT '' CHECK (resolution IN ('', 'cleared', 'rejected')),
  escalated_at timestamptz,
  resolved_at timestamptz,
  PRIMARY KEY (merchant_id, event_id),
  FOREIGN KEY (merchant_id, event_id) REFERENCES booking_assessments (merchant_id, event_id)
);
CREATE TABLE IF NOT EXISTS booking_hold_history (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  merchant_id text NOT NULL,
  event_id uuid NOT NULL,
  actor_id text NOT NULL,
  action text NOT NULL CHECK (action IN ('claim', 'escalate', 'resolve')),
  prior_state text NOT NULL,
  state text NOT NULL,
  recorded_at timestamptz NOT NULL,
  FOREIGN KEY (merchant_id, event_id) REFERENCES booking_holds (merchant_id, event_id)
);
`

// A fixed local SLA keeps the demo deterministic without choosing a payment policy.
const bookingHoldDue = `COALESCE((a.result->>'assessedAt')::timestamptz, e.received_at) + interval '15 minutes'`

const (
	holdOpen      = "OPEN"
	holdClaimed   = "CLAIMED"
	holdEscalated = "ESCALATED"
	holdResolved  = "RESOLVED"
)

var errBookingHoldTransition = errors.New("This HOLD transition conflicts with its current state or assignee.")

type bookingHold struct {
	State         string     `json:"state"`
	DueAt         time.Time  `json:"dueAt"`
	AssignedActor string     `json:"assignedActor,omitempty"`
	Resolution    string     `json:"resolution,omitempty"`
	EscalatedAt   *time.Time `json:"escalatedAt,omitempty"`
	ResolvedAt    *time.Time `json:"resolvedAt,omitempty"`
	Overdue       bool       `json:"overdue"`
}

func (hold *bookingHold) normalize(now time.Time) {
	hold.DueAt = hold.DueAt.UTC()
	if hold.EscalatedAt != nil {
		stamp := hold.EscalatedAt.UTC()
		hold.EscalatedAt = &stamp
	}
	if hold.ResolvedAt != nil {
		stamp := hold.ResolvedAt.UTC()
		hold.ResolvedAt = &stamp
	}
	hold.Overdue = hold.State != holdResolved && hold.EscalatedAt == nil && !now.Before(hold.DueAt)
}

func (hold *bookingHold) transition(actor, action, resolution string, now time.Time) (bool, error) {
	switch action {
	case "claim":
		if hold.State == holdClaimed && hold.AssignedActor == actor && !hold.Overdue {
			return false, nil
		}
		if (hold.State == holdOpen || hold.State == holdEscalated) && !hold.Overdue {
			hold.State = holdClaimed
			hold.AssignedActor = actor
			return true, nil
		}
	case "escalate":
		if hold.State == holdEscalated {
			return false, nil
		}
		if hold.Overdue && (hold.State == holdOpen || hold.State == holdClaimed) {
			hold.State = holdEscalated
			hold.AssignedActor = ""
			hold.EscalatedAt = &now
			hold.Overdue = false
			return true, nil
		}
	case "resolve":
		if hold.State == holdResolved && hold.AssignedActor == actor && hold.Resolution == resolution {
			return false, nil
		}
		if hold.State == holdClaimed && hold.AssignedActor == actor && !hold.Overdue {
			hold.State = holdResolved
			hold.Resolution = resolution
			hold.ResolvedAt = &now
			return true, nil
		}
	}
	return false, errBookingHoldTransition
}

func (h bookingOpsHandler) changeHold(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := h.auth.requireBookingActor(w, r, bookingRoleAnalyst)
		if actor.MerchantID == "" {
			return
		}
		eventID := bookingOpsEventID(w, r)
		if eventID == "" {
			return
		}
		var input struct {
			Resolution string `json:"resolution"`
		}
		if action == "resolve" {
			if !decodeBookingRiskJSON(w, r, &input) {
				return
			}
			if input.Resolution != "cleared" && input.Resolution != "rejected" {
				problem(w, http.StatusUnprocessableEntity, "Choose a supported HOLD resolution.")
				return
			}
		}
		hold, err := h.repo.transitionBookingHold(r.Context(), actor, eventID, action, input.Resolution)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			problem(w, http.StatusNotFound, "Booking review HOLD not found.")
		case errors.Is(err, errBookingHoldTransition):
			problem(w, http.StatusConflict, err.Error())
		case err != nil:
			bookingRiskUnavailable(w, err)
		default:
			respond(w, http.StatusOK, hold)
		}
	}
}

func (r repository) transitionBookingHold(ctx context.Context, actor bookingActor, eventID, action, resolution string) (bookingHold, error) {
	var hold bookingHold
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO booking_holds (merchant_id, event_id, state, due_at)
			SELECT e.merchant_id, e.event_id, 'OPEN', `+bookingHoldDue+`
			FROM booking_events e JOIN booking_assessments a USING (merchant_id, event_id)
			WHERE e.merchant_id = $1 AND e.event_id = $2::text::uuid AND a.result->>'handoff' = 'MANUAL_REVIEW'
			ON CONFLICT DO NOTHING`, actor.MerchantID, eventID)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			SELECT state, due_at, assigned_actor, resolution, escalated_at, resolved_at FROM booking_holds
			WHERE merchant_id = $1 AND event_id = $2::text::uuid FOR UPDATE`, actor.MerchantID, eventID).
			Scan(&hold.State, &hold.DueAt, &hold.AssignedActor, &hold.Resolution, &hold.EscalatedAt, &hold.ResolvedAt)
		if err != nil {
			return err
		}
		// Evaluate expiration after acquiring the row lock.
		var now time.Time
		if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
			return err
		}
		now = now.UTC()
		hold.normalize(now)
		priorState := hold.State
		changed, err := hold.transition(actor.ActorID, action, resolution, now)
		if err != nil || !changed {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE booking_holds SET state = $3, assigned_actor = $4, resolution = $5, escalated_at = $6, resolved_at = $7
			WHERE merchant_id = $1 AND event_id = $2::text::uuid`, actor.MerchantID, eventID,
			hold.State, hold.AssignedActor, hold.Resolution, hold.EscalatedAt, hold.ResolvedAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO booking_hold_history (merchant_id, event_id, actor_id, action, prior_state, state, recorded_at)
			VALUES ($1, $2::text::uuid, $3, $4, $5, $6, $7)`, actor.MerchantID, eventID,
			actor.ActorID, action, priorState, hold.State, now)
		return err
	})
	return hold, err
}
