package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	outcomeReferencePattern    = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,128}$`)
	errOutcomeConflict         = errors.New("outcome ID or source reference already belongs to a different import")
	errVerifiedOutcomeConflict = errors.New("booking already has a different conclusive verified demo label")
	errOutcomeBeforeBooking    = errors.New("outcome occurredAt must be on or after booking occurredAt")
	errOutcomeNeedsAssessment  = errors.New("assess the booking before importing a verified demo normal outcome")
)

type bookingOutcomeInput struct {
	OutcomeID       string    `json:"outcomeId"`
	Source          string    `json:"source"`
	SourceReference string    `json:"sourceReference"`
	Label           string    `json:"label"`
	OccurredAt      time.Time `json:"occurredAt"`
}

type bookingOutcome struct {
	bookingOutcomeInput
	EventID    string    `json:"eventId"`
	ReceivedAt time.Time `json:"receivedAt"`
	Eligible   bool      `json:"eligible"`
}

type bookingOutcomeImport struct {
	Outcome  bookingOutcome  `json:"outcome"`
	Baseline bookingBaseline `json:"baseline"`
}

func (input bookingOutcomeInput) valid() bool {
	return validID.MatchString(input.OutcomeID) &&
		(input.Source == "verified_demo" || input.Source == "analyst" || input.Source == "chargeback") &&
		outcomeReferencePattern.MatchString(input.SourceReference) &&
		(input.Label == "normal" || input.Label == "fraud" || input.Label == "inconclusive") &&
		validBookingTime(input.OccurredAt)
}

func (input bookingOutcomeInput) verifiedNormal() bool {
	return input.Source == "verified_demo" && input.Label == "normal"
}

func baselineSummary(id *int64, count int) bookingBaseline {
	summary := bookingBaseline{State: "COLD_START", SampleCount: &count}
	if id != nil {
		version := fmt.Sprintf("baseline-v1-%d", *id)
		summary.State = "ACTIVE"
		summary.Version = &version
	}
	return summary
}

func (h bookingRiskHandler) importOutcome(w http.ResponseWriter, r *http.Request) {
	merchant := h.auth.requireMerchant(w, r)
	if merchant == "" {
		return
	}
	eventID := r.PathValue("id")
	if !validID.MatchString(eventID) {
		problem(w, http.StatusBadRequest, "Expected a lowercase booking event UUID.")
		return
	}
	var input bookingOutcomeInput
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if !input.valid() {
		problem(w, http.StatusUnprocessableEntity, "Expected a lowercase outcome UUID, supported source and label, source reference, and RFC3339 occurredAt with microsecond precision.")
		return
	}
	result, created, err := h.repo.saveBookingOutcome(r.Context(), merchant, eventID, input, h.baselineMinSamples)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Booking event not found.")
	case errors.Is(err, errOutcomeConflict), errors.Is(err, errVerifiedOutcomeConflict), errors.Is(err, errOutcomeNeedsAssessment):
		problem(w, http.StatusConflict, err.Error())
	case errors.Is(err, errOutcomeBeforeBooking):
		problem(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		bookingRiskUnavailable(w, err)
	case created:
		respond(w, http.StatusCreated, result)
	default:
		respond(w, http.StatusOK, result)
	}
}

func (r repository) saveBookingOutcome(ctx context.Context, merchant, eventID string, input bookingOutcomeInput, minSamples int) (bookingOutcomeImport, bool, error) {
	var result bookingOutcomeImport
	var created bool
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockBookingMerchant(ctx, tx, merchant); err != nil {
			return err
		}
		var occurredAt time.Time
		var mode *bookingJevMode
		err := tx.QueryRow(ctx, `
			SELECT e.occurred_at, a.jev_mode
			FROM booking_events e
			LEFT JOIN booking_assessments a USING (merchant_id, event_id)
			WHERE e.merchant_id = $1 AND e.event_id = $2::text::uuid`, merchant, eventID).Scan(&occurredAt, &mode)
		if err != nil {
			return err
		}
		replayed, found, err := replayBookingOutcome(ctx, tx, merchant, eventID, input)
		if err != nil {
			return err
		}
		if found {
			result = replayed
			return nil
		}
		if input.OccurredAt.Before(occurredAt) {
			return errOutcomeBeforeBooking
		}
		if input.verifiedNormal() && mode == nil {
			return errOutcomeNeedsAssessment
		}
		if input.Source == "verified_demo" && input.Label != "inconclusive" {
			var conflicting bool
			err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM booking_outcomes
					WHERE merchant_id = $1 AND event_id = $2::text::uuid
					AND source = 'verified_demo' AND label IN ('normal','fraud') AND label <> $3)`,
				merchant, eventID, input.Label).Scan(&conflicting)
			if err != nil {
				return err
			}
			if conflicting {
				return errVerifiedOutcomeConflict
			}
		}

		outcome := bookingOutcome{bookingOutcomeInput: input, EventID: eventID, Eligible: input.verifiedNormal() && mode != nil && *mode == bookingJevMock}
		outcome.OccurredAt = outcome.OccurredAt.UTC()
		baselineID, count, err := resolveOutcomeBaseline(ctx, tx, merchant, outcome, minSamples)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO booking_outcomes
				(merchant_id, outcome_id, event_id, source, source_reference, label, occurred_at, eligible, baseline_id, baseline_sample_count)
			VALUES ($1, $2::text::uuid, $3::text::uuid, $4, $5, $6, $7, $8, $9, $10)
			RETURNING received_at`, merchant, input.OutcomeID, eventID, input.Source, input.SourceReference,
			input.Label, input.OccurredAt, outcome.Eligible, baselineID, count).Scan(&outcome.ReceivedAt)
		if err != nil {
			return err
		}
		outcome.ReceivedAt = outcome.ReceivedAt.UTC()
		result = bookingOutcomeImport{Outcome: outcome, Baseline: baselineSummary(baselineID, count)}
		created = true
		return nil
	})
	return result, created, err
}

func replayBookingOutcome(ctx context.Context, tx pgx.Tx, merchant, eventID string, input bookingOutcomeInput) (bookingOutcomeImport, bool, error) {
	var stored bookingOutcome
	var baselineID *int64
	var count int
	err := tx.QueryRow(ctx, `
		SELECT outcome_id::text, event_id::text, source, source_reference, label, occurred_at, received_at, eligible,
			baseline_id, baseline_sample_count
		FROM booking_outcomes
		WHERE merchant_id = $1 AND (outcome_id = $2::text::uuid OR (source = $3 AND source_reference = $4))`,
		merchant, input.OutcomeID, input.Source, input.SourceReference).Scan(&stored.OutcomeID, &stored.EventID,
		&stored.Source, &stored.SourceReference, &stored.Label, &stored.OccurredAt, &stored.ReceivedAt, &stored.Eligible,
		&baselineID, &count)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingOutcomeImport{}, false, nil
	}
	if err != nil {
		return bookingOutcomeImport{}, false, err
	}
	if stored.EventID != eventID || stored.OutcomeID != input.OutcomeID || stored.Source != input.Source ||
		stored.SourceReference != input.SourceReference || stored.Label != input.Label || !stored.OccurredAt.Equal(input.OccurredAt) {
		return bookingOutcomeImport{}, false, errOutcomeConflict
	}
	stored.OccurredAt = stored.OccurredAt.UTC()
	stored.ReceivedAt = stored.ReceivedAt.UTC()
	return bookingOutcomeImport{Outcome: stored, Baseline: baselineSummary(baselineID, count)}, true, nil
}

func resolveOutcomeBaseline(ctx context.Context, tx pgx.Tx, merchant string, outcome bookingOutcome, minSamples int) (*int64, int, error) {
	var activeID int64
	var activeCount int
	err := tx.QueryRow(ctx, `
		SELECT id, sample_count FROM booking_baselines
		WHERE merchant_id = $1 ORDER BY id DESC LIMIT 1`, merchant).Scan(&activeID, &activeCount)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, err
	}
	var eventIDs []string
	err = tx.QueryRow(ctx, `
		SELECT coalesce(array_agg(DISTINCT event_id::text ORDER BY event_id::text), ARRAY[]::text[])
		FROM booking_outcomes WHERE merchant_id = $1 AND eligible`, merchant).Scan(&eventIDs)
	if err != nil {
		return nil, 0, err
	}
	if outcome.Eligible && !slices.Contains(eventIDs, outcome.EventID) {
		eventIDs = append(eventIDs, outcome.EventID)
		if minSamples > 0 && len(eventIDs) >= minSamples {
			slices.Sort(eventIDs)
			nextID := activeID + 1
			_, err := tx.Exec(ctx, `
				INSERT INTO booking_baselines (merchant_id, id, sample_count, sample_event_ids)
				VALUES ($1, $2, $3, $4::text[]::uuid[])`, merchant, nextID, len(eventIDs), eventIDs)
			return &nextID, len(eventIDs), err
		}
	}
	if activeID != 0 {
		return &activeID, activeCount, nil
	}
	return nil, len(eventIDs), nil
}
