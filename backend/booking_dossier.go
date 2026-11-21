package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

const bookingDossierSchema = `
CREATE TABLE IF NOT EXISTS booking_dossiers (
  merchant_id text NOT NULL,
  event_id uuid NOT NULL,
  requested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  result jsonb,
  PRIMARY KEY (merchant_id, event_id),
  FOREIGN KEY (merchant_id, event_id) REFERENCES booking_assessments (merchant_id, event_id)
);
CREATE INDEX IF NOT EXISTS booking_dossier_pending_idx ON booking_dossiers (requested_at) WHERE result IS NULL;
`

type dossierCitation struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Version string `json:"version"`
}

type bookingDossier struct {
	EventID   string            `json:"eventId"`
	Citations []dossierCitation `json:"citations"`
}

type dossierStatus struct {
	Status  string          `json:"status"`
	Dossier *bookingDossier `json:"dossier,omitempty"`
}

// Build from the decision's typed fields. Free-form strings, contact details,
// outcome provenance, and browser instructions never enter the dossier.
func buildBookingDossier(eventID string, snapshot bookingAssessment) bookingDossier {
	result := bookingDossier{EventID: eventID, Citations: []dossierCitation{}}
	if slices.Contains([]string{"LOW_RISK", "REVIEW", "UNRESOLVED"}, snapshot.Outcome) {
		result.Citations = append(result.Citations, dossierCitation{"outcome", snapshot.Outcome, snapshot.RuleVersion})
	}
	if snapshot.Handoff == "MANUAL_REVIEW" {
		result.Citations = append(result.Citations, dossierCitation{"handoff", snapshot.Handoff, snapshot.RuleVersion})
	}
	if slices.Contains(snapshot.RuleHits, "BOOKING_VELOCITY_60S") {
		result.Citations = append(result.Citations, dossierCitation{"ruleHits", "BOOKING_VELOCITY_60S", snapshot.RuleVersion})
	}
	if snapshot.ListVersion != "" && slices.Contains(snapshot.RuleHits, bookingProductListRuleCode) {
		result.Citations = append(result.Citations, dossierCitation{"ruleHits", bookingProductListRuleCode, snapshot.ListVersion})
	}
	if snapshot.Jev.Status == bookingJevStatusMock || snapshot.Jev.Status == bookingJevStatusTimeout {
		result.Citations = append(result.Citations, dossierCitation{"jev.status", string(snapshot.Jev.Status), snapshot.Jev.ModelVersion})
	}
	if snapshot.Baseline.State == "ACTIVE" && snapshot.Baseline.Version != nil {
		result.Citations = append(result.Citations, dossierCitation{"baseline.state", "ACTIVE", *snapshot.Baseline.Version})
	} else if snapshot.Baseline.State == "COLD_START" {
		result.Citations = append(result.Citations, dossierCitation{"baseline.state", "COLD_START", snapshot.ProfileVersion})
	}
	return result
}

func registerBookingDossiers(mux *http.ServeMux, repo repository, auth bookingAuth) {
	handler := bookingOpsHandler{repo: repo, auth: auth}
	path := bookingOpsPath + "/{id}/dossier"
	mux.HandleFunc("POST "+path, handler.bookingDossier)
	mux.HandleFunc("GET "+path, handler.bookingDossier)
}

func (h bookingOpsHandler) bookingDossier(w http.ResponseWriter, r *http.Request) {
	actor := h.readActor(w, r)
	if actor.MerchantID == "" {
		return
	}
	eventID := bookingOpsEventID(w, r)
	if eventID == "" {
		return
	}
	ctx := r.Context()
	if r.Method == http.MethodPost {
		var exists, assessed bool
		err := h.repo.db.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM booking_events WHERE merchant_id=$1 AND event_id=$2::text::uuid),
			       EXISTS (SELECT 1 FROM booking_assessments WHERE merchant_id=$1 AND event_id=$2::text::uuid)`,
			actor.MerchantID, eventID).Scan(&exists, &assessed)
		if err != nil {
			bookingRiskUnavailable(w, err)
			return
		}
		if !exists {
			problem(w, http.StatusNotFound, "Booking event not found.")
			return
		}
		if !assessed {
			problem(w, http.StatusConflict, "An assessment is required before generating a dossier.")
			return
		}
		_, err = h.repo.db.Exec(ctx, `
			INSERT INTO booking_dossiers(merchant_id,event_id) VALUES ($1,$2::text::uuid) ON CONFLICT DO NOTHING`, actor.MerchantID, eventID)
		if err != nil {
			bookingRiskUnavailable(w, err)
			return
		}
	}
	var dossier *bookingDossier
	err := h.repo.db.QueryRow(ctx, `
		SELECT result FROM booking_dossiers WHERE merchant_id=$1 AND event_id=$2::text::uuid`, actor.MerchantID, eventID).Scan(&dossier)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Dossier has not been requested.")
	case err != nil:
		bookingRiskUnavailable(w, err)
	case dossier != nil:
		respond(w, http.StatusOK, dossierStatus{"READY", dossier})
	default:
		respond(w, http.StatusAccepted, dossierStatus{Status: "PENDING"})
	}
}

func (r repository) runBookingDossiers(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := r.completeBookingDossier(ctx)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				log.Printf("booking dossier worker: %v", err)
			}
		}
	}
}

func (r repository) completeBookingDossier(ctx context.Context) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var merchant, eventID string
		var snapshot bookingAssessment
		err := tx.QueryRow(ctx, `
			SELECT j.merchant_id, j.event_id::text, a.result
			FROM booking_dossiers j JOIN booking_assessments a USING (merchant_id,event_id)
			WHERE j.result IS NULL ORDER BY j.requested_at,j.merchant_id,j.event_id
			LIMIT 1 FOR UPDATE OF j SKIP LOCKED`).Scan(&merchant, &eventID, &snapshot)
		if err != nil {
			return err
		}
		data, err := json.Marshal(buildBookingDossier(eventID, snapshot))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE booking_dossiers SET result=$3::jsonb WHERE merchant_id=$1 AND event_id=$2::text::uuid`, merchant, eventID, data)
		return err
	})
}
