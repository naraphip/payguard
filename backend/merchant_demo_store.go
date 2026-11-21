package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const merchantDemoSchema = `
CREATE TABLE IF NOT EXISTS merchant_demo_merchants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32)
);
ALTER TABLE merchant_demo_merchants
  ADD COLUMN IF NOT EXISTS setup jsonb,
  ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE TABLE IF NOT EXISTS merchant_demo_scenarios (
  merchant_id uuid NOT NULL REFERENCES merchant_demo_merchants(id),
  id uuid NOT NULL,
  revision integer NOT NULL CHECK (revision > 0),
  name text NOT NULL,
  threshold_minor bigint NOT NULL CHECK (threshold_minor BETWEEN 0 AND 9007199254740991),
  PRIMARY KEY (merchant_id, id, revision)
);
CREATE TABLE IF NOT EXISTS merchant_demo_transactions (
  merchant_id uuid NOT NULL REFERENCES merchant_demo_merchants(id),
  id uuid NOT NULL,
  scenario_id uuid NOT NULL,
  scenario_revision integer NOT NULL,
  source text NOT NULL CHECK (source IN ('checkout', 'simulation')),
  product text NOT NULL,
  amount_minor bigint CHECK (amount_minor BETWEEN 0 AND 9007199254740991),
  simulate_timeout boolean NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('LOW_RISK', 'REVIEW', 'UNRESOLVED')),
  reason text NOT NULL,
  rule_hits text[] NOT NULL,
  jev_status text NOT NULL CHECK (jev_status IN ('MOCKED', 'TIMEOUT')),
  jev_version text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  disposition text CHECK (disposition IN ('cleared', 'rejected')),
  decided_by text,
  decided_at timestamptz,
  PRIMARY KEY (merchant_id, id),
  FOREIGN KEY (merchant_id, scenario_id, scenario_revision) REFERENCES merchant_demo_scenarios(merchant_id, id, revision),
  CHECK ((disposition IS NULL AND decided_by IS NULL AND decided_at IS NULL) OR
         (disposition IS NOT NULL AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND outcome <> 'LOW_RISK'))
);
CREATE INDEX IF NOT EXISTS merchant_demo_transactions_created_idx
  ON merchant_demo_transactions(merchant_id, created_at DESC, id DESC);
ALTER TABLE merchant_demo_transactions ADD COLUMN IF NOT EXISTS jev_rule_version text NOT NULL DEFAULT '';
`

type mockMerchantQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

const mockMerchantListQuery = `
SELECT m.id::text, m.name, m.setup, s.id::text, s.name, s.threshold_minor, s.revision
FROM merchant_demo_merchants m
LEFT JOIN LATERAL (
	SELECT DISTINCT ON (id) id, name, threshold_minor, revision
	FROM merchant_demo_scenarios
	WHERE merchant_id=m.id
	ORDER BY id, revision DESC
) s ON true
WHERE m.deleted_at IS NULL AND (NULLIF($1::text, '') IS NULL OR m.id=NULLIF($1::text, '')::uuid)
ORDER BY lower(m.name), m.id, lower(s.name), s.id`

func scanMockMerchants(ctx context.Context, db mockMerchantQueryer, merchantID string) ([]mockMerchant, error) {
	rows, err := db.Query(ctx, mockMerchantListQuery, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merchants := []mockMerchant{}
	for rows.Next() {
		var id, name string
		var setup *merchantSetup
		var scenarioID, scenarioName *string
		var threshold *int64
		var revision *int
		if err := rows.Scan(&id, &name, &setup, &scenarioID, &scenarioName, &threshold, &revision); err != nil {
			return nil, err
		}
		if len(merchants) == 0 || merchants[len(merchants)-1].ID != id {
			merchants = append(merchants, mockMerchant{ID: id, Name: name, Setup: setup, Scenarios: []mockScenario{}})
		}
		if scenarioID != nil {
			m := &merchants[len(merchants)-1]
			m.Scenarios = append(m.Scenarios, mockScenario{
				ID: *scenarioID, Name: *scenarioName, ThresholdMinor: *threshold,
				ScenarioVersion: *revision, RuleVersion: mockRuleVersion(*revision),
			})
		}
	}
	return merchants, rows.Err()
}

func (r repository) listMockMerchants(ctx context.Context, merchantID string) ([]mockMerchant, error) {
	return scanMockMerchants(ctx, r.db, merchantID)
}

func (r repository) createMockMerchant(ctx context.Context, input mockMerchantInput) (mockMerchant, string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return mockMerchant{}, "", err
	}
	token := "merchant-demo-" + hex.EncodeToString(secret[:])
	hash := sha256.Sum256([]byte(token))
	merchant := mockMerchant{Name: input.Name, Setup: input.Setup, Scenarios: []mockScenario{}}
	err := r.db.QueryRow(ctx, `
		INSERT INTO merchant_demo_merchants(name, token_hash, setup)
		VALUES ($1, $2, NULLIF($3::jsonb, 'null'::jsonb))
		RETURNING id::text`, input.Name, hash[:], input.Setup).Scan(&merchant.ID)
	if err != nil {
		return mockMerchant{}, "", err
	}
	return merchant, token, nil
}

func (r repository) updateMockMerchant(ctx context.Context, merchantID string, input mockMerchantInput) (mockMerchant, error) {
	var merchant mockMerchant
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			UPDATE merchant_demo_merchants SET name=$2, setup=COALESCE(NULLIF($3::jsonb, 'null'::jsonb), setup)
			WHERE id=$1::text::uuid AND deleted_at IS NULL`, merchantID, input.Name, input.Setup)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		rows, err := scanMockMerchants(ctx, tx, merchantID)
		if err != nil {
			return err
		}
		merchant = rows[0]
		return nil
	})
	return merchant, err
}

func (r repository) deleteMockMerchant(ctx context.Context, merchantID string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE merchant_demo_merchants SET deleted_at=COALESCE(deleted_at, clock_timestamp())
		WHERE id=$1::text::uuid`, merchantID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func lockMockMerchant(ctx context.Context, tx pgx.Tx, merchantID string) error {
	var id string
	return tx.QueryRow(ctx, `
		SELECT id::text FROM merchant_demo_merchants WHERE id=$1::text::uuid AND deleted_at IS NULL FOR UPDATE
	`, merchantID).Scan(&id)
}

func mockRuleVersion(revision int) string {
	return fmt.Sprintf("rules-v%d", revision)
}

func getMockScenario(ctx context.Context, db mockMerchantQueryer, merchantID, scenarioID string) (mockScenario, error) {
	result := mockScenario{ID: scenarioID}
	err := db.QueryRow(ctx, `
		SELECT name, threshold_minor, revision
		FROM merchant_demo_scenarios WHERE merchant_id=$1::text::uuid AND id=$2::text::uuid
		ORDER BY revision DESC LIMIT 1`, merchantID, scenarioID).Scan(&result.Name, &result.ThresholdMinor, &result.ScenarioVersion)
	if err == nil {
		result.RuleVersion = mockRuleVersion(result.ScenarioVersion)
	}
	return result, err
}

func (r repository) saveMockScenario(ctx context.Context, merchantID, scenarioID, name string, threshold int64) (mockScenario, error) {
	result := mockScenario{ID: scenarioID, Name: name, ThresholdMinor: threshold, ScenarioVersion: 1}
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockMockMerchant(ctx, tx, merchantID); err != nil {
			return err
		}
		if scenarioID != "" {
			current, err := getMockScenario(ctx, tx, merchantID, scenarioID)
			if err != nil {
				return err
			}
			if current.Name == name && current.ThresholdMinor == threshold {
				result = current
				return nil
			}
			result.ScenarioVersion = current.ScenarioVersion + 1
		}
		var duplicate bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM (
					SELECT DISTINCT ON (id) id, name FROM merchant_demo_scenarios
					WHERE merchant_id=$1::text::uuid ORDER BY id, revision DESC
				) active WHERE active.id IS DISTINCT FROM NULLIF($2::text, '')::uuid AND lower(active.name)=lower($3)
			)`, merchantID, scenarioID, name).Scan(&duplicate)
		if err != nil {
			return err
		}
		if duplicate {
			return errDemoNameInUse
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO merchant_demo_scenarios (merchant_id, id, revision, name, threshold_minor)
			VALUES ($1::text::uuid, COALESCE(NULLIF($2::text,'')::uuid, gen_random_uuid()), $3, $4, $5)
			RETURNING id::text`, merchantID, scenarioID, result.ScenarioVersion, name, threshold).Scan(&result.ID)
		if err == nil {
			result.RuleVersion = mockRuleVersion(result.ScenarioVersion)
		}
		return err
	})
	return result, err
}

const mockTransactionReadQuery = `
SELECT t.id::text, t.merchant_id::text, t.scenario_id::text, s.name, t.scenario_revision,
	s.threshold_minor, t.source, t.product, t.amount_minor, t.outcome, t.reason, t.rule_hits, t.jev_status,
	t.jev_version, t.jev_rule_version, t.created_at, t.disposition, t.decided_by, t.decided_at
FROM merchant_demo_transactions t
JOIN merchant_demo_merchants m ON m.id=t.merchant_id AND m.deleted_at IS NULL
JOIN merchant_demo_scenarios s ON s.merchant_id=t.merchant_id AND s.id=t.scenario_id AND s.revision=t.scenario_revision
WHERE t.merchant_id=$1::text::uuid`

func scanMockTransaction(row pgx.Row) (mockTransaction, error) {
	var result mockTransaction
	err := row.Scan(&result.ID, &result.MerchantID, &result.ScenarioID, &result.ScenarioName, &result.ScenarioVersion,
		&result.ThresholdMinor, &result.Source, &result.Product, &result.AmountMinor, &result.Outcome,
		&result.Reason, &result.RuleHits, &result.JevStatus, &result.JevVersion, &result.JevRuleVersion, &result.CreatedAt,
		&result.Disposition, &result.DecidedBy, &result.DecidedAt)
	if err != nil {
		return result, err
	}
	result.RuleVersion = mockRuleVersion(result.ScenarioVersion)
	switch {
	case result.Disposition != nil:
		result.ReviewState = "RESOLVED"
	case result.Outcome == "LOW_RISK":
		result.ReviewState = "NONE"
	default:
		result.ReviewState = "NEEDS_DECISION"
	}
	return result, nil
}

func readMockTransaction(ctx context.Context, db mockMerchantQueryer, merchantID, transactionID string) (mockTransaction, error) {
	return scanMockTransaction(db.QueryRow(ctx, mockTransactionReadQuery+" AND t.id=$2::text::uuid", merchantID, transactionID))
}

func (r repository) listMockTransactions(ctx context.Context, merchantID string) ([]mockTransaction, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM merchant_demo_merchants WHERE id=$1::text::uuid AND deleted_at IS NULL)
	`, merchantID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, pgx.ErrNoRows
	}
	rows, err := r.db.Query(ctx, mockTransactionReadQuery+" ORDER BY t.created_at DESC,t.id DESC", merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []mockTransaction{}
	for rows.Next() {
		transaction, err := scanMockTransaction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, transaction)
	}
	return result, rows.Err()
}

func evaluateMockJev(input mockTransactionInput, scenario mockScenario) mockTransaction {
	result := mockTransaction{
		ID: input.ID, ScenarioID: scenario.ID, ScenarioName: scenario.Name,
		ScenarioVersion: scenario.ScenarioVersion, RuleVersion: scenario.RuleVersion,
		ThresholdMinor: scenario.ThresholdMinor, Source: input.Source, Product: input.Product,
		AmountMinor: input.AmountMinor, RuleHits: []string{}, JevStatus: "MOCKED", JevVersion: "mock-jev-v1",
	}
	switch {
	case input.SimulateTimeout:
		result.JevStatus, result.Outcome, result.ReviewState = "TIMEOUT", "UNRESOLVED", "NEEDS_DECISION"
		result.Reason = "Mock Jev timed out before returning a decision."
	case input.AmountMinor == nil:
		result.Outcome, result.ReviewState = "UNRESOLVED", "NEEDS_DECISION"
		result.Reason = "Booking amount was unavailable, so the review rule could not be evaluated."
	case *input.AmountMinor >= scenario.ThresholdMinor:
		result.Outcome, result.ReviewState = "REVIEW", "NEEDS_DECISION"
		result.Reason = fmt.Sprintf("Booking amount %d meets the review threshold of %d minor units.", *input.AmountMinor, scenario.ThresholdMinor)
		result.RuleHits = []string{"AMOUNT_REVIEW_THRESHOLD"}
	default:
		result.Outcome, result.ReviewState = "LOW_RISK", "NONE"
		result.Reason = fmt.Sprintf("Booking amount %d is below the review threshold of %d minor units.", *input.AmountMinor, scenario.ThresholdMinor)
	}
	return result
}

func (r repository) saveMockTransaction(ctx context.Context, merchantID string, input mockTransactionInput) (mockTransaction, bool, error) {
	var result mockTransaction
	var created bool
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		// Serialize traffic per merchant so the scenario revision and replay lookup
		// cannot race a scenario edit or a concurrent submission of the same ID.
		if err := lockMockMerchant(ctx, tx, merchantID); err != nil {
			return err
		}
		current, err := readMockTransaction(ctx, tx, merchantID, input.ID)
		if err == nil {
			if current.ScenarioID != input.ScenarioID || current.Source != input.Source || current.Product != input.Product ||
				!sameBookingAmount(current.AmountMinor, input.AmountMinor) || (current.JevStatus == "TIMEOUT") != input.SimulateTimeout {
				return errDemoReplayConflict
			}
			result = current
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		scenario, err := getMockScenario(ctx, tx, merchantID, input.ScenarioID)
		if err != nil {
			return err
		}
		jevRules, err := readJevRules(ctx, tx, merchantID)
		if err != nil {
			return err
		}
		result = evaluateMockJev(input, scenario)
		result.applyMerchantJevRules(jevRules)
		result.MerchantID = merchantID
		err = tx.QueryRow(ctx, `
			INSERT INTO merchant_demo_transactions
				(merchant_id, id, scenario_id, scenario_revision, source, product, amount_minor, simulate_timeout,
				 outcome, reason, rule_hits, jev_status, jev_version, jev_rule_version)
			VALUES ($1::text::uuid, $2::text::uuid, $3::text::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			RETURNING created_at`,
			merchantID, input.ID, input.ScenarioID, result.ScenarioVersion, input.Source, input.Product, input.AmountMinor,
			input.SimulateTimeout, result.Outcome, result.Reason, result.RuleHits, result.JevStatus, result.JevVersion,
			result.JevRuleVersion).Scan(&result.CreatedAt)
		created = err == nil
		return err
	})
	return result, created, err
}

func (r repository) decideMockTransaction(ctx context.Context, merchantID, transactionID, decision, decidedBy string) (mockTransaction, error) {
	var result mockTransaction
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockMockMerchant(ctx, tx, merchantID); err != nil {
			return err
		}
		current, err := readMockTransaction(ctx, tx, merchantID, transactionID)
		if err != nil {
			return err
		}
		if current.Disposition != nil && *current.Disposition == decision {
			result = current
			return nil
		}
		if current.ReviewState != "NEEDS_DECISION" {
			return errDemoDecisionConflict
		}
		_, err = tx.Exec(ctx, `
			UPDATE merchant_demo_transactions SET disposition=$3, decided_by=$4, decided_at=clock_timestamp()
			WHERE merchant_id=$1::text::uuid AND id=$2::text::uuid`, merchantID, transactionID, decision, decidedBy)
		if err != nil {
			return err
		}
		result, err = readMockTransaction(ctx, tx, merchantID, transactionID)
		return err
	})
	return result, err
}
