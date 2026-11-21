package main

import (
	"context"
	"errors"
	"time"
)

const omiseSchema = `
CREATE TABLE IF NOT EXISTS simulation_omise_charges (
  merchant_id text NOT NULL,
  run_id text NOT NULL,
  amount bigint NOT NULL CHECK (amount > 0),
  currency text NOT NULL CHECK (currency = 'THB'),
  status text NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown', 'pending', 'failed', 'successful')),
  charge_id text NOT NULL DEFAULT '',
  failure_code text NOT NULL DEFAULT '',
  started_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  PRIMARY KEY (merchant_id, run_id),
  FOREIGN KEY (merchant_id, run_id) REFERENCES simulation_runs (merchant_id, run_id)
);
`

type omiseChargeRecord struct {
	Result     omiseChargeResult
	StartedAt  time.Time
	FinishedAt *time.Time
}

func (r repository) reserveOmiseCharge(ctx context.Context, tenant, runID string, amount int64) (bool, error) {
	result, err := r.db.Exec(ctx, `
		INSERT INTO simulation_omise_charges (merchant_id, run_id, amount, currency)
		VALUES ($1, $2, $3, $4) ON CONFLICT (merchant_id, run_id) DO NOTHING
	`, tenant, runID, amount, omiseCurrency)
	return result.RowsAffected() == 1, err
}

func (r repository) readOmiseCharge(ctx context.Context, tenant, runID string) (omiseChargeRecord, error) {
	var record omiseChargeRecord
	err := r.db.QueryRow(ctx, `
		SELECT status, charge_id, amount, currency, failure_code, started_at, finished_at
		FROM simulation_omise_charges WHERE merchant_id=$1 AND run_id=$2
	`, tenant, runID).Scan(&record.Result.Status, &record.Result.ChargeID, &record.Result.Amount,
		&record.Result.Currency, &record.Result.FailureCode, &record.StartedAt, &record.FinishedAt)
	return record, err
}

func (r repository) finishOmiseCharge(ctx context.Context, tenant, runID string, result omiseChargeResult) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE simulation_omise_charges
		SET status=$3, charge_id=$4, failure_code=$5, finished_at=now()
		WHERE merchant_id=$1 AND run_id=$2 AND finished_at IS NULL
	`, tenant, runID, result.Status, result.ChargeID, result.FailureCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("simulation charge reservation no longer pending")
	}
	return nil
}
