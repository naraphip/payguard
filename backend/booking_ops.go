package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const bookingOpsSchema = `
CREATE TABLE IF NOT EXISTS booking_contact_audit (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  merchant_id text NOT NULL,
  event_id uuid NOT NULL,
  actor_id text NOT NULL,
  reason text NOT NULL CHECK (reason IN ('manual_review', 'support')),
  accessed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY (merchant_id, event_id) REFERENCES booking_events (merchant_id, event_id)
);
CREATE INDEX IF NOT EXISTS booking_contact_audit_event_idx ON booking_contact_audit (merchant_id, event_id, id);
`

const bookingOpsPath = "/api/v1/booking/ops/transactions"

type bookingOpsHandler struct {
	repo   repository
	auth   bookingAuth
	secret []byte
}

type bookingOpsContact struct {
	State string `json:"state"`
	Email string `json:"email,omitempty"`
}

type bookingOpsDetail struct {
	Booking    bookingEvent      `json:"booking"`
	Assessment json.RawMessage   `json:"assessment"`
	Contact    bookingOpsContact `json:"contact"`
	Actor      bookingActor      `json:"actor"`
	Hold       *bookingHold      `json:"hold,omitempty"`
}

func registerBookingOps(mux *http.ServeMux, repo repository, auth bookingAuth, secret []byte) {
	handler := bookingOpsHandler{repo, auth, secret}
	mux.HandleFunc("GET "+bookingOpsPath, handler.transactions)
	mux.HandleFunc("GET "+bookingOpsPath+"/{id}", handler.detail)
	mux.HandleFunc("POST "+bookingOpsPath+"/{id}/reveal", handler.revealContact)
	for _, action := range []string{"claim", "escalate", "resolve"} {
		mux.HandleFunc("POST "+bookingOpsPath+"/{id}/hold/"+action, handler.changeHold(action))
	}
}

func (h bookingOpsHandler) readActor(w http.ResponseWriter, r *http.Request) bookingActor {
	return h.auth.requireBookingActor(w, r, bookingRoleMerchant, bookingRoleViewer, bookingRoleAnalyst)
}

func bookingOpsEventID(w http.ResponseWriter, r *http.Request) string {
	eventID := r.PathValue("id")
	if !validID.MatchString(eventID) {
		problem(w, http.StatusBadRequest, "Expected a lowercase booking event UUID.")
		return ""
	}
	return eventID
}

func (h bookingOpsHandler) detail(w http.ResponseWriter, r *http.Request) {
	actor := h.readActor(w, r)
	if actor.MerchantID == "" {
		return
	}
	eventID := bookingOpsEventID(w, r)
	if eventID == "" {
		return
	}

	detail := bookingOpsDetail{Actor: actor, Contact: bookingOpsContact{State: "UNAVAILABLE"}}
	booking := &detail.Booking
	booking.MerchantID = actor.MerchantID
	booking.EventID = eventID
	var hold bookingHold
	var state *string
	var dueAt *time.Time
	var now time.Time
	err := h.repo.db.QueryRow(r.Context(), `
		SELECT e.schema_version, e.stage, e.occurred_at, e.amount_minor, e.product, e.sdk_status, e.received_at, a.result,
		  CASE WHEN a.result->>'handoff' = 'MANUAL_REVIEW' THEN COALESCE(h.state, 'OPEN') END,
		  CASE WHEN a.result->>'handoff' = 'MANUAL_REVIEW' THEN COALESCE(h.due_at, `+bookingHoldDue+`) END,
		  COALESCE(h.assigned_actor, ''), COALESCE(h.resolution, ''), h.escalated_at, h.resolved_at, statement_timestamp()
		FROM booking_events e LEFT JOIN booking_assessments a USING (merchant_id, event_id)
		LEFT JOIN booking_holds h USING (merchant_id, event_id)
		WHERE e.merchant_id = $1 AND e.event_id = $2::text::uuid`, actor.MerchantID, eventID).
		Scan(&booking.SchemaVersion, &booking.Stage, &booking.OccurredAt, &booking.AmountMinor, &booking.Product,
			&booking.SDKStatus, &booking.ReceivedAt, &detail.Assessment, &state, &dueAt, &hold.AssignedActor,
			&hold.Resolution, &hold.EscalatedAt, &hold.ResolvedAt, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, http.StatusNotFound, "Booking event not found.")
		return
	}
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	booking.OccurredAt = booking.OccurredAt.UTC()
	booking.ReceivedAt = booking.ReceivedAt.UTC()
	if booking.SDKStatus == "" {
		booking.SDKStatus = "unknown"
	}
	if booking.Product != "" {
		detail.Contact = bookingOpsContact{State: "MASKED", Email: "b•••@checkout.test"}
	}
	if state != nil && dueAt != nil {
		hold.State, hold.DueAt = *state, *dueAt
		hold.normalize(now)
		detail.Hold = &hold
	}
	respond(w, http.StatusOK, detail)
}

func (h bookingOpsHandler) revealContact(w http.ResponseWriter, r *http.Request) {
	actor := h.auth.requireBookingActor(w, r, bookingRoleAnalyst)
	if actor.MerchantID == "" {
		return
	}
	eventID := bookingOpsEventID(w, r)
	if eventID == "" {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if input.Reason != "manual_review" && input.Reason != "support" {
		problem(w, http.StatusUnprocessableEntity, "Choose a supported contact access reason.")
		return
	}
	var auditID int64
	// A single committed statement records access before the contact is returned.
	err := h.repo.db.QueryRow(r.Context(), `
		INSERT INTO booking_contact_audit (merchant_id, event_id, actor_id, reason)
		SELECT merchant_id, event_id, $3, $4
		FROM booking_events WHERE merchant_id = $1 AND event_id = $2::text::uuid AND product <> ''
		RETURNING id`, actor.MerchantID, eventID, actor.ActorID, input.Reason).Scan(&auditID)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, http.StatusNotFound, "Synthetic booking contact not available.")
		return
	}
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, struct {
		Email   string `json:"email"`
		AuditID int64  `json:"auditId"`
	}{syntheticBookingEmail(h.secret, actor.MerchantID, eventID), auditID})
}

func syntheticBookingEmail(secret []byte, merchant, eventID string) string {
	hash := hmac.New(sha256.New, secret)
	_, _ = hash.Write([]byte("checkout-contact:" + merchant + ":" + eventID))
	return "booking-" + hex.EncodeToString(hash.Sum(nil)[:10]) + "@checkout.test"
}
