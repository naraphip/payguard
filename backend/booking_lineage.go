package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

type bookingLineage struct {
	Booking    bookingEvent     `json:"booking"`
	Assessment json.RawMessage  `json:"assessment"`
	Outcomes   []bookingOutcome `json:"outcomes"`
}

func (h bookingRiskHandler) readLineage(w http.ResponseWriter, r *http.Request) {
	merchant := h.auth.requireMerchant(w, r)
	if merchant == "" {
		return
	}
	eventID := r.PathValue("id")
	if !validID.MatchString(eventID) {
		problem(w, http.StatusBadRequest, "Expected a lowercase booking event UUID.")
		return
	}
	result, err := h.repo.bookingLineage(r.Context(), merchant, eventID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Booking event not found.")
	case err != nil:
		bookingRiskUnavailable(w, err)
	default:
		respond(w, http.StatusOK, result)
	}
}

func (r repository) bookingLineage(ctx context.Context, merchant, eventID string) (bookingLineage, error) {
	result := bookingLineage{Outcomes: []bookingOutcome{}}
	result.Booking.MerchantID = merchant
	result.Booking.EventID = eventID
	err := pgx.BeginTxFunc(ctx, r.db, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		booking := &result.Booking
		err := tx.QueryRow(ctx, `
			SELECT e.schema_version, e.stage, e.occurred_at, e.amount_minor, e.product, e.sdk_status, e.received_at, a.result
			FROM booking_events e
			LEFT JOIN booking_assessments a USING (merchant_id, event_id)
			WHERE e.merchant_id = $1 AND e.event_id = $2::text::uuid`, merchant, eventID).Scan(&booking.SchemaVersion,
			&booking.Stage, &booking.OccurredAt, &booking.AmountMinor, &booking.Product, &booking.SDKStatus,
			&booking.ReceivedAt, &result.Assessment)
		if err != nil {
			return err
		}
		if booking.SDKStatus == "" {
			booking.SDKStatus = "unknown"
		}
		booking.OccurredAt = booking.OccurredAt.UTC()
		booking.ReceivedAt = booking.ReceivedAt.UTC()

		rows, err := tx.Query(ctx, `
			SELECT outcome_id::text, source, source_reference, label, occurred_at, received_at, eligible
			FROM booking_outcomes WHERE merchant_id = $1 AND event_id = $2::text::uuid
			ORDER BY occurred_at, received_at, outcome_id`, merchant, eventID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			outcome := bookingOutcome{EventID: eventID}
			if err := rows.Scan(&outcome.OutcomeID, &outcome.Source, &outcome.SourceReference, &outcome.Label,
				&outcome.OccurredAt, &outcome.ReceivedAt, &outcome.Eligible); err != nil {
				return err
			}
			outcome.OccurredAt = outcome.OccurredAt.UTC()
			outcome.ReceivedAt = outcome.ReceivedAt.UTC()
			result.Outcomes = append(result.Outcomes, outcome)
		}
		return rows.Err()
	})
	return result, err
}
