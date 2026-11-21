package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const merchantManagementSchema = `
ALTER TABLE merchant_demo_merchants ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';
ALTER TABLE merchant_demo_merchants ADD COLUMN IF NOT EXISTS creation_request_hash bytea;
CREATE UNIQUE INDEX IF NOT EXISTS merchant_demo_creation_request_idx
	ON merchant_demo_merchants(creation_request_hash) WHERE creation_request_hash IS NOT NULL;
-- Existing creation timestamps are unknown. Only new merchants receive a timestamp.
ALTER TABLE merchant_demo_merchants ADD COLUMN IF NOT EXISTS created_at timestamptz;
ALTER TABLE merchant_demo_merchants ALTER COLUMN created_at SET DEFAULT clock_timestamp();
CREATE TABLE IF NOT EXISTS merchant_demo_audit (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	merchant_id uuid NOT NULL REFERENCES merchant_demo_merchants(id),
	action text NOT NULL,
	actor text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS merchant_demo_audit_merchant_idx
	ON merchant_demo_audit (merchant_id,created_at DESC);
`

type merchantPage struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

func merchantPagination(values url.Values) (merchantPage, error) {
	page := merchantPage{Page: 1, Limit: 20}
	for _, item := range []struct {
		key   string
		value *int
		max   int
	}{{"page", &page.Page, 10000}, {"limit", &page.Limit, 100}} {
		if values.Has(item.key) {
			n, err := strconv.Atoi(values.Get(item.key))
			if err != nil || n < 1 || n > item.max {
				return page, fmt.Errorf("Invalid %s parameter.", item.key)
			}
			*item.value = n
		}
	}
	return page, nil
}

type merchantManagementRow struct {
	mockMerchant
	TransactionCount int64      `json:"transactionCount"`
	UnderReview      int64      `json:"underReview"`
	RuleCount        int64      `json:"ruleCount"`
	LastActivity     *time.Time `json:"lastActivity,omitempty"`
}

func (s mockMerchantService) managedMerchantList(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, err := merchantPagination(values)
	query := strings.TrimSpace(values.Get("q"))
	if err != nil || utf8.RuneCountInString(query) > 100 {
		problem(w, http.StatusBadRequest, "Use valid pagination and a search of up to 100 characters.")
		return
	}
	var order string
	switch values.Get("sort") {
	case "", "name":
		order = "lower(m.name),m.id"
	case "created":
		order = "m.created_at DESC NULLS LAST,m.id"
	case "activity":
		order = "activity.at DESC NULLS LAST,m.id"
	default:
		problem(w, http.StatusBadRequest, "Choose name, created, or activity sorting.")
		return
	}
	ctx := r.Context()
	tx, err := s.repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		demoWriteError(w, err)
		return
	}
	defer tx.Rollback(ctx)
	const selection = `m.deleted_at IS NULL AND ($1='' OR strpos(lower(m.name),lower($1))>0 OR strpos(m.id::text,lower($1))>0)`
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM merchant_demo_merchants m WHERE `+selection, query).Scan(&page.Total); err != nil {
		demoWriteError(w, err)
		return
	}
	rows, err := tx.Query(ctx, `
		WITH selected AS (
			SELECT m.id,m.name,m.description,m.setup,m.policy_integration,m.created_at,activity.at
			FROM merchant_demo_merchants m
			LEFT JOIN LATERAL (
				SELECT created_at AS at FROM merchant_demo_transactions WHERE merchant_id=m.id ORDER BY created_at DESC LIMIT 1
			) activity ON true
			WHERE `+selection+` ORDER BY `+order+`
			LIMIT $2 OFFSET $3
		)
		SELECT m.id::text,m.name,m.description,m.setup,m.policy_integration,m.created_at,m.at,
			stats.total,stats.review,policies.total
		FROM selected m LEFT JOIN LATERAL (
			SELECT count(*) AS total,count(*) FILTER (WHERE outcome<>'LOW_RISK' AND disposition IS NULL) AS review
			FROM merchant_demo_transactions WHERE merchant_id=m.id
		) stats ON true
		LEFT JOIN LATERAL (
			SELECT count(*) AS total FROM (
				SELECT DISTINCT ON (rule_id) status,`+merchantPolicyExpiry+` AS expires_at FROM merchant_policy_rule_versions
				WHERE merchant_id=m.id AND status<>'PENDING_APPROVAL' ORDER BY rule_id,version DESC
			) rules WHERE status='ACTIVE' AND (expires_at IS NULL OR expires_at>statement_timestamp())
		) policies ON true
		ORDER BY `+strings.ReplaceAll(order, "activity.at", "m.at"), query, page.Limit, (page.Page-1)*page.Limit)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	defer rows.Close()
	merchants := []merchantManagementRow{}
	for rows.Next() {
		var item merchantManagementRow
		item.Scenarios = []mockScenario{}
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Setup, &item.PolicyIntegration, &item.CreatedAt, &item.LastActivity,
			&item.TransactionCount, &item.UnderReview, &item.RuleCount); err != nil {
			demoWriteError(w, err)
			return
		}
		merchants = append(merchants, item)
	}
	if err := rows.Err(); err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, struct {
		merchantPage
		Role      string                  `json:"role"`
		Merchants []merchantManagementRow `json:"merchants"`
	}{page, "admin", merchants})
}

type merchantMetrics struct {
	WindowDays     int   `json:"windowDays"`
	Total          int64 `json:"totalTransactions"`
	Allowed        int64 `json:"allowed"`
	Challenged     int64 `json:"challenged"`
	UnderReview    int64 `json:"underReview"`
	Blocked        int64 `json:"blocked"`
	ActivePolicies int   `json:"activePolicies"`
}

const merchantTransactionAction = `COALESCE(t.policy_decision #>> '{finalDecision,action}', CASE WHEN t.outcome='LOW_RISK' THEN 'ALLOW' ELSE t.outcome END)`
const merchantTransactionReview = `CASE WHEN t.disposition IS NOT NULL THEN 'RESOLVED' WHEN t.outcome='LOW_RISK' THEN 'NONE' ELSE 'NEEDS_DECISION' END`

func (s mockMerchantService) managementMerchantID(w http.ResponseWriter, r *http.Request) (string, bool) {
	if _, ok := s.authorize(w, r, false); !ok {
		return "", false
	}
	return demoPathID(w, r, "merchantId")
}

func (s mockMerchantService) managedMerchantDetails(w http.ResponseWriter, r *http.Request) {
	id, ok := s.managementMerchantID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		demoWriteError(w, err)
		return
	}
	defer tx.Rollback(ctx)
	merchants, err := scanMockMerchants(ctx, tx, id)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	if len(merchants) == 0 {
		demoWriteError(w, pgx.ErrNoRows)
		return
	}
	profile, _, err := readMerchantPolicyContext(ctx, tx, id)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	metrics := merchantMetrics{WindowDays: 30}
	err = tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE action='ALLOW'), count(*) FILTER (WHERE action='CHALLENGE'),
		count(*) FILTER (WHERE review='NEEDS_DECISION'),count(*) FILTER (WHERE action='BLOCK')
		FROM (SELECT `+merchantTransactionAction+` AS action,`+merchantTransactionReview+` AS review
		FROM merchant_demo_transactions t WHERE merchant_id=$1::text::uuid AND created_at>=now()-interval '30 days') events`, id).
		Scan(&metrics.Total, &metrics.Allowed, &metrics.Challenged, &metrics.UnderReview, &metrics.Blocked)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	active, err := readMerchantPolicies(ctx, tx, id, true)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	metrics.ActivePolicies = len(active)
	recent, err := readManagedTransactions(ctx, tx, mockTransactionReadQuery+` ORDER BY t.created_at DESC,t.id DESC LIMIT 5`, id)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"merchant": merchants[0], "metrics": metrics, "profile": profile, "recentTransactions": recent})
}

func readManagedTransactions(ctx context.Context, db mockMerchantQueryer, query string, args ...any) ([]mockTransaction, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	transactions := []mockTransaction{}
	for rows.Next() {
		item, err := scanMockTransaction(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, item)
	}
	return transactions, rows.Err()
}

func merchantFilterDate(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	date, err := time.Parse("2006-01-02", raw)
	return &date, err
}

func (s mockMerchantService) managedTransactionPage(w http.ResponseWriter, r *http.Request, id string) {
	values := r.URL.Query()
	page, err := merchantPagination(values)
	from, fromErr := merchantFilterDate(values.Get("from"))
	to, toErr := merchantFilterDate(values.Get("to"))
	outcome, review, search := values.Get("outcome"), values.Get("reviewState"), strings.ToLower(strings.TrimSpace(values.Get("q")))
	if err != nil || fromErr != nil || toErr != nil || from != nil && to != nil && from.After(*to) || len(search) > 36 ||
		!slices.Contains([]string{"", "LOW_RISK", "ALLOW", "BLOCK", "CHALLENGE", "REVIEW", "UNRESOLVED"}, outcome) ||
		!slices.Contains([]string{"", "NONE", "NEEDS_DECISION", "RESOLVED"}, review) {
		problem(w, http.StatusBadRequest, "Use valid transaction filters, pagination, and UTC dates.")
		return
	}
	if to != nil {
		next := to.AddDate(0, 0, 1)
		to = &next
	}
	ctx := r.Context()
	tx, err := s.repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		demoWriteError(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `SELECT id::text FROM merchant_demo_merchants WHERE id=$1::text::uuid AND deleted_at IS NULL`, id).Scan(&id); err != nil {
		demoWriteError(w, err)
		return
	}
	filter := ` AND ($2='' OR strpos(t.id::text,$2)=1)
		AND ($3='' OR ` + merchantTransactionAction + `=$3 OR ($3='LOW_RISK' AND t.outcome='LOW_RISK'))
		AND ($4='' OR ` + merchantTransactionReview + `=$4)
		AND ($5::timestamptz IS NULL OR t.created_at>=$5) AND ($6::timestamptz IS NULL OR t.created_at<$6)`
	args := []any{id, search, outcome, review, from, to}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM merchant_demo_transactions t WHERE t.merchant_id=$1::text::uuid`+filter, args...).Scan(&page.Total); err != nil {
		demoWriteError(w, err)
		return
	}
	transactions, err := readManagedTransactions(ctx, tx, mockTransactionReadQuery+filter+` ORDER BY t.created_at DESC,t.id DESC LIMIT $7 OFFSET $8`,
		append(args, page.Limit, (page.Page-1)*page.Limit)...)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, struct {
		merchantPage
		Transactions []mockTransaction `json:"transactions"`
	}{page, transactions})
}

func (s mockMerchantService) managedMerchantPolicies(w http.ResponseWriter, r *http.Request) {
	id, ok := s.managementMerchantID(w, r)
	if !ok {
		return
	}
	if _, err := s.merchantPolicyProfile(r.Context(), id); err != nil {
		merchantPolicyError(w, err)
		return
	}
	s.respondMerchantPolicies(w, r, id)
}

func (s mockMerchantService) managedMerchantPolicyHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := s.managementMerchantID(w, r)
	if !ok {
		return
	}
	if _, err := s.merchantPolicyProfile(r.Context(), id); err != nil {
		merchantPolicyError(w, err)
		return
	}
	s.respondMerchantPolicyHistory(w, r, id)
}

type merchantAuditEntry struct {
	ID              string    `json:"id"`
	Action          string    `json:"action"`
	Actor           string    `json:"actor"`
	ResourceID      string    `json:"resourceId"`
	PreviousVersion *int      `json:"previousVersion,omitempty"`
	Version         *int      `json:"version,omitempty"`
	OccurredAt      time.Time `json:"occurredAt"`
	Reason          string    `json:"reason,omitempty"`
}

func auditMerchant(ctx context.Context, tx pgx.Tx, id, action string) error {
	_, err := tx.Exec(ctx, `INSERT INTO merchant_demo_audit(merchant_id,action,actor) VALUES($1::text::uuid,$2,'platform-admin')`, id, action)
	return err
}

func (s mockMerchantService) managedMerchantAudit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.managementMerchantID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var exists string
	err := s.repo.db.QueryRow(ctx, `SELECT id::text FROM merchant_demo_merchants WHERE id=$1::text::uuid AND deleted_at IS NULL`, id).Scan(&exists)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	rows, err := s.repo.db.Query(ctx, `
		SELECT * FROM (
			SELECT id::text,action,actor,merchant_id::text AS resource_id,
				NULL::int prior,NULL::int version,created_at AS occurred_at,'' AS reason
				FROM merchant_demo_audit WHERE merchant_id=$1::text::uuid
			UNION ALL
			SELECT rule_id::text||':'||version,'POLICY_'||upper(change_reason),'merchant',rule_id::text,
				NULLIF(version-1,0),version,created_at,COALESCE(preview->>'approvalReason','')
				FROM merchant_policy_rule_versions WHERE merchant_id=$1::text::uuid
			UNION ALL
			SELECT id::text||':review','REVIEW_RESOLVED',decided_by,id::text,NULL,NULL,decided_at,disposition
				FROM merchant_demo_transactions WHERE merchant_id=$1::text::uuid AND decided_at IS NOT NULL
		) entries ORDER BY occurred_at DESC,1 DESC LIMIT 100`, id)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	defer rows.Close()
	events := []merchantAuditEntry{}
	for rows.Next() {
		var event merchantAuditEntry
		if err := rows.Scan(&event.ID, &event.Action, &event.Actor, &event.ResourceID, &event.PreviousVersion, &event.Version, &event.OccurredAt, &event.Reason); err != nil {
			demoWriteError(w, err)
			return
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"events": events})
}
