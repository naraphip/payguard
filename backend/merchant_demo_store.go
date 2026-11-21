package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

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
  ADD COLUMN IF NOT EXISTS mailbox_ref_key uuid NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE TABLE IF NOT EXISTS merchant_demo_access_tokens (
  token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  merchant_id uuid NOT NULL REFERENCES merchant_demo_merchants(id),
  expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS merchant_demo_access_tokens_expiry_idx
  ON merchant_demo_access_tokens(expires_at);
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
  scenario_id uuid,
  scenario_revision integer,
  source text NOT NULL CHECK (source IN ('checkout', 'simulation')),
  product text NOT NULL,
  amount_minor bigint CHECK (amount_minor BETWEEN 0 AND 9007199254740991),
  simulate_timeout boolean NOT NULL,
  outcome text NOT NULL CONSTRAINT merchant_demo_outcome_v2_check CHECK (outcome IN ('LOW_RISK', 'HIGH_RISK', 'REVIEW', 'UNRESOLVED')),
  reason text NOT NULL,
  rule_hits text[] NOT NULL,
  jev_status text NOT NULL CONSTRAINT merchant_demo_jev_status_v2_check
    CHECK (jev_status IN ('MOCKED', 'TIMEOUT', 'NOT_RUN', 'UNAVAILABLE', 'LIVE')),
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
ALTER TABLE merchant_demo_transactions
  ALTER COLUMN scenario_id DROP NOT NULL,
  ALTER COLUMN scenario_revision DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS platform_rule_version text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS account_age_days bigint CHECK (account_age_days BETWEEN 0 AND 36500),
  ADD COLUMN IF NOT EXISTS jev_reason text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS customer_mailbox_ref text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS customer_id_ref text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS jev_rule_version text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS jev_answer text NOT NULL DEFAULT '' CHECK (jev_answer IN ('', 'LOW_RISK', 'REVIEW')),
  ADD COLUMN IF NOT EXISTS jev_decision_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS jev_confidence double precision CHECK (jev_confidence BETWEEN 0 AND 1),
  DROP CONSTRAINT IF EXISTS merchant_demo_transactions_jev_status_check,
  DROP CONSTRAINT IF EXISTS merchant_demo_transactions_outcome_check;
CREATE INDEX IF NOT EXISTS merchant_demo_transactions_customer_history_idx
  ON merchant_demo_transactions(merchant_id, customer_id_ref, created_at DESC, id DESC)
  WHERE customer_id_ref <> '';
DO $migration$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'merchant_demo_transactions'::regclass AND conname = 'merchant_demo_jev_status_v2_check'
  ) THEN
    ALTER TABLE merchant_demo_transactions ADD CONSTRAINT merchant_demo_jev_status_v2_check
      CHECK (jev_status IN ('MOCKED', 'TIMEOUT', 'NOT_RUN', 'UNAVAILABLE', 'LIVE'));
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'merchant_demo_transactions'::regclass AND conname = 'merchant_demo_scenario_pair_check'
  ) THEN
    ALTER TABLE merchant_demo_transactions ADD CONSTRAINT merchant_demo_scenario_pair_check
      CHECK ((scenario_id IS NULL) = (scenario_revision IS NULL));
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'merchant_demo_transactions'::regclass AND conname = 'merchant_demo_outcome_v2_check'
  ) THEN
    ALTER TABLE merchant_demo_transactions ADD CONSTRAINT merchant_demo_outcome_v2_check
      CHECK (outcome IN ('LOW_RISK', 'HIGH_RISK', 'REVIEW', 'UNRESOLVED'));
  END IF;
END;
$migration$;
`

type mockMerchantQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

const mockMerchantListQuery = `
SELECT m.id::text, m.name, m.description, m.created_at, m.setup, m.policy_integration, s.id::text, s.name, s.threshold_minor, s.revision
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
		var id, name, description string
		var createdAt *time.Time
		var setup *merchantSetup
		var integration *merchantPolicyIntegration
		var scenarioID, scenarioName *string
		var threshold *int64
		var revision *int
		if err := rows.Scan(&id, &name, &description, &createdAt, &setup, &integration, &scenarioID, &scenarioName, &threshold, &revision); err != nil {
			return nil, err
		}
		if len(merchants) == 0 || merchants[len(merchants)-1].ID != id {
			merchants = append(merchants, mockMerchant{ID: id, Name: name, Description: description, CreatedAt: createdAt, Setup: setup, PolicyIntegration: integration, Scenarios: []mockScenario{}})
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

func newMerchantToken(prefix string) (string, [sha256.Size]byte, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", [sha256.Size]byte{}, err
	}
	token := prefix + hex.EncodeToString(secret[:])
	return token, sha256.Sum256([]byte(token)), nil
}

func (r repository) createMockMerchant(ctx context.Context, input mockMerchantInput) (mockMerchant, string, error) {
	token, hash, err := newMerchantToken("merchant-demo-")
	if err != nil {
		return mockMerchant{}, "", err
	}
	merchant := mockMerchant{Name: input.Name, Setup: input.Setup, PolicyIntegration: input.PolicyIntegration, Scenarios: []mockScenario{}}
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO merchant_demo_merchants(name, token_hash, setup, policy_integration, description, creation_request_hash)
			VALUES ($1, $2, NULLIF($3::jsonb, 'null'::jsonb), NULLIF($4::jsonb, 'null'::jsonb), COALESCE($5, ''), $6)
 ON CONFLICT (creation_request_hash) WHERE creation_request_hash IS NOT NULL DO NOTHING
			RETURNING id::text, description, created_at`, input.Name, hash[:], input.Setup, input.PolicyIntegration, input.Description, input.CreationRequestHash).
			Scan(&merchant.ID, &merchant.Description, &merchant.CreatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errMerchantCreateReplay
			}
			return err
		}
		if err := auditMerchant(ctx, tx, merchant.ID, "MERCHANT_CREATED"); err != nil {
			return err
		}
		return auditMerchant(ctx, tx, merchant.ID, "MERCHANT_CREDENTIAL_ISSUED")
	})
	if err != nil {
		return mockMerchant{}, "", err
	}
	return merchant, token, nil
}

func (r repository) issueMerchantAccessToken(ctx context.Context, merchantID string) (merchantAccessToken, error) {
	token, hash, err := newMerchantToken(merchantAccessTokenPrefix)
	if err != nil {
		return merchantAccessToken{}, err
	}
	result := merchantAccessToken{Token: token}
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockMockMerchant(ctx, tx, merchantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM merchant_demo_access_tokens WHERE expires_at <= statement_timestamp()`); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO merchant_demo_access_tokens (token_hash, merchant_id, expires_at)
			VALUES ($1, $2::text::uuid, clock_timestamp() + interval '1 hour')
			RETURNING expires_at`, hash[:], merchantID).Scan(&result.ExpiresAt); err != nil {
			return err
		}
		return auditMerchant(ctx, tx, merchantID, "MERCHANT_TEMPORARY_CREDENTIAL_ISSUED")
	})
	if err != nil {
		return merchantAccessToken{}, err
	}
	return result, nil
}

func (r repository) updateMockMerchant(ctx context.Context, merchantID string, input mockMerchantInput) (mockMerchant, error) {
	var merchant mockMerchant
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := checkCustomFieldChanges(ctx, tx, merchantID, input.PolicyIntegration); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `
			UPDATE merchant_demo_merchants SET name=$2, setup=COALESCE(NULLIF($3::jsonb, 'null'::jsonb), setup),
 policy_integration=COALESCE(NULLIF($4::jsonb, 'null'::jsonb), policy_integration),
 description=COALESCE($5,description)
			WHERE id=$1::text::uuid AND deleted_at IS NULL`, merchantID, input.Name, input.Setup, input.PolicyIntegration, input.Description)
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
		return auditMerchant(ctx, tx, merchantID, "MERCHANT_UPDATED")
	})
	return merchant, err
}

func (r repository) deleteMockMerchant(ctx context.Context, merchantID string) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var deletedAt *time.Time
		if err := tx.QueryRow(ctx, `SELECT deleted_at FROM merchant_demo_merchants WHERE id=$1::text::uuid FOR UPDATE`,
			merchantID).Scan(&deletedAt); err != nil {
			return err
		}
		if deletedAt != nil {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE merchant_demo_merchants SET deleted_at=clock_timestamp() WHERE id=$1::text::uuid`, merchantID); err != nil {
			return err
		}
		if err := auditMerchant(ctx, tx, merchantID, "MERCHANT_DEACTIVATED"); err != nil {
			return err
		}
		return auditMerchant(ctx, tx, merchantID, "MERCHANT_CREDENTIAL_REVOKED")
	})
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
SELECT t.id::text, t.merchant_id::text, COALESCE(t.scenario_id::text, ''), COALESCE(s.name, ''), COALESCE(t.scenario_revision, 0),
	s.threshold_minor, t.source, t.product, t.amount_minor, t.simulate_timeout, t.outcome, t.reason, t.rule_hits, t.jev_status,
	t.jev_version, t.jev_rule_version, t.jev_answer, t.jev_decision_id, t.jev_confidence, t.jev_configuration_version,
	t.created_at, t.disposition, t.decided_by, t.decided_at, t.customer_email_domain, t.customer_mailbox_ref, t.customer_id_ref, t.policy_decision,
	t.platform_rule_version, t.jev_reason, t.account_age_days, t.custom_signals, ` + merchantTransactionReview + `
FROM merchant_demo_transactions t
JOIN merchant_demo_merchants m ON m.id=t.merchant_id AND m.deleted_at IS NULL
LEFT JOIN merchant_demo_scenarios s ON s.merchant_id=t.merchant_id AND s.id=t.scenario_id AND s.revision=t.scenario_revision
WHERE t.merchant_id=$1::text::uuid`

func scanMockTransaction(row pgx.Row) (mockTransaction, error) {
	var result mockTransaction
	err := row.Scan(&result.ID, &result.MerchantID, &result.ScenarioID, &result.ScenarioName, &result.ScenarioVersion,
		&result.ThresholdMinor, &result.Source, &result.Product, &result.AmountMinor, &result.simulateTimeout, &result.Outcome,
		&result.Reason, &result.RuleHits, &result.JevStatus, &result.JevVersion, &result.JevRuleVersion, &result.JevAnswer, &result.JevDecisionID, &result.JevConfidence, &result.JevConfigurationVersion, &result.CreatedAt,
		&result.Disposition, &result.DecidedBy, &result.DecidedAt, &result.CustomerEmailDomain, &result.customerMailboxRef, &result.customerIDRef, &result.PolicyDecision,
		&result.RuleVersion, &result.JevReason, &result.AccountAgeDays, &result.CustomSignals, &result.ReviewState)
	if err != nil {
		return result, err
	}
	if result.ScenarioID != "" {
		result.RuleVersion = mockRuleVersion(result.ScenarioVersion)
	}
	if len(result.CustomSignals) == 0 {
		result.CustomSignals = nil
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
		ThresholdMinor: &scenario.ThresholdMinor, Source: input.Source, Product: input.Product,
		AmountMinor: input.AmountMinor, RuleHits: []string{}, JevStatus: "MOCKED", JevVersion: "mock-jev-v1", simulateTimeout: input.SimulateTimeout,
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

func (result mockTransaction) matchesRequest(input mockTransactionInput) bool {
	return result.ScenarioID == input.ScenarioID && result.Source == input.Source && result.Product == input.Product &&
		result.CustomerEmailDomain == input.CustomerEmailDomain && sameBookingAmount(result.AmountMinor, input.AmountMinor) &&
		result.simulateTimeout == input.SimulateTimeout && sameBookingAmount(result.AccountAgeDays, input.AccountAgeDays) &&
		equalCustomSignals(result.CustomSignals, input.CustomSignals)
}
func (r repository) saveMockTransaction(ctx context.Context, merchantID string, input mockTransactionInput, options ...merchantJev) (mockTransaction, bool, error) {
	var jev merchantJev
	if len(options) > 0 {
		jev = options[0]
	}
	var result mockTransaction
	var created bool
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		// Serialize traffic per merchant so the scenario revision and replay lookup
		// cannot race a scenario edit or a concurrent submission of the same ID.
		if err := lockMockMerchant(ctx, tx, merchantID); err != nil {
			return err
		}
		mailboxRef, err := merchantMailboxRef(ctx, tx, merchantID, input.CustomerMailboxDigest)
		if err != nil {
			return err
		}
		input.CustomerMailboxDigest = ""
		customerRef, err := merchantCustomerIDRef(ctx, tx, merchantID, input.CustomerID)
		if err != nil {
			return err
		}
		input.CustomerID = ""
		current, err := readMockTransaction(ctx, tx, merchantID, input.ID)
		if err == nil {
			if current.customerMailboxRef != mailboxRef || current.customerIDRef != customerRef || !current.matchesRequest(input) {
				return errDemoReplayConflict
			}
			result = current
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if input.ScenarioID == "" {
			profile, _, err := readMerchantPolicyContext(ctx, tx, merchantID)
			if err != nil {
				return err
			}
			if !slices.Contains(profile.Products, input.Product) {
				return merchantPolicyValidationError{errors.New("Choose a product available for this merchant.")}
			}
		}
		if err := validateTransactionCustomSignals(ctx, tx, merchantID, input.CustomSignals); err != nil {
			return err
		}
		var scenario mockScenario
		if input.ScenarioID != "" {
			scenario, err = getMockScenario(ctx, tx, merchantID, input.ScenarioID)
			if err != nil {
				return err
			}
		}
		jevRules, err := readJevRules(ctx, tx, merchantID)
		if err != nil {
			return err
		}
		savedConfig, err := readJevAIConfiguration(ctx, tx)
		if err != nil {
			return err
		}
		if savedConfig.Version > 0 {
			jev = jev.withConfiguration(savedConfig.Config)
		}
		activePolicies, err := readMerchantPolicies(ctx, tx, merchantID, true)
		if err != nil {
			return err
		}
		evidence := merchantPolicyEvidence{
			product: input.Product, stage: merchantPolicyStage(input.Product), domain: input.CustomerEmailDomain,
			mailboxRef: mailboxRef, amount: input.AmountMinor, custom: input.CustomSignals,
		}
		// Reserve time for writing the decision after Jev, including after waiting for the merchant lock.
		decisionDeadline := time.Now().Add(10 * time.Second)
		if deadline, ok := ctx.Deadline(); ok && deadline.Add(-time.Second).Before(decisionDeadline) {
			decisionDeadline = deadline.Add(-time.Second)
		}
		decisionCtx, cancelDecision := context.WithDeadline(ctx, decisionDeadline)
		defer cancelDecision()
		if input.ScenarioID == "" {
			changedEmail, err := customerChangedDisposableEmail(ctx, tx, merchantID, customerRef, input.CustomerEmailDomain, mailboxRef)
			if err != nil {
				return err
			}
			platformRules, err := readActivePlatformRules(ctx, tx)
			if err != nil {
				return err
			}
			result = evaluateMerchantRuleDecision(decisionCtx, input, platformRules, activePolicies, evidence, jevRules, jev)
			result.PolicyDecision.RiskScore = calculateMerchantRiskScore(input, result, changedEmail)
		} else {
			merchant := evaluateMerchantPoliciesWithJev(decisionCtx, activePolicies, evidence, jev, input.SimulateTimeout, false)
			result = evaluateMerchantJev(decisionCtx, input, scenario, jevRules, jev)
			result.applyLegacyMerchantPolicyDecision(merchant)
			result.PolicyDecision.requireReviewForUnresolvedMerchantPolicy()
			if result.PolicyDecision.Final.Source == "MERCHANT_POLICY" &&
				(slices.Contains(merchant.ReasonCodes, merchantJevUnresolved) || slices.ContainsFunc(activePolicies, func(rule merchantPolicyRule) bool {
					return slices.Contains(merchant.MatchedRuleIDs, rule.ID) && hasSemanticMerchantCondition(rule)
				})) {
				result.applyMerchantRuleOutcome()
			}
		}
		result.JevConfigurationVersion = savedConfig.Version
		result.MerchantID = merchantID
		result.CustomerEmailDomain = input.CustomerEmailDomain
		result.customerMailboxRef = mailboxRef
		result.customerIDRef = customerRef
		result.AccountAgeDays = input.AccountAgeDays
		if len(input.CustomSignals) > 0 {
			result.CustomSignals = input.CustomSignals
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO merchant_demo_transactions
				(merchant_id, id, scenario_id, scenario_revision, source, product, amount_minor, simulate_timeout,
				 outcome, reason, rule_hits, jev_status, jev_version, jev_rule_version, customer_email_domain, customer_mailbox_ref,
				 policy_decision, jev_answer, jev_decision_id, jev_confidence, jev_configuration_version,
			 platform_rule_version, jev_reason, account_age_days, custom_signals, customer_id_ref)
			VALUES ($1::text::uuid, $2::text::uuid, NULLIF($3::text,'')::uuid, NULLIF($4::int,0), $5, $6, $7, $8, $9, $10, $11, $12, $13,
			 $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
			RETURNING created_at`,
			merchantID, input.ID, input.ScenarioID, result.ScenarioVersion, input.Source, input.Product, input.AmountMinor,
			input.SimulateTimeout, result.Outcome, result.Reason, result.RuleHits, result.JevStatus, result.JevVersion,
			result.JevRuleVersion, result.CustomerEmailDomain, result.customerMailboxRef, result.PolicyDecision,
			result.JevAnswer, result.JevDecisionID, result.JevConfidence, result.JevConfigurationVersion,
			result.RuleVersion, result.JevReason, result.AccountAgeDays, result.CustomSignals, result.customerIDRef).Scan(&result.CreatedAt)
		created = err == nil
		return err
	})
	return result, created, err
}

func (result *mockTransaction) applyLegacyMerchantPolicyDecision(merchant policyDecisionPart) {
	platform := policyDecisionPart{Action: result.Outcome, ReasonCodes: result.RuleHits}
	if platform.Action == "LOW_RISK" {
		platform.Action = "ALLOW"
	}
	policy := resolvePolicyDecision(platform, merchant)
	policy.ScoringProfileVersion = merchantScenarioProfileVersion
	result.PolicyDecision = &policy
	if policy.Final.Action == "REVIEW" && result.Outcome == "LOW_RISK" {
		result.Outcome, result.ReviewState = "REVIEW", "NEEDS_DECISION"
		result.Reason = "A merchant business policy requires manual review."
	}
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
