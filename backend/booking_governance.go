package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

const bookingGovernanceSchema = `
CREATE TABLE IF NOT EXISTS booking_change_proposals (
  merchant_id text NOT NULL,
  id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('profile', 'rule', 'rubric', 'list')),
  maker_id text NOT NULL,
  request_digest text NOT NULL,
  candidate jsonb NOT NULL,
  shadow jsonb,
  rollback_of uuid,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (merchant_id, id),
  FOREIGN KEY (merchant_id, rollback_of) REFERENCES booking_change_proposals (merchant_id, id)
);
CREATE INDEX IF NOT EXISTS booking_change_proposals_latest
  ON booking_change_proposals (merchant_id, created_at DESC, id);
CREATE TABLE IF NOT EXISTS booking_change_decisions (
  merchant_id text NOT NULL,
  proposal_id uuid NOT NULL,
  checker_id text NOT NULL,
  decision text NOT NULL CHECK (decision IN ('approve', 'reject')),
  decided_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (merchant_id, proposal_id),
  FOREIGN KEY (merchant_id, proposal_id) REFERENCES booking_change_proposals (merchant_id, id)
);
CREATE TABLE IF NOT EXISTS booking_active_changes (
  merchant_id text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('profile', 'rule', 'rubric', 'list')),
  proposal_id uuid NOT NULL,
  PRIMARY KEY (merchant_id, kind),
  FOREIGN KEY (merchant_id, proposal_id) REFERENCES booking_change_decisions (merchant_id, proposal_id)
);
`

const bookingChangesPath = "/api/v1/booking/changes"

var (
	errBookingChangeConflict = errors.New("proposal ID or checker decision is already in use")
	errBookingChangeExpired  = errors.New("proposal has expired")
	errBookingChangeSelf     = errors.New("the maker cannot check this proposal")
	errBookingChangeRollback = errors.New("choose an approved revision of the same kind to roll back")
)

type bookingChangeRule struct {
	ReviewThreshold int64 `json:"reviewThreshold"`
}

type bookingChangeRubric struct {
	ElevatedVelocity int64 `json:"elevatedVelocity"`
}

type bookingChangeList struct {
	ReviewProducts []string `json:"reviewProducts"`
}

type bookingChangeRequest struct {
	ID           string               `json:"id"`
	Kind         string               `json:"kind"`
	ExpiresAt    time.Time            `json:"expiresAt"`
	Candidate    json.RawMessage      `json:"candidate,omitempty"`
	RollbackOf   string               `json:"rollbackOf,omitempty"`
	ShadowWindow *bookingShadowWindow `json:"shadowWindow,omitempty"`
}

type bookingChangeDecision struct {
	CheckerID string    `json:"checkerId"`
	Decision  string    `json:"decision"`
	DecidedAt time.Time `json:"decidedAt"`
}

type bookingChange struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"`
	MakerID   string                 `json:"makerId"`
	Candidate json.RawMessage        `json:"candidate"`
	Shadow    json.RawMessage        `json:"shadow,omitempty"`
	Rollback  *string                `json:"rollbackOf,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	ExpiresAt time.Time              `json:"expiresAt"`
	Status    string                 `json:"status"`
	Revision  string                 `json:"revision,omitempty"`
	Active    bool                   `json:"active"`
	Decision  *bookingChangeDecision `json:"decision,omitempty"`
	digest    string
}

type bookingGovernanceHandler struct {
	repo repository
	auth bookingAuth
}

func registerBookingGovernance(mux *http.ServeMux, repo repository, auth bookingAuth) {
	h := bookingGovernanceHandler{repo, auth}
	mux.HandleFunc("GET "+bookingChangesPath, h.list)
	mux.HandleFunc("GET "+bookingChangesPath+"/{id}", h.read)
	mux.HandleFunc("POST "+bookingChangesPath, h.propose)
	mux.HandleFunc("POST "+bookingChangesPath+"/{id}/decision", h.decide)
}

func decodeBookingChangeCandidate(kind string, raw json.RawMessage) (json.RawMessage, int64, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, 0, errors.New("provide a typed candidate for the change")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decode := func(value any) error {
		if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
			return errors.New("candidate does not match the change kind")
		}
		return nil
	}
	var candidate any
	var threshold int64
	switch kind {
	case "profile":
		var profile bookingProfile
		if err := decode(&profile); err != nil {
			return nil, 0, err
		}
		if err := validateBookingProfile(&profile); err != nil {
			return nil, 0, err
		}
		candidate = profile
	case "rule":
		var rule bookingChangeRule
		if err := decode(&rule); err != nil {
			return nil, 0, err
		}
		if rule.ReviewThreshold < 1 {
			return nil, 0, errors.New("reviewThreshold must be positive")
		}
		threshold = rule.ReviewThreshold
		candidate = rule
	case "rubric":
		var rubric bookingChangeRubric
		if err := decode(&rubric); err != nil {
			return nil, 0, err
		}
		if rubric.ElevatedVelocity < 1 {
			return nil, 0, errors.New("elevatedVelocity must be positive")
		}
		candidate = rubric
	case "list":
		var list bookingChangeList
		if err := decode(&list); err != nil {
			return nil, 0, err
		}
		if list.ReviewProducts == nil {
			return nil, 0, errors.New("provide a reviewProducts array")
		}
		for _, product := range list.ReviewProducts {
			if product == "" || !validBookingProduct(product) {
				return nil, 0, errors.New("list contains an unsupported booking product")
			}
		}
		slices.Sort(list.ReviewProducts)
		compacted := slices.Compact(list.ReviewProducts)
		if len(compacted) != len(list.ReviewProducts) {
			return nil, 0, errors.New("list products must be distinct")
		}
		candidate = list
	default:
		return nil, 0, errors.New("choose profile, rule, rubric or list")
	}
	canonical, err := json.Marshal(candidate)
	return canonical, threshold, err
}

func (h bookingGovernanceHandler) propose(w http.ResponseWriter, r *http.Request) {
	actor := h.auth.requireBookingActor(w, r, bookingRoleMerchant)
	if actor.MerchantID == "" {
		return
	}
	var input bookingChangeRequest
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if !validID.MatchString(input.ID) || !validBookingTime(input.ExpiresAt) ||
		(input.RollbackOf != "" && (!validID.MatchString(input.RollbackOf) || input.RollbackOf == input.ID || len(input.Candidate) > 0)) {
		problem(w, http.StatusUnprocessableEntity, "Expected a proposal UUID, expiry time and either a candidate or a rollback revision UUID.")
		return
	}

	input.ExpiresAt = input.ExpiresAt.UTC()
	rawCandidate := input.Candidate
	if input.RollbackOf != "" {
		err := h.repo.db.QueryRow(r.Context(), `
			SELECT p.candidate FROM booking_change_proposals AS p
			JOIN booking_change_decisions AS d ON d.merchant_id = p.merchant_id AND d.proposal_id = p.id
			WHERE p.merchant_id = $1 AND p.id = $2::text::uuid AND p.kind = $3 AND d.decision = 'approve'`,
			actor.MerchantID, input.RollbackOf, input.Kind).Scan(&rawCandidate)
		if errors.Is(err, pgx.ErrNoRows) {
			problem(w, http.StatusUnprocessableEntity, errBookingChangeRollback.Error())
			return
		}
		if err != nil {
			bookingRiskUnavailable(w, err)
			return
		}
	}
	candidate, threshold, err := decodeBookingChangeCandidate(input.Kind, rawCandidate)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	input.Candidate = candidate
	if input.Kind == "rule" {
		if input.ShadowWindow == nil || input.ShadowWindow.ReviewThreshold != threshold || !input.ShadowWindow.valid(time.Now()) {
			problem(w, http.StatusUnprocessableEntity, "Provide a valid temporal shadow window using the candidate threshold.")
			return
		}
		input.ShadowWindow.HoldoutStart = input.ShadowWindow.HoldoutStart.UTC()
		input.ShadowWindow.HoldoutEnd = input.ShadowWindow.HoldoutEnd.UTC()
		input.ShadowWindow.AsOf = input.ShadowWindow.AsOf.UTC()
	} else if input.ShadowWindow != nil {
		problem(w, http.StatusUnprocessableEntity, "Shadow windows apply only to rule changes.")
		return
	}

	canonical, err := json.Marshal(input)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	hash := sha256.Sum256(canonical)
	digest := hex.EncodeToString(hash[:])
	existing, err := lookupBookingChange(r.Context(), h.repo.db, actor.MerchantID, input.ID)
	if err == nil {
		if existing.digest != digest || existing.MakerID != actor.ActorID {
			problem(w, http.StatusConflict, errBookingChangeConflict.Error())
			return
		}
		respond(w, http.StatusOK, existing)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		bookingRiskUnavailable(w, err)
		return
	}
	if !input.ExpiresAt.After(time.Now()) {
		problem(w, http.StatusUnprocessableEntity, "Proposal expiry must be in the future.")
		return
	}

	var proof json.RawMessage
	if input.ShadowWindow != nil {
		report, err := h.repo.bookingShadow(r.Context(), actor.MerchantID, *input.ShadowWindow)
		if errors.Is(err, errBookingShadowWindowTooLarge) {
			problem(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if err != nil {
			bookingRiskUnavailable(w, err)
			return
		}
		if m := report.Shadow; m.TP+m.TN+m.FP+m.FN == 0 {
			problem(w, http.StatusUnprocessableEntity, "The shadow window must contain at least one scorable booking.")
			return
		}
		proof, err = json.Marshal(report)
		if err != nil {
			bookingRiskUnavailable(w, err)
			return
		}
	}

	result, created, err := h.repo.saveBookingChange(r.Context(), actor, input, digest, proof)
	switch {
	case errors.Is(err, errBookingChangeConflict):
		problem(w, http.StatusConflict, err.Error())
	case errors.Is(err, errBookingChangeExpired):
		problem(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		bookingRiskUnavailable(w, err)
	case created:
		respond(w, http.StatusCreated, result)
	default:
		respond(w, http.StatusOK, result)
	}
}

func (h bookingGovernanceHandler) read(w http.ResponseWriter, r *http.Request) {
	actor := h.auth.requireBookingActor(w, r, bookingRoleMerchant, bookingRoleAnalyst, bookingRoleViewer)
	if actor.MerchantID == "" {
		return
	}
	id := bookingChangeID(w, r)
	if id == "" {
		return
	}
	result, err := lookupBookingChange(r.Context(), h.repo.db, actor.MerchantID, id)
	h.writeResult(w, result, err)
}

func (h bookingGovernanceHandler) list(w http.ResponseWriter, r *http.Request) {
	actor := h.auth.requireBookingActor(w, r, bookingRoleMerchant, bookingRoleAnalyst, bookingRoleViewer)
	if actor.MerchantID == "" {
		return
	}
	rows, err := h.repo.db.Query(r.Context(), readBookingChangesSQL+
		" WHERE p.merchant_id = $1 ORDER BY p.created_at DESC, p.id LIMIT 100", actor.MerchantID)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	defer rows.Close()
	result := []bookingChange{}
	for rows.Next() {
		change, err := scanBookingChange(rows)
		if err != nil {
			bookingRiskUnavailable(w, err)
			return
		}
		result = append(result, change)
	}
	if err := rows.Err(); err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, result)
}

func (h bookingGovernanceHandler) decide(w http.ResponseWriter, r *http.Request) {
	actor := h.auth.requireBookingActor(w, r, bookingRoleAnalyst)
	if actor.MerchantID == "" {
		return
	}
	id := bookingChangeID(w, r)
	if id == "" {
		return
	}
	var input struct {
		Decision string `json:"decision"`
	}
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if input.Decision != "approve" && input.Decision != "reject" {
		problem(w, http.StatusUnprocessableEntity, "Choose approve or reject.")
		return
	}
	result, err := h.repo.decideBookingChange(r.Context(), actor, id, input.Decision)
	h.writeResult(w, result, err)
}

func (h bookingGovernanceHandler) writeResult(w http.ResponseWriter, result bookingChange, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Change proposal not found.")
	case errors.Is(err, errBookingChangeSelf):
		problem(w, http.StatusForbidden, err.Error())
	case errors.Is(err, errBookingChangeConflict), errors.Is(err, errBookingChangeExpired):
		problem(w, http.StatusConflict, err.Error())
	case err != nil:
		bookingRiskUnavailable(w, err)
	default:
		respond(w, http.StatusOK, result)
	}
}

func bookingChangeID(w http.ResponseWriter, r *http.Request) string {
	id := r.PathValue("id")
	if !validID.MatchString(id) {
		problem(w, http.StatusBadRequest, "Expected a lowercase proposal UUID.")
		return ""
	}
	return id
}
