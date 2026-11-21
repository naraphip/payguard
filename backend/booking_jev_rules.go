package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const bookingJevRulesSchema = `
CREATE TABLE IF NOT EXISTS booking_jev_rules (
  merchant_id text NOT NULL,
  revision bigint NOT NULL CHECK (revision > 1),
  rule_id text NOT NULL,
  definition jsonb NOT NULL,
  PRIMARY KEY (merchant_id, revision),
  UNIQUE (merchant_id, rule_id)
);
ALTER TABLE booking_jev_rules
  ADD COLUMN IF NOT EXISTS target_rule_id text,
  ADD COLUMN IF NOT EXISTS deleted boolean NOT NULL DEFAULT false;
`

const (
	bookingVelocitySignal = "bookingVelocity60s"
	jevReviewAction       = "REVIEW"
	jevRuleIDPrefix       = "jev-rule-"
	jevRulePreviewLimit   = 200
	jevRuleMaxValue       = 9007199254740991
)

type jevRuleCondition struct {
	Signal   string `json:"signal"`
	Operator string `json:"operator"`
	Value    *int64 `json:"value"`
}

type jevRuleDefinition struct {
	Name       string             `json:"name"`
	Conditions []jevRuleCondition `json:"conditions"`
	Action     string             `json:"action"`
}

type publishedJevRule struct {
	ID string `json:"id"`
	jevRuleDefinition
}

type jevRulesSnapshot struct {
	Version string             `json:"version"`
	Rules   []publishedJevRule `json:"rules"`
}

var (
	errJevRulesVersion = errors.New("The Jev rules changed. Refresh the ruleset and try again.")
	errJevRuleExists   = errors.New("An active Jev rule already has this definition.")
)

type jevRulePreview struct {
	Evaluated int `json:"evaluated"`
	Matched   int `json:"matched"`
	Missing   int `json:"missing"`
}

func (d *jevRuleDefinition) validate() error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || utf8.RuneCountInString(d.Name) > 80 || strings.ContainsFunc(d.Name, unicode.IsControl) {
		return fmt.Errorf("Rule name must be 1 to 80 characters without control characters.")
	}
	if d.Action != jevReviewAction {
		return fmt.Errorf("The Jev rule action must be REVIEW.")
	}
	if len(d.Conditions) < 1 || len(d.Conditions) > 5 {
		return fmt.Errorf("A Jev rule must have 1 to 5 conditions.")
	}
	for i, condition := range d.Conditions {
		if condition.Signal != bookingAmount && condition.Signal != bookingVelocitySignal {
			return fmt.Errorf("Condition %d must use amountMinor or bookingVelocity60s.", i+1)
		}
		switch condition.Operator {
		case "gte", "gt", "lte", "lt", "eq":
		default:
			return fmt.Errorf("Condition %d must use gte, gt, lte, lt, or eq.", i+1)
		}
		if condition.Value == nil || *condition.Value < 0 || *condition.Value > jevRuleMaxValue {
			return fmt.Errorf("Condition %d must use a nonnegative integer up to %d.", i+1, jevRuleMaxValue)
		}
	}
	// The order of AND clauses has no effect on a rule's identity.
	slices.SortFunc(d.Conditions, func(a, b jevRuleCondition) int {
		if order := cmp.Compare(a.Signal, b.Signal); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Operator, b.Operator); order != 0 {
			return order
		}
		return cmp.Compare(*a.Value, *b.Value)
	})
	return nil
}

func jevRuleSignals(features []bookingFeature) map[string]int64 {
	observed := make(map[string]int64, 2)
	for _, feature := range features {
		if feature.Presence != "OBSERVED" {
			continue
		}
		if (feature.Name != bookingAmount || feature.Source != "BOOKING_EVENT") &&
			(feature.Name != bookingVelocitySignal || feature.Source != "BOOKING_EVENT_HISTORY") {
			continue
		}
		switch value := feature.Value.(type) {
		case int64:
			observed[feature.Name] = value
		case json.Number:
			if n, err := value.Int64(); err == nil {
				observed[feature.Name] = n
			}
		}
	}
	return observed
}

func (d jevRuleDefinition) matches(observed map[string]int64) (matched, missing bool) {
	matched = true
	for _, condition := range d.Conditions {
		actual, ok := observed[condition.Signal]
		if !ok {
			return false, true
		}
		expected := *condition.Value
		switch condition.Operator {
		case "gte":
			matched = matched && actual >= expected
		case "gt":
			matched = matched && actual > expected
		case "lte":
			matched = matched && actual <= expected
		case "lt":
			matched = matched && actual < expected
		case "eq":
			matched = matched && actual == expected
		}
	}
	return matched, false
}

func (assessment *bookingAssessment) applyJevRules(snapshot jevRulesSnapshot) {
	if len(snapshot.Rules) == 0 {
		return
	}
	if assessment.RuleVersion == bookingRuleVersion {
		assessment.RuleVersion = snapshot.Version
	} else {
		assessment.JevRuleVersion = snapshot.Version
	}
	observed := jevRuleSignals(assessment.Features)
	for _, rule := range snapshot.Rules {
		if matched, _ := rule.matches(observed); !matched {
			continue
		}
		assessment.RuleHits = append(assessment.RuleHits, rule.ID)
		if assessment.Jev.Status != bookingJevStatusTimeout {
			if assessment.Jev.RubricVersion == bookingRubricVersion {
				assessment.Jev.Answer = bookingJevElevated
			}
			assessment.Outcome = "REVIEW"
			assessment.Handoff = "MANUAL_REVIEW"
		}
	}
}

type jevRuleReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readJevRules(ctx context.Context, db jevRuleReader, merchant string) (jevRulesSnapshot, error) {
	snapshot := jevRulesSnapshot{Version: bookingRuleVersion, Rules: []publishedJevRule{}}
	rows, err := db.Query(ctx, `
		SELECT revision, rule_id, definition, target_rule_id, deleted FROM booking_jev_rules
		WHERE merchant_id = $1 ORDER BY revision`, merchant)
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	active := map[string]publishedJevRule{}
	var order []string
	var revision int64
	for rows.Next() {
		var rule publishedJevRule
		var targetID *string
		var deleted bool
		if err := rows.Scan(&revision, &rule.ID, &rule.jevRuleDefinition, &targetID, &deleted); err != nil {
			return snapshot, err
		}
		if targetID == nil {
			order = append(order, rule.ID)
		} else {
			rule.ID = *targetID
		}
		if deleted {
			delete(active, rule.ID)
		} else {
			active[rule.ID] = rule
		}
	}
	for _, id := range order {
		if rule, ok := active[id]; ok {
			snapshot.Rules = append(snapshot.Rules, rule)
		}
	}
	if revision > 1 {
		snapshot.Version = fmt.Sprintf("booking-rules-v%d", revision)
	}
	return snapshot, rows.Err()
}

func (r repository) publishJevRule(ctx context.Context, merchant string, definition jevRuleDefinition) (jevRulesSnapshot, error) {
	var snapshot jevRulesSnapshot
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		// Serialize appends for this merchant so every revision identifies an immutable prefix.
		if err := lockBookingMerchant(ctx, tx, merchant); err != nil {
			return err
		}
		current, err := readJevRules(ctx, tx, merchant)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(current.Rules, func(rule publishedJevRule) bool {
			return reflect.DeepEqual(rule.jevRuleDefinition, definition)
		}) {
			snapshot = current
			return nil
		}
		if err := appendJevRule(ctx, tx, merchant, "", definition, false); err != nil {
			return err
		}
		snapshot, err = readJevRules(ctx, tx, merchant)
		return err
	})
	return snapshot, err
}

func appendJevRule(ctx context.Context, tx pgx.Tx, merchant, targetID string, definition jevRuleDefinition, deleted bool) error {
	raw, err := json.Marshal(definition)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	id := jevRuleIDPrefix + hex.EncodeToString(digest[:])
	_, err = tx.Exec(ctx, `
		WITH next AS (
			SELECT COALESCE(MAX(revision), 1) + 1 AS revision
			FROM booking_jev_rules WHERE merchant_id = $1
		)
		INSERT INTO booking_jev_rules (merchant_id, revision, rule_id, definition, target_rule_id, deleted)
		SELECT $1, next.revision,
			CASE WHEN $4::text <> '' THEN 'jev-change-v' || next.revision
				WHEN EXISTS (SELECT 1 FROM booking_jev_rules WHERE merchant_id = $1 AND rule_id = $2)
				THEN $2 || '-v' || next.revision
				ELSE $2 END,
			$3::jsonb, NULLIF($4::text, ''), $5
		FROM next`, merchant, id, raw, targetID, deleted)
	return err
}

func (r repository) reviseMerchantJevRule(ctx context.Context, merchant, targetID, expectedVersion string, definition *jevRuleDefinition) (jevRulesSnapshot, error) {
	var snapshot jevRulesSnapshot
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockMockMerchant(ctx, tx, merchant); err != nil {
			return err
		}
		if err := lockBookingMerchant(ctx, tx, merchant); err != nil {
			return err
		}
		current, err := readJevRules(ctx, tx, merchant)
		if err != nil {
			return err
		}
		if current.Version != expectedVersion {
			return errJevRulesVersion
		}
		index := slices.IndexFunc(current.Rules, func(rule publishedJevRule) bool { return rule.ID == targetID })
		if index < 0 {
			return pgx.ErrNoRows
		}
		revision := current.Rules[index].jevRuleDefinition
		event := "MERCHANT_JEV_RULE_REMOVED"
		if definition != nil {
			if reflect.DeepEqual(revision, *definition) {
				snapshot = current
				return nil
			}
			if slices.ContainsFunc(current.Rules, func(rule publishedJevRule) bool {
				return rule.ID != targetID && reflect.DeepEqual(rule.jevRuleDefinition, *definition)
			}) {
				return errJevRuleExists
			}
			revision = *definition
			event = "MERCHANT_JEV_RULE_UPDATED"
		}
		if err := appendJevRule(ctx, tx, merchant, targetID, revision, definition == nil); err != nil {
			return err
		}
		if err := auditMerchant(ctx, tx, merchant, event); err != nil {
			return err
		}
		snapshot, err = readJevRules(ctx, tx, merchant)
		return err
	})
	return snapshot, err
}

func (r repository) previewJevRule(ctx context.Context, merchant string, definition jevRuleDefinition) (jevRulePreview, error) {
	var preview jevRulePreview
	rows, err := r.db.Query(ctx, `
		SELECT result->'features' FROM booking_assessments WHERE merchant_id = $1
		ORDER BY (result->>'assessedAt')::timestamptz DESC NULLS LAST, event_id DESC LIMIT $2`, merchant, jevRulePreviewLimit)
	if err != nil {
		return preview, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return preview, err
		}
		var features []bookingFeature
		if len(raw) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			if err := decoder.Decode(&features); err != nil {
				return preview, err
			}
		}
		matched, missing := definition.matches(jevRuleSignals(features))
		preview.Evaluated++
		if matched {
			preview.Matched++
		}
		if missing {
			preview.Missing++
		}
	}
	return preview, rows.Err()
}

func (h bookingRiskHandler) listJevRules(w http.ResponseWriter, request *http.Request) {
	merchant := h.auth.requireMerchant(w, request)
	if merchant == "" {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	snapshot, err := readJevRules(ctx, h.repo.db, merchant)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, snapshot)
}

func decodeJevRule(w http.ResponseWriter, request *http.Request, definition *jevRuleDefinition) bool {
	if !decodeBookingRiskJSON(w, request, definition) {
		return false
	}
	if err := definition.validate(); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return false
	}
	return true
}

func (h bookingRiskHandler) pushJevRule(w http.ResponseWriter, request *http.Request) {
	merchant := h.auth.requireMerchant(w, request)
	if merchant == "" {
		return
	}
	var definition jevRuleDefinition
	if !decodeJevRule(w, request, &definition) {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	snapshot, err := h.repo.publishJevRule(ctx, merchant, definition)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, snapshot)
}

func (h bookingRiskHandler) testJevRule(w http.ResponseWriter, request *http.Request) {
	merchant := h.auth.requireMerchant(w, request)
	if merchant == "" {
		return
	}
	var definition jevRuleDefinition
	if !decodeJevRule(w, request, &definition) {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	preview, err := h.repo.previewJevRule(ctx, merchant, definition)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, preview)
}
