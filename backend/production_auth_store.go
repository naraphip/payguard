package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Production authentication migration 001. Apply after the merchant tables.
// Versioned separately from the sandbox schemas and safe to replay at startup.
const productionAuthSchema = `
CREATE TABLE IF NOT EXISTS production_auth_identities (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  issuer text NOT NULL CHECK (length(issuer) > 0),
  subject text NOT NULL CHECK (octet_length(subject) BETWEEN 1 AND 255),
  role text NOT NULL CHECK (role IN ('platform_admin', 'platform_checker', 'merchant')),
  merchant_id uuid REFERENCES merchant_demo_merchants(id),
  revision bigint NOT NULL DEFAULT 1,
  disabled_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE (issuer, subject),
  CHECK ((role = 'merchant') = (merchant_id IS NOT NULL))
);
CREATE OR REPLACE FUNCTION production_auth_revise_identity() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.issuer, NEW.subject) IS DISTINCT FROM (OLD.id, OLD.issuer, OLD.subject) THEN
    RAISE EXCEPTION 'a provisioned identity cannot change its issuer or subject';
  END IF;
  NEW.revision := OLD.revision;
  IF (NEW.role, NEW.merchant_id, NEW.disabled_at)
       IS DISTINCT FROM (OLD.role, OLD.merchant_id, OLD.disabled_at) THEN
    NEW.revision := OLD.revision + 1;
  END IF;
  RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER production_auth_identity_revision
  BEFORE UPDATE ON production_auth_identities FOR EACH ROW
  EXECUTE FUNCTION production_auth_revise_identity();
CREATE TABLE IF NOT EXISTS production_auth_oidc_flows (
  state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash) = 32),
  browser_hash bytea NOT NULL CHECK (octet_length(browser_hash) = 32),
  nonce text NOT NULL,
  verifier text NOT NULL,
  expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS production_auth_oidc_flows_expire_idx ON production_auth_oidc_flows (expires_at);
CREATE TABLE IF NOT EXISTS production_auth_sessions (
  token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  identity_id uuid NOT NULL REFERENCES production_auth_identities(id),
  identity_revision bigint NOT NULL,
  started_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS production_auth_sessions_expire_idx ON production_auth_sessions (expires_at);
CREATE INDEX IF NOT EXISTS production_auth_sessions_identity_idx ON production_auth_sessions (identity_id);
CREATE TABLE IF NOT EXISTS production_auth_audit (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id uuid,
  role text,
  event text NOT NULL,
  reason text NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS production_auth_audit_occurred_idx ON production_auth_audit (occurred_at DESC);
`

type productionAuthStore struct{ db *pgxpool.Pool }

const productionActiveIdentity = `i.disabled_at IS NULL AND (i.merchant_id IS NULL OR EXISTS (
  SELECT 1 FROM merchant_demo_merchants AS m WHERE m.id = i.merchant_id AND m.deleted_at IS NULL))`

func (s productionAuthStore) begin(ctx context.Context, state, browser []byte, nonce, verifier string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO production_auth_oidc_flows (state_hash,browser_hash,nonce,verifier,expires_at)
		VALUES ($1,$2,$3,$4,clock_timestamp()+($5::int*interval '1 second'))`, state, browser, nonce, verifier, int(productionLoginTTL.Seconds()))
	return err
}

func (s productionAuthStore) consume(ctx context.Context, state, browser []byte) (string, string, error) {
	var nonce, verifier string
	err := s.db.QueryRow(ctx, `DELETE FROM production_auth_oidc_flows
    WHERE state_hash=$1 AND browser_hash=$2 AND expires_at>clock_timestamp()
    RETURNING nonce,verifier`, state, browser).Scan(&nonce, &verifier)
	return nonce, verifier, err
}

func (s productionAuthStore) issue(ctx context.Context, issuer, subject string, hash, previous []byte) (productionActor, error) {
	var actor productionActor
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var revision int64
		err := tx.QueryRow(ctx, `SELECT i.id::text,i.role,COALESCE(i.merchant_id::text,''),i.revision
      FROM production_auth_identities AS i WHERE i.issuer=$1 AND i.subject=$2 AND `+productionActiveIdentity+` FOR SHARE OF i`,
			issuer, subject).Scan(&actor.ID, &actor.Role, &actor.MerchantID, &revision)
		if err != nil {
			return err
		}
		if previous != nil {
			if _, err := tx.Exec(ctx, `UPDATE production_auth_sessions SET revoked_at=clock_timestamp()
        WHERE token_hash=$1 AND revoked_at IS NULL`, previous); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO production_auth_sessions (token_hash,identity_id,identity_revision,started_at,last_seen_at,expires_at)
			VALUES ($1,$2::uuid,$3,statement_timestamp(),statement_timestamp(),statement_timestamp()+($4::int*interval '1 second'))`,
			hash, actor.ID, revision, int(productionSessionTTL.Seconds())); err != nil {
			return err
		}
		return writeProductionAuthAudit(ctx, tx, actor, "login_succeeded", "session_created")
	})
	return actor, err
}

func (s productionAuthStore) actor(ctx context.Context, hash []byte, touch bool) (productionActor, error) {
	var actor productionActor
	// A read-only session probe never renews inactivity. Business requests advance
	// last_seen_at at most once per minute, without extending absolute expiry.
	err := s.db.QueryRow(ctx, `WITH valid_session AS (
      SELECT t.token_hash,i.id::text AS actor_id,i.role,COALESCE(i.merchant_id::text,'') AS merchant_id
      FROM production_auth_sessions AS t JOIN production_auth_identities AS i ON i.id=t.identity_id
      WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND t.expires_at>statement_timestamp()
        AND t.last_seen_at>statement_timestamp()-interval '30 minutes' AND i.revision=t.identity_revision
        AND `+productionActiveIdentity+`
    ), touched AS (
      UPDATE production_auth_sessions AS t SET last_seen_at=statement_timestamp() FROM valid_session AS v
      WHERE $2 AND t.token_hash=v.token_hash AND t.revoked_at IS NULL
        AND t.last_seen_at<=statement_timestamp()-interval '1 minute'
    ) SELECT actor_id,role,merchant_id FROM valid_session`, hash, touch).Scan(&actor.ID, &actor.Role, &actor.MerchantID)
	return actor, err
}

func (s productionAuthStore) revoke(ctx context.Context, hash []byte) error {
	if hash == nil {
		return nil
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE production_auth_sessions AS t SET revoked_at=clock_timestamp()
      FROM production_auth_identities AS i WHERE t.token_hash=$1 AND t.identity_id=i.id AND t.revoked_at IS NULL
      RETURNING i.id::text,i.role,COALESCE(i.merchant_id::text,'')`, hash)
		if err != nil {
			return err
		}
		actors, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (productionActor, error) {
			var actor productionActor
			err := row.Scan(&actor.ID, &actor.Role, &actor.MerchantID)
			return actor, err
		})
		if err != nil || len(actors) == 0 {
			return err
		}
		return writeProductionAuthAudit(ctx, tx, actors[0], "logout", "session_revoked")
	})
}

type productionAuditWriter interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func writeProductionAuthAudit(ctx context.Context, db productionAuditWriter, actor productionActor, event, reason string) error {
	_, err := db.Exec(ctx, `INSERT INTO production_auth_audit (actor_id,role,event,reason)
    VALUES (NULLIF($1,'')::uuid,NULLIF($2,''),$3,$4)`, actor.ID, actor.Role, event, reason)
	return err
}

func (s productionAuthStore) audit(ctx context.Context, actor productionActor, event, reason string) error {
	return writeProductionAuthAudit(ctx, s.db, actor, event, reason)
}

func (s productionAuthStore) pruneExpired(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		// Use bounded statements so expiry cleanup doesn't block request traffic.
		pruneCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := s.db.Exec(pruneCtx, `DELETE FROM production_auth_oidc_flows WHERE state_hash IN (
        SELECT state_hash FROM production_auth_oidc_flows WHERE expires_at<=clock_timestamp() LIMIT 1000)`)
		if err == nil {
			_, err = s.db.Exec(pruneCtx, `DELETE FROM production_auth_sessions WHERE token_hash IN (
          SELECT token_hash FROM production_auth_sessions WHERE expires_at<=clock_timestamp() LIMIT 1000)`)
		}
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Print("production authentication expiry cleanup failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
