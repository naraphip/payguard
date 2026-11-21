package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const (
	bookingRuleVersion             = "booking-rules-v1"
	bookingRubricVersion           = "booking-risk-rubric-v1"
	bookingVelocityWindow          = time.Minute
	bookingVelocityReviewThreshold = 5
)

var (
	errNoActiveBookingProfile        = errors.New("approve a scoring profile before assessing a booking")
	errBookingAssessmentModeConflict = errors.New("booking has already been assessed using a different Jev mode")
)

type bookingJevMode string

const (
	bookingJevMock    bookingJevMode = "mock"
	bookingJevTimeout bookingJevMode = "timeout"
)

type bookingJevStatus string

const (
	bookingJevStatusMock    bookingJevStatus = "MOCK"
	bookingJevStatusTimeout bookingJevStatus = "TIMEOUT"
)

type bookingJevAnswer string

const (
	bookingJevLow         bookingJevAnswer = "LOW"
	bookingJevElevated    bookingJevAnswer = "ELEVATED"
	bookingJevUnavailable bookingJevAnswer = "UNAVAILABLE"
)

type bookingJevQuestion string

const bookingRiskQuestion bookingJevQuestion = "BOOKING_RISK_LEVEL"

type bookingJevResult struct {
	Question      bookingJevQuestion `json:"question"`
	Status        bookingJevStatus   `json:"status"`
	Answer        bookingJevAnswer   `json:"answer"`
	ModelVersion  string             `json:"modelVersion"`
	RubricVersion string             `json:"rubricVersion"`
}

type bookingFeature struct {
	Name     string `json:"name"`
	Value    any    `json:"value"`
	Presence string `json:"presence"`
	Source   string `json:"source"`
}

type bookingBaseline struct {
	State       string  `json:"state"`
	Version     *string `json:"version"`
	SampleCount *int    `json:"sampleCount,omitempty"`
}

type bookingAssessment struct {
	EventID        string           `json:"eventId"`
	AssessedAt     *time.Time       `json:"assessedAt,omitempty"`
	ProfileVersion string           `json:"profileVersion"`
	RuleVersion    string           `json:"ruleVersion"`
	Baseline       bookingBaseline  `json:"baseline"`
	Features       []bookingFeature `json:"features"`
	Jev            bookingJevResult `json:"jev"`
	Outcome        string           `json:"outcome"`
	Handoff        string           `json:"handoff"`
	RuleHits       []string         `json:"ruleHits"`
}

func evaluateBooking(booking bookingEvent, version string, profile profileCandidate, velocity int64, mode bookingJevMode) bookingAssessment {
	result := bookingAssessment{
		EventID:        booking.EventID,
		ProfileVersion: version,
		RuleVersion:    bookingRuleVersion,
		Baseline:       bookingBaseline{State: "COLD_START"},
		Features: []bookingFeature{
			{bookingTimestamp, booking.OccurredAt.UTC().Format(time.RFC3339Nano), "OBSERVED", "BOOKING_EVENT"},
			{"bookingVelocity60s", velocity, "OBSERVED", "BOOKING_EVENT_HISTORY"},
		},
		Jev: bookingJevResult{
			Question: bookingRiskQuestion, Status: bookingJevStatusMock, Answer: bookingJevLow,
			ModelVersion: jevVersion, RubricVersion: bookingRubricVersion,
		},
		Outcome:  "LOW_RISK",
		Handoff:  "NONE",
		RuleHits: []string{},
	}
	if slices.Contains(profile.Signals, bookingAmount) {
		amount := bookingFeature{Name: bookingAmount, Presence: "MISSING", Source: "BOOKING_EVENT"}
		if booking.AmountMinor != nil {
			amount.Value = *booking.AmountMinor
			amount.Presence = "OBSERVED"
		}
		result.Features = append(result.Features, amount)
	}
	if velocity >= bookingVelocityReviewThreshold {
		result.RuleHits = append(result.RuleHits, "BOOKING_VELOCITY_60S")
		result.Jev.Answer = bookingJevElevated
		result.Outcome = "REVIEW"
		result.Handoff = "MANUAL_REVIEW"
	}
	if mode == bookingJevTimeout {
		result.Jev.Status = bookingJevStatusTimeout
		result.Jev.Answer = bookingJevUnavailable
		result.Outcome = "UNRESOLVED"
		result.Handoff = "MANUAL_REVIEW"
	}
	return result
}

func (h bookingRiskHandler) assessEvent(w http.ResponseWriter, r *http.Request) {
	merchant := h.auth.requireMerchant(w, r)
	if merchant == "" {
		return
	}
	eventID := r.PathValue("id")
	if !validID.MatchString(eventID) {
		problem(w, http.StatusBadRequest, "Expected a lowercase booking event UUID.")
		return
	}
	var input struct {
		JevMode bookingJevMode `json:"jevMode"`
	}
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if input.JevMode != bookingJevMock && input.JevMode != bookingJevTimeout {
		problem(w, http.StatusUnprocessableEntity, "Expected jevMode mock or timeout.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, created, err := h.repo.assessBooking(ctx, h.redis, merchant, eventID, input.JevMode)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Booking event not found.")
	case errors.Is(err, errNoActiveBookingProfile), errors.Is(err, errBookingAssessmentModeConflict):
		problem(w, http.StatusConflict, err.Error())
	case err != nil:
		bookingRiskUnavailable(w, err)
	case created:
		respond(w, http.StatusCreated, result)
	default:
		respond(w, http.StatusOK, result)
	}
}

func (r repository) assessBooking(ctx context.Context, cache *redis.Client, merchant, eventID string, mode bookingJevMode) (json.RawMessage, bool, error) {
	booking := bookingEvent{MerchantID: merchant}
	booking.EventID = eventID
	var result json.RawMessage
	var created bool
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		// Serialize first assessments of the same booking before pinning the profile.
		var baselineID *int64
		var sampleCount *int
		err := tx.QueryRow(ctx, `
			SELECT e.stage, e.occurred_at, e.amount_minor, e.received_at, b.id, b.sample_count
			FROM booking_events e
			LEFT JOIN booking_baselines b ON e.merchant_id = b.merchant_id AND e.baseline_id = b.id
			WHERE e.merchant_id = $1 AND e.event_id = $2::text::uuid
			FOR UPDATE OF e`, merchant, eventID).Scan(&booking.Stage, &booking.OccurredAt, &booking.AmountMinor, &booking.ReceivedAt, &baselineID, &sampleCount)
		if err != nil {
			return err
		}
		var savedMode bookingJevMode
		err = tx.QueryRow(ctx, `
			SELECT jev_mode, result FROM booking_assessments
			WHERE merchant_id = $1 AND event_id = $2::text::uuid`, merchant, eventID).Scan(&savedMode, &result)
		if err == nil {
			if savedMode != mode {
				return errBookingAssessmentModeConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		var version string
		var profile bookingProfile
		err = tx.QueryRow(ctx, `
			SELECT p.version, p.approved_profile
			FROM booking_active_profiles a JOIN booking_profiles p USING (merchant_id, version)
			WHERE a.merchant_id = $1`, merchant).Scan(&version, &profile)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoActiveBookingProfile
		}
		if err != nil {
			return err
		}
		velocity, err := observedBookingVelocity(ctx, tx, cache, booking)
		if err != nil {
			return err
		}
		computed := evaluateBooking(booking, version, profile.Candidate, velocity, mode)
		assessedAt := time.Now().UTC().Truncate(time.Microsecond)
		computed.AssessedAt = &assessedAt
		if baselineID != nil {
			computed.Baseline = baselineSummary(baselineID, *sampleCount)
		}
		raw, err := json.Marshal(computed)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO booking_assessments (merchant_id, event_id, profile_version, jev_mode, result)
			VALUES ($1, $2::text::uuid, $3, $4, $5::jsonb)
			RETURNING result`, merchant, eventID, version, mode, raw).Scan(&result)
		created = err == nil
		return err
	})
	return result, created, err
}

func observedBookingVelocity(ctx context.Context, tx pgx.Tx, cache *redis.Client, booking bookingEvent) (int64, error) {
	rows, err := tx.Query(ctx, `
		SELECT event_id::text, occurred_at
		FROM booking_events
		WHERE merchant_id = $1 AND stage = $2 AND occurred_at <= $3 AND received_at <= $4
		  AND occurred_at > $5 AND event_id <> $6::text::uuid`,
		booking.MerchantID, booking.Stage, booking.OccurredAt, booking.ReceivedAt,
		booking.OccurredAt.Add(-bookingVelocityWindow), booking.EventID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var prior []redis.Z
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return 0, err
		}
		// Relative microsecond scores avoid float precision loss for distant dates.
		prior = append(prior, redis.Z{Member: id, Score: float64(at.Sub(booking.OccurredAt).Microseconds())})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	attempt, err := attemptID()
	if err != nil {
		return 0, err
	}
	velocity := redisVelocity{client: cache, prefix: "payguard:booking:" + attempt}
	key := velocity.key(booking.Stage, entity{Kind: "merchant", Key: booking.MerchantID})
	pipe := cache.TxPipeline()
	defer pipe.Discard()
	if len(prior) > 0 {
		pipe.ZAdd(ctx, key, prior...)
	}
	count := trackVelocity.Eval(ctx, pipe, []string{key}, 0, booking.EventID, -bookingVelocityWindow.Microseconds())
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return count.Int64()
}
