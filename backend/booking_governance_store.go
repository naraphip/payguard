package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

const readBookingChangesSQL = `
SELECT p.id::text, p.kind, p.maker_id, p.request_digest, p.candidate, p.shadow, p.rollback_of::text,
    p.created_at, p.expires_at, d.checker_id, d.decision, d.decided_at,
    COALESCE(a.proposal_id = p.id, false), p.expires_at <= clock_timestamp()
FROM booking_change_proposals AS p
LEFT JOIN booking_change_decisions AS d ON d.merchant_id = p.merchant_id AND d.proposal_id = p.id
LEFT JOIN booking_active_changes AS a ON a.merchant_id = p.merchant_id AND a.kind = p.kind`

type bookingChangeQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func lookupBookingChange(ctx context.Context, db bookingChangeQueryer, merchant, id string) (bookingChange, error) {
	return scanBookingChange(db.QueryRow(ctx, readBookingChangesSQL+" WHERE p.merchant_id = $1 AND p.id = $2::text::uuid", merchant, id))
}

func scanBookingChange(row pgx.Row) (bookingChange, error) {
	var c bookingChange
	var checker, decision *string
	var decidedAt *time.Time
	var expired bool
	err := row.Scan(&c.ID, &c.Kind, &c.MakerID, &c.digest, &c.Candidate, &c.Shadow, &c.Rollback,
		&c.CreatedAt, &c.ExpiresAt, &checker, &decision, &decidedAt, &c.Active, &expired)
	if err != nil {
		return bookingChange{}, err
	}
	c.CreatedAt = c.CreatedAt.UTC()
	c.ExpiresAt = c.ExpiresAt.UTC()
	switch {
	case decision != nil:
		c.Decision = &bookingChangeDecision{CheckerID: *checker, Decision: *decision, DecidedAt: decidedAt.UTC()}
		c.Status = "REJECTED"
		if *decision == "approve" {
			c.Status = "APPROVED"
			c.Revision = bookingChangeRevision(c.Kind, c.ID)
		}
	case expired:
		c.Status = "EXPIRED"
	default:
		c.Status = "PENDING"
	}
	return c, nil
}

func bookingChangeRevision(kind, id string) string {
	switch kind {
	case "profile":
		return "profile-v1-governed-" + id
	case "rule":
		return bookingRuleVersion + "-governed-" + id
	case "rubric":
		return bookingRubricVersion + "-governed-" + id
	default:
		return "booking-product-list-v1-governed-" + id
	}
}

func (r repository) saveBookingChange(ctx context.Context, actor bookingActor, input bookingChangeRequest, digest string, proof json.RawMessage) (bookingChange, bool, error) {
	var result bookingChange
	var created bool
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockBookingMerchant(ctx, tx, actor.MerchantID); err != nil {
			return err
		}
		existing, err := lookupBookingChange(ctx, tx, actor.MerchantID, input.ID)
		if err == nil {
			if existing.digest != digest || existing.MakerID != actor.ActorID {
				return errBookingChangeConflict
			}
			result = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO booking_change_proposals (merchant_id, id, kind, maker_id, request_digest, candidate, shadow, rollback_of, expires_at)
			SELECT $1, $2::text::uuid, $3, $4, $5, $6::jsonb, $7::jsonb, NULLIF($8, '')::uuid, $9
			WHERE $9 > clock_timestamp()
			RETURNING id::text`, actor.MerchantID, input.ID, input.Kind, actor.ActorID,
			digest, input.Candidate, proof, input.RollbackOf, input.ExpiresAt).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errBookingChangeExpired
		}
		if err != nil {
			return err
		}
		result, err = lookupBookingChange(ctx, tx, actor.MerchantID, id)
		created = err == nil
		return err
	})
	return result, created, err
}

func (r repository) decideBookingChange(ctx context.Context, actor bookingActor, id, decision string) (bookingChange, error) {
	var result bookingChange
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockBookingMerchant(ctx, tx, actor.MerchantID); err != nil {
			return err
		}
		c, err := lookupBookingChange(ctx, tx, actor.MerchantID, id)
		if err != nil {
			return err
		}
		if c.MakerID == actor.ActorID {
			return errBookingChangeSelf
		}
		if c.Decision != nil {
			if c.Decision.CheckerID != actor.ActorID || c.Decision.Decision != decision {
				return errBookingChangeConflict
			}
			result = c
			return nil
		}
		var recordedID string
		err = tx.QueryRow(ctx, `
			INSERT INTO booking_change_decisions (merchant_id, proposal_id, checker_id, decision)
			SELECT merchant_id, id, $3, $4 FROM booking_change_proposals
			WHERE merchant_id = $1 AND id = $2::text::uuid AND expires_at > clock_timestamp()
			RETURNING proposal_id::text`, actor.MerchantID, id, actor.ActorID, decision).Scan(&recordedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errBookingChangeExpired
		}
		if err != nil {
			return err
		}
		if decision == "approve" {
			if err := publishBookingChange(ctx, tx, actor.MerchantID, c); err != nil {
				return err
			}
		}
		result, err = lookupBookingChange(ctx, tx, actor.MerchantID, recordedID)
		return err
	})
	return result, err
}

func publishBookingChange(ctx context.Context, tx pgx.Tx, merchant string, change bookingChange) error {
	if change.Kind == "profile" {
		version := bookingChangeRevision(change.Kind, change.ID)
		if _, err := tx.Exec(ctx, `
			INSERT INTO booking_profiles (merchant_id, version, approved_profile) VALUES ($1, $2, $3::jsonb)`,
			merchant, version, change.Candidate); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO booking_active_profiles (merchant_id, version) VALUES ($1, $2)
			ON CONFLICT (merchant_id) DO UPDATE SET version = EXCLUDED.version`, merchant, version); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO booking_active_changes (merchant_id, kind, proposal_id) VALUES ($1, $2, $3::text::uuid)
		ON CONFLICT (merchant_id, kind) DO UPDATE SET proposal_id = EXCLUDED.proposal_id`,
		merchant, change.Kind, change.ID)
	return err
}

const bookingProductListRuleCode = "BOOKING_PRODUCT_REVIEW_LIST"

type bookingGovernancePolicy struct {
	rule          bookingChangeRule
	ruleVersion   string
	rubric        bookingChangeRubric
	rubricVersion string
	list          bookingChangeList
	listVersion   string
}

func loadBookingGovernance(ctx context.Context, tx pgx.Tx, merchant string) (bookingGovernancePolicy, error) {
	var policy bookingGovernancePolicy
	rows, err := tx.Query(ctx, `
		SELECT p.kind, p.id::text, p.candidate FROM booking_active_changes AS a
		JOIN booking_change_proposals AS p ON p.merchant_id = a.merchant_id AND p.id = a.proposal_id
		WHERE a.merchant_id = $1 AND a.kind IN ('rule','rubric','list')`, merchant)
	if err != nil {
		return policy, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id string
		var candidate json.RawMessage
		if err := rows.Scan(&kind, &id, &candidate); err != nil {
			return policy, err
		}
		var target any
		switch kind {
		case "rule":
			policy.ruleVersion = bookingChangeRevision(kind, id)
			target = &policy.rule
		case "rubric":
			policy.rubricVersion = bookingChangeRevision(kind, id)
			target = &policy.rubric
		case "list":
			policy.listVersion = bookingChangeRevision(kind, id)
			target = &policy.list
		}
		if err := json.Unmarshal(candidate, target); err != nil {
			return policy, err
		}
	}
	return policy, rows.Err()
}

func (p bookingGovernancePolicy) apply(result *bookingAssessment, product string, velocity int64, mode bookingJevMode) {
	if p.ruleVersion == "" && p.rubricVersion == "" && p.listVersion == "" {
		return
	}
	threshold := int64(bookingVelocityReviewThreshold)
	if p.ruleVersion != "" {
		threshold = p.rule.ReviewThreshold
		result.RuleVersion = p.ruleVersion
	}
	if p.listVersion != "" {
		result.ListVersion = p.listVersion
	}
	result.RuleHits = result.RuleHits[:0]
	if velocity >= threshold {
		result.RuleHits = append(result.RuleHits, "BOOKING_VELOCITY_60S")
	}
	if slices.Contains(p.list.ReviewProducts, product) {
		result.RuleHits = append(result.RuleHits, bookingProductListRuleCode)
	}
	if p.rubricVersion != "" {
		result.Jev.RubricVersion = p.rubricVersion
	}
	if mode == bookingJevTimeout {
		return
	}
	result.Outcome, result.Handoff = "LOW_RISK", "NONE"
	if len(result.RuleHits) > 0 {
		result.Outcome, result.Handoff = "REVIEW", "MANUAL_REVIEW"
	}
	result.Jev.Answer = bookingJevLow
	if (p.rubricVersion == "" && len(result.RuleHits) > 0) ||
		(p.rubricVersion != "" && velocity >= p.rubric.ElevatedVelocity) {
		result.Jev.Answer = bookingJevElevated
	}
}
