package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const rulesSchema = `
CREATE TABLE IF NOT EXISTS system_rule_drafts (
  tenant_id text NOT NULL,
  rule_id text NOT NULL,
  input jsonb NOT NULL,
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, rule_id)
);
CREATE INDEX IF NOT EXISTS simulation_runs_recent ON simulation_runs (merchant_id, created_at DESC, run_id DESC);
`

var errStaleRule = errors.New("rule version is stale")

func readDraft(row pgx.Row) (systemRule, error) {
	rule := systemRule{Status: "DRAFT"}
	var updatedAt time.Time
	if err := row.Scan(&rule.ID, &rule.ruleInput, &rule.Version, &updatedAt); err != nil {
		return systemRule{}, err
	}
	rule.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return rule, nil
}

func (r repository) draftRules(ctx context.Context, tenant string) ([]systemRule, error) {
	rows, err := r.db.Query(ctx, `SELECT rule_id, input, version, updated_at FROM system_rule_drafts
		WHERE tenant_id=$1 ORDER BY updated_at DESC, rule_id DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]systemRule, 0)
	for rows.Next() {
		rule, err := readDraft(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r repository) createDraft(ctx context.Context, tenant string, input ruleInput) (systemRule, error) {
	id, err := attemptID()
	if err != nil {
		return systemRule{}, err
	}
	return readDraft(r.db.QueryRow(ctx, `INSERT INTO system_rule_drafts (tenant_id, rule_id, input)
		VALUES ($1,$2,$3) RETURNING rule_id, input, version, updated_at`, tenant, id, input))
}

func (r repository) updateDraft(ctx context.Context, tenant, id string, version int64, input ruleInput) (systemRule, error) {
	rule, err := readDraft(r.db.QueryRow(ctx, `UPDATE system_rule_drafts
		SET input=$4, version=version+1, updated_at=now()
		WHERE tenant_id=$1 AND rule_id=$2 AND version=$3
		RETURNING rule_id, input, version, updated_at`, tenant, id, version, input))
	if !errors.Is(err, pgx.ErrNoRows) {
		return rule, err
	}

	var exists bool
	err = r.db.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM system_rule_drafts WHERE tenant_id=$1 AND rule_id=$2
	)`, tenant, id).Scan(&exists)
	if err != nil {
		return systemRule{}, err
	}
	if exists {
		return systemRule{}, errStaleRule
	}
	return systemRule{}, pgx.ErrNoRows
}

type previewRun struct {
	ID        string    `json:"id"`
	Seed      uint64    `json:"seed"`
	Product   string    `json:"product"`
	Scenario  string    `json:"scenario"`
	CreatedAt time.Time `json:"createdAt"`
}

func (r repository) previewRuns(ctx context.Context, tenant string) ([]previewRun, error) {
	rows, err := r.db.Query(ctx, `SELECT run_id, config, created_at FROM simulation_runs
		WHERE merchant_id=$1 ORDER BY created_at DESC, run_id DESC LIMIT 25`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]previewRun, 0)
	for rows.Next() {
		var id string
		var c config
		var createdAt time.Time
		if err := rows.Scan(&id, &c, &createdAt); err != nil {
			return nil, err
		}
		runs = append(runs, previewRun{id, c.Seed, c.Product, c.Scenario, createdAt.UTC()})
	}
	return runs, rows.Err()
}
