package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const schema = `
CREATE TABLE IF NOT EXISTS simulation_runs (
  merchant_id text NOT NULL,
  run_id text NOT NULL,
  config jsonb NOT NULL,
  result jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (merchant_id, run_id)
);
CREATE SCHEMA IF NOT EXISTS simulation_qa;
CREATE TABLE IF NOT EXISTS simulation_qa.ground_truth (
  merchant_id text NOT NULL,
  run_id text NOT NULL,
  event_id text NOT NULL,
  is_fraud boolean NOT NULL,
  hidden_ring_id text,
  PRIMARY KEY (merchant_id, run_id, event_id),
  FOREIGN KEY (merchant_id, run_id) REFERENCES simulation_runs (merchant_id, run_id)
);
`

type redisVelocity struct {
	client *redis.Client
	prefix string
}

var trackVelocity = redis.NewScript(`
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
redis.call('EXPIRE', KEYS[1], 600)
return redis.call('ZCOUNT', KEYS[1], ARGV[3], ARGV[1])
`)

func (v redisVelocity) count(ctx context.Context, e event) ([]feature, error) {
	pipe := v.client.Pipeline()
	defer pipe.Discard()
	commands := make([]*redis.Cmd, 0, len(e.Entities))
	for _, entity := range e.Entities {
		key := v.prefix + ":" + e.Stage + ":" + entity.Kind + ":" + entity.Key
		commands = append(commands, trackVelocity.Eval(ctx, pipe, []string{key}, e.At, e.OrderID, e.At-59))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("velocity: %w", err)
	}
	features := make([]feature, 0, len(commands))
	for i, command := range commands {
		count, err := command.Int64()
		if err != nil {
			return nil, fmt.Errorf("velocity result: %w", err)
		}
		features = append(features, feature{e.Entities[i].Kind, count})
	}
	return features, nil
}

func attemptID() (string, error) {
	var random [16]byte
	_, err := rand.Read(random[:])
	return hex.EncodeToString(random[:]), err
}

type run struct {
	ID               string     `json:"id"`
	Config           config     `json:"config"`
	GeneratorVersion string     `json:"generatorVersion"`
	RuleVersion      string     `json:"ruleVersion"`
	JevVersion       string     `json:"jevVersion"`
	Duplicates       int        `json:"duplicates"`
	Decisions        []decision `json:"decisions"`
}

type repository struct {
	db *pgxpool.Pool
}

func (r repository) get(ctx context.Context, tenant, id string) (run, error) {
	var result run
	var raw []byte
	if err := r.db.QueryRow(ctx, `SELECT result FROM simulation_runs WHERE merchant_id=$1 AND run_id=$2`, tenant, id).Scan(&raw); err != nil {
		return result, err
	}
	return result, json.Unmarshal(raw, &result)
}

func (r repository) save(ctx context.Context, tenant string, result run, truth []label) (run, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return run{}, err
	}
	defer tx.Rollback(ctx)
	configJSON, err := json.Marshal(result.Config)
	if err != nil {
		return run{}, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return run{}, err
	}
	inserted, err := tx.Exec(ctx, `INSERT INTO simulation_runs (merchant_id,run_id,config,result)
		VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, result.ID, configJSON, resultJSON)
	if err != nil {
		return run{}, err
	}
	if inserted.RowsAffected() == 0 {
		if err := tx.Rollback(ctx); err != nil {
			return run{}, err
		}
		return r.get(ctx, tenant, result.ID)
	}
	batch := &pgx.Batch{}
	for _, l := range truth {
		batch.Queue(`INSERT INTO simulation_qa.ground_truth
			(merchant_id,run_id,event_id,is_fraud,hidden_ring_id) VALUES ($1,$2,$3,$4,NULLIF($5,''))`,
			tenant, result.ID, l.EventID, l.Fraud, l.RingID)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return run{}, err
	}
	return result, tx.Commit(ctx)
}

type report struct {
	TP        int      `json:"tp"`
	FP        int      `json:"fp"`
	TN        int      `json:"tn"`
	FN        int      `json:"fn"`
	Unscored  int      `json:"unscored"`
	Samples   int      `json:"samples"`
	Precision *float64 `json:"precision"`
	Recall    *float64 `json:"recall"`
	FPR       *float64 `json:"fpr"`
}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	value := float64(n) / float64(d)
	return &value
}

func (r repository) qaReport(ctx context.Context, tenant string, saved run) (report, error) {
	rows, err := r.db.Query(ctx, `SELECT event_id, is_fraud FROM simulation_qa.ground_truth
		WHERE merchant_id=$1 AND run_id=$2`, tenant, saved.ID)
	if err != nil {
		return report{}, err
	}
	defer rows.Close()
	decisions := make(map[string]decision, len(saved.Decisions))
	for _, d := range saved.Decisions {
		decisions[d.Event.ID] = d
	}
	var result report
	for rows.Next() {
		var id string
		var fraud bool
		if err := rows.Scan(&id, &fraud); err != nil {
			return report{}, err
		}
		d, found := decisions[id]
		if !found {
			return report{}, errors.New("QA event has no corresponding decision")
		}
		result.Samples++
		positive := d.Platform == "REVIEW" || d.Platform == "CHALLENGE" || d.Platform == "BLOCK"
		switch {
		case d.JevStatus == "TIMEOUT":
			result.Unscored++
		case positive && fraud:
			result.TP++
		case positive:
			result.FP++
		case fraud:
			result.FN++
		default:
			result.TN++
		}
	}
	if err := rows.Err(); err != nil {
		return report{}, err
	}
	result.Precision = ratio(result.TP, result.TP+result.FP)
	result.Recall = ratio(result.TP, result.TP+result.FN)
	result.FPR = ratio(result.FP, result.FP+result.TN)
	return result, nil
}

func initialize(ctx context.Context, db *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(746690167)"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
