package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type bookingQueueFilters struct {
	Outcome string
	Product string
	Hold    string
	Limit   int
}

type bookingQueueRow struct {
	EventID     string       `json:"eventId"`
	Product     string       `json:"product,omitempty"`
	ReceivedAt  time.Time    `json:"receivedAt"`
	AssessedAt  *time.Time   `json:"assessedAt,omitempty"`
	Outcome     string       `json:"outcome,omitempty"`
	AmountMinor *int64       `json:"amountMinor,omitempty"`
	SDKStatus   string       `json:"sdkStatus"`
	Hold        *bookingHold `json:"hold,omitempty"`
}

type bookingQueueKPIs struct {
	Total                    int64      `json:"total"`
	Assessed                 int64      `json:"assessed"`
	ManualReview             int64      `json:"manualReview"`
	OpenHolds                int64      `json:"openHolds"`
	OverdueHolds             int64      `json:"overdueHolds"`
	AverageAssessmentLatency *float64   `json:"averageAssessmentLatencyMs"`
	LatestReceivedAt         *time.Time `json:"latestReceivedAt"`
	FreshnessSeconds         *int64     `json:"freshnessSeconds"`
}

type bookingQueue struct {
	Transactions []bookingQueueRow `json:"transactions"`
	KPIs         bookingQueueKPIs  `json:"kpis"`
	Actor        bookingActor      `json:"actor"`
}

func (filters bookingQueueFilters) valid() bool {
	outcomeValid := filters.Outcome == "" || filters.Outcome == "LOW_RISK" || filters.Outcome == "REVIEW" || filters.Outcome == "UNRESOLVED"
	holdValid := filters.Hold == "" || filters.Hold == holdOpen || filters.Hold == holdClaimed ||
		filters.Hold == holdEscalated || filters.Hold == holdResolved
	return outcomeValid && holdValid && validBookingProduct(filters.Product) && filters.Limit >= 1 && filters.Limit <= 100
}

func (h bookingOpsHandler) transactions(w http.ResponseWriter, r *http.Request) {
	actor := h.readActor(w, r)
	if actor.MerchantID == "" {
		return
	}
	query := r.URL.Query()
	filters := bookingQueueFilters{Outcome: query.Get("outcome"), Product: query.Get("product"), Hold: query.Get("hold"), Limit: 30}
	if value := query.Get("limit"); value != "" {
		filters.Limit, _ = strconv.Atoi(value)
	}
	if !filters.valid() {
		problem(w, http.StatusBadRequest, "Choose supported outcome, product, and HOLD filters and a limit from 1 to 100.")
		return
	}
	queue, err := h.repo.bookingQueue(r.Context(), actor, filters)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, queue)
}

const bookingQueueRows = `
WITH transaction_rows AS (
  SELECT e.event_id, e.product, e.received_at, (a.result->>'assessedAt')::timestamptz AS assessed_at,
    COALESCE(a.result->>'outcome','') AS outcome, e.amount_minor,
    COALESCE(NULLIF(e.sdk_status,''), 'unknown') AS sdk_status,
    CASE WHEN a.result->>'handoff' = 'MANUAL_REVIEW' THEN COALESCE(h.state, 'OPEN') END AS hold_state,
    CASE WHEN a.result->>'handoff' = 'MANUAL_REVIEW' THEN COALESCE(h.due_at, ` + bookingHoldDue + `) END AS due_at,
    COALESCE(h.assigned_actor,'') AS assigned_actor, COALESCE(h.resolution,'') AS resolution, h.escalated_at, h.resolved_at
  FROM booking_events e LEFT JOIN booking_assessments a USING (merchant_id, event_id)
  LEFT JOIN booking_holds h USING (merchant_id, event_id)
  WHERE e.merchant_id = $1
)
`

func (r repository) bookingQueue(ctx context.Context, actor bookingActor, filters bookingQueueFilters) (bookingQueue, error) {
	queue := bookingQueue{Transactions: []bookingQueueRow{}, Actor: actor}
	err := pgx.BeginTxFunc(ctx, r.db, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var now time.Time
		kpis := &queue.KPIs
		err := tx.QueryRow(ctx, bookingQueueRows+`
			SELECT count(*), count(*) FILTER (WHERE outcome <> ''), count(hold_state),
			  count(*) FILTER (WHERE hold_state <> 'RESOLVED'),
			  count(*) FILTER (WHERE hold_state <> 'RESOLVED' AND escalated_at IS NULL AND due_at <= transaction_timestamp()),
			  avg(extract(epoch from assessed_at - received_at) * 1000) FILTER (WHERE assessed_at >= received_at)::double precision,
			  max(received_at), transaction_timestamp()
			FROM transaction_rows`, actor.MerchantID).Scan(&kpis.Total, &kpis.Assessed, &kpis.ManualReview,
			&kpis.OpenHolds, &kpis.OverdueHolds, &kpis.AverageAssessmentLatency, &kpis.LatestReceivedAt, &now)
		if err != nil {
			return err
		}
		if kpis.LatestReceivedAt != nil {
			latest := kpis.LatestReceivedAt.UTC()
			kpis.LatestReceivedAt = &latest
			freshness := max(int64(now.Sub(latest)/time.Second), 0)
			kpis.FreshnessSeconds = &freshness
		}
		rows, err := tx.Query(ctx, bookingQueueRows+`
			SELECT event_id::text, product, received_at, assessed_at, outcome, amount_minor, sdk_status,
			  hold_state, due_at, assigned_actor, resolution, escalated_at, resolved_at
			FROM transaction_rows
			WHERE ($2 = '' OR outcome = $2) AND ($3 = '' OR product = $3) AND ($4 = '' OR hold_state = $4)
			ORDER BY received_at DESC, event_id DESC LIMIT $5`, actor.MerchantID, filters.Outcome, filters.Product, filters.Hold, filters.Limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row bookingQueueRow
			var hold bookingHold
			var state *string
			var dueAt *time.Time
			if err := rows.Scan(&row.EventID, &row.Product, &row.ReceivedAt, &row.AssessedAt, &row.Outcome, &row.AmountMinor,
				&row.SDKStatus, &state, &dueAt, &hold.AssignedActor, &hold.Resolution, &hold.EscalatedAt, &hold.ResolvedAt); err != nil {
				return err
			}
			if state != nil && dueAt != nil {
				hold.State, hold.DueAt = *state, *dueAt
				hold.normalize(now)
				row.Hold = &hold
			}
			row.ReceivedAt = row.ReceivedAt.UTC()
			if row.AssessedAt != nil {
				at := row.AssessedAt.UTC()
				row.AssessedAt = &at
			}
			queue.Transactions = append(queue.Transactions, row)
		}
		return rows.Err()
	})
	return queue, err
}
