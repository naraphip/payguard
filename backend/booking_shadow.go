package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxBookingShadowBookings = 1000

var errBookingShadowWindowTooLarge = errors.New("holdout contains more than 1000 bookings; use a shorter interval")

type bookingShadowWindow struct {
	HoldoutStart    time.Time `json:"holdoutStart"`
	HoldoutEnd      time.Time `json:"holdoutEnd"`
	AsOf            time.Time `json:"asOf"`
	ReviewThreshold int64     `json:"reviewThreshold"`
}

func (w bookingShadowWindow) valid(now time.Time) bool {
	return validBookingTime(w.HoldoutStart) && validBookingTime(w.HoldoutEnd) && validBookingTime(w.AsOf) &&
		w.HoldoutStart.Before(w.HoldoutEnd) && !w.HoldoutEnd.After(w.AsOf) && !w.AsOf.After(now) &&
		w.ReviewThreshold > 0
}

type bookingShadowMetrics struct {
	TP     int      `json:"tp"`
	TN     int      `json:"tn"`
	FP     int      `json:"fp"`
	FN     int      `json:"fn"`
	FPR    *float64 `json:"fpr"`
	Recall *float64 `json:"recall"`
}

func (m *bookingShadowMetrics) observe(fraud, review bool) {
	switch {
	case fraud && review:
		m.TP++
	case fraud:
		m.FN++
	case review:
		m.FP++
	default:
		m.TN++
	}
}

func (m *bookingShadowMetrics) finish() {
	if negatives := m.FP + m.TN; negatives > 0 {
		ratio := float64(m.FP) / float64(negatives)
		m.FPR = &ratio
	}
	if positives := m.TP + m.FN; positives > 0 {
		ratio := float64(m.TP) / float64(positives)
		m.Recall = &ratio
	}
}

type bookingShadowExcluded struct {
	AssessmentUnavailable int `json:"assessmentUnavailable"`
	LabelUnavailable      int `json:"labelUnavailable"`
	AmbiguousLabel        int `json:"ambiguousLabel"`
	BaselineOverlap       int `json:"baselineOverlap"`
	Unresolved            int `json:"unresolved"`
	EvidenceUnavailable   int `json:"evidenceUnavailable"`
}

type bookingShadowLineage struct {
	ListVersion     string  `json:"listVersion,omitempty"`
	ProfileVersion  string  `json:"profileVersion"`
	RuleVersion     string  `json:"ruleVersion"`
	JevRuleVersion  string  `json:"jevRuleVersion,omitempty"`
	BaselineVersion *string `json:"baselineVersion"`
	ModelVersion    string  `json:"modelVersion"`
	RubricVersion   string  `json:"rubricVersion"`
}

type bookingShadowReport struct {
	Window   bookingShadowWindow    `json:"window"`
	Bookings int                    `json:"bookings"`
	Excluded bookingShadowExcluded  `json:"excluded"`
	Stored   bookingShadowMetrics   `json:"stored"`
	Shadow   bookingShadowMetrics   `json:"shadow"`
	Lineage  []bookingShadowLineage `json:"lineage"`
}

type bookingShadowSample struct {
	raw           json.RawMessage
	label         string
	labelCount    int
	baselineValid bool
}

func (r *bookingShadowReport) include(sample bookingShadowSample) error {
	r.Bookings++
	if len(sample.raw) == 0 {
		r.Excluded.AssessmentUnavailable++
		return nil
	}
	var assessment bookingAssessment
	decoder := json.NewDecoder(bytes.NewReader(sample.raw))
	decoder.UseNumber()
	if err := decoder.Decode(&assessment); err != nil {
		return err
	}
	if assessment.AssessedAt == nil || assessment.AssessedAt.After(r.Window.AsOf) {
		r.Excluded.AssessmentUnavailable++
		return nil
	}
	if assessment.Outcome == "UNRESOLVED" {
		r.Excluded.Unresolved++
		return nil
	}
	if sample.labelCount == 0 {
		r.Excluded.LabelUnavailable++
		return nil
	}
	if sample.labelCount != 1 {
		r.Excluded.AmbiguousLabel++
		return nil
	}
	if !sample.baselineValid {
		r.Excluded.BaselineOverlap++
		return nil
	}
	velocity, ok := pinnedShadowVelocity(assessment)
	if !ok || (assessment.Outcome != "REVIEW" && assessment.Outcome != "LOW_RISK") ||
		assessment.ProfileVersion == "" || assessment.RuleVersion == "" ||
		assessment.Jev.ModelVersion == "" || assessment.Jev.RubricVersion == "" {
		r.Excluded.EvidenceUnavailable++
		return nil
	}

	fraud := sample.label == "fraud"
	r.Stored.observe(fraud, assessment.Outcome == "REVIEW")
	otherReviewRuleMatched := slices.ContainsFunc(assessment.RuleHits, func(hit string) bool {
		return hit == bookingProductListRuleCode || strings.HasPrefix(hit, jevRuleIDPrefix)
	})
	r.Shadow.observe(fraud, velocity >= r.Window.ReviewThreshold || otherReviewRuleMatched)

	lineage := bookingShadowLineage{
		ProfileVersion: assessment.ProfileVersion, RuleVersion: assessment.RuleVersion,
		JevRuleVersion: assessment.JevRuleVersion,
		ModelVersion:   assessment.Jev.ModelVersion, RubricVersion: assessment.Jev.RubricVersion,
		BaselineVersion: assessment.Baseline.Version, ListVersion: assessment.ListVersion,
	}
	for _, existing := range r.Lineage {
		if existing.ProfileVersion == lineage.ProfileVersion && existing.RuleVersion == lineage.RuleVersion &&
			existing.JevRuleVersion == lineage.JevRuleVersion &&
			existing.ModelVersion == lineage.ModelVersion && existing.RubricVersion == lineage.RubricVersion && existing.ListVersion == lineage.ListVersion &&
			sameShadowBaselineVersion(existing.BaselineVersion, lineage.BaselineVersion) {
			return nil
		}
	}
	r.Lineage = append(r.Lineage, lineage)
	return nil
}

func sameShadowBaselineVersion(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func pinnedShadowVelocity(assessment bookingAssessment) (int64, bool) {
	for _, feature := range assessment.Features {
		if feature.Name != "bookingVelocity60s" || feature.Presence != "OBSERVED" ||
			feature.Source != "BOOKING_EVENT_HISTORY" {
			continue
		}
		number, ok := feature.Value.(json.Number)
		if !ok {
			break
		}
		value, err := number.Int64()
		return value, err == nil && value > 0
	}
	return 0, false
}

func (h bookingRiskHandler) shadowBookings(w http.ResponseWriter, request *http.Request) {
	merchant := h.auth.requireMerchant(w, request)
	if merchant == "" {
		return
	}
	var window bookingShadowWindow
	if !decodeBookingRiskJSON(w, request, &window) {
		return
	}
	if !window.valid(time.Now()) {
		problem(w, http.StatusUnprocessableEntity, "Expected RFC3339 timestamps with holdoutStart < holdoutEnd <= asOf <= now and a positive reviewThreshold.")
		return
	}
	window.HoldoutStart = window.HoldoutStart.UTC()
	window.HoldoutEnd = window.HoldoutEnd.UTC()
	window.AsOf = window.AsOf.UTC()
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	result, err := h.repo.bookingShadow(ctx, merchant, window)
	switch {
	case errors.Is(err, errBookingShadowWindowTooLarge):
		problem(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		bookingRiskUnavailable(w, err)
	default:
		respond(w, http.StatusOK, result)
	}
}

func (r repository) bookingShadow(ctx context.Context, merchant string, window bookingShadowWindow) (bookingShadowReport, error) {
	report := bookingShadowReport{Window: window, Lineage: []bookingShadowLineage{}}
	err := pgx.BeginTxFunc(ctx, r.db, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		// Derive the holdout from server receipt time to prevent backdated events from
		// entering a closed evaluation window. Aggregate votes before joining them to bookings.
		rows, err := tx.Query(ctx, `
			WITH holdout AS MATERIALIZED (
				SELECT event_id, received_at, baseline_id FROM booking_events
				WHERE merchant_id = $1 AND received_at >= $2 AND received_at < $3
				ORDER BY received_at, event_id LIMIT $5
			), pinned AS MATERIALIZED (
				SELECT b.id, b.created_at < $2 AND NOT EXISTS (
					SELECT 1 FROM unnest(b.sample_event_ids) AS sample(event_id)
					WHERE NOT EXISTS (
						SELECT 1 FROM booking_events AS e
						JOIN booking_outcomes AS o USING (merchant_id, event_id)
						WHERE e.merchant_id = $1 AND e.event_id = sample.event_id
						AND e.received_at < $2 AND e.occurred_at < $2
						AND o.eligible AND o.source = 'verified_demo' AND o.label = 'normal'
						AND o.received_at < $2 AND o.occurred_at < $2
					)
				) AS valid FROM booking_baselines AS b
				WHERE b.merchant_id = $1 AND b.id IN (SELECT baseline_id FROM holdout)
			)
			SELECT a.result, labels.vote_count, COALESCE(labels.label, ''),
				h.baseline_id IS NULL OR COALESCE(p.valid, false)
			FROM holdout AS h
			LEFT JOIN booking_assessments AS a ON a.merchant_id = $1 AND a.event_id = h.event_id
			LEFT JOIN pinned AS p ON p.id = h.baseline_id
			CROSS JOIN LATERAL (
				SELECT count(DISTINCT o.label) AS vote_count, min(o.label) AS label
				FROM booking_outcomes AS o
				WHERE o.merchant_id = $1 AND o.event_id = h.event_id
				AND o.source = 'verified_demo' AND o.label IN ('normal', 'fraud')
				AND o.received_at <= $4 AND o.occurred_at <= $4
			) AS labels
			ORDER BY h.received_at, h.event_id`, merchant, window.HoldoutStart, window.HoldoutEnd,
			window.AsOf, maxBookingShadowBookings+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if report.Bookings >= maxBookingShadowBookings {
				return errBookingShadowWindowTooLarge
			}
			var sample bookingShadowSample
			if err := rows.Scan(&sample.raw, &sample.labelCount, &sample.label, &sample.baselineValid); err != nil {
				return err
			}
			if err := report.include(sample); err != nil {
				return err
			}
		}
		return rows.Err()
	})
	if err != nil {
		return bookingShadowReport{}, err
	}
	report.Stored.finish()
	report.Shadow.finish()
	return report, nil
}
