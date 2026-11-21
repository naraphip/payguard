package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const (
	bookingStage     = "booking"
	bookingTimestamp = "occurredAt"
	bookingAmount    = "amountMinor"
	profileSchema    = 1
)

type merchantSetup struct {
	SchemaVersion    int      `json:"schemaVersion"`
	BusinessModel    string   `json:"businessModel"`
	Stage            string   `json:"stage"`
	AvailableSignals []string `json:"availableSignals"`
}

type profileCandidate struct {
	SchemaVersion int      `json:"schemaVersion"`
	Stage         string   `json:"stage"`
	Signals       []string `json:"signals"`
}

type bookingProfile struct {
	Setup     merchantSetup    `json:"setup"`
	Candidate profileCandidate `json:"candidate"`
}

func canonicalBookingSignals(signals []string) ([]string, error) {
	if !slices.Contains(signals, bookingTimestamp) {
		return nil, errors.New("the booking timestamp signal is required")
	}
	seen := make(map[string]bool)
	for _, signal := range signals {
		if signal != bookingTimestamp && signal != bookingAmount {
			return nil, errors.New("unknown or unavailable signal for the booking stage")
		}
		if seen[signal] {
			return nil, errors.New("booking signals must be distinct")
		}
		seen[signal] = true
	}
	canonical := []string{bookingTimestamp}
	if seen[bookingAmount] {
		canonical = append(canonical, bookingAmount)
	}
	return canonical, nil
}

func validateBookingSetup(setup *merchantSetup) error {
	if setup.SchemaVersion != profileSchema || (setup.BusinessModel != "ota" && setup.BusinessModel != "ecommerce") {
		return errors.New("expected merchant setup schemaVersion 1 and an OTA or e-commerce business model")
	}
	if setup.Stage != bookingStage {
		return errors.New("merchant setup must use the booking stage")
	}
	signals, err := canonicalBookingSignals(setup.AvailableSignals)
	if err != nil {
		return err
	}
	setup.AvailableSignals = signals
	return nil
}

func validateBookingProfile(profile *bookingProfile) error {
	if err := validateBookingSetup(&profile.Setup); err != nil {
		return err
	}
	if profile.Candidate.SchemaVersion != profileSchema || profile.Candidate.Stage != profile.Setup.Stage {
		return errors.New("profile schemaVersion and stage must match the merchant setup")
	}
	signals, err := canonicalBookingSignals(profile.Candidate.Signals)
	if err != nil {
		return err
	}
	for _, signal := range signals {
		if !slices.Contains(profile.Setup.AvailableSignals, signal) {
			return errors.New("profile includes a signal missing from merchant setup")
		}
	}
	profile.Candidate.Signals = signals
	return nil
}

func proposeBookingProfile(setup merchantSetup) (bookingProfile, error) {
	proposal := bookingProfile{setup, profileCandidate{profileSchema, bookingStage, setup.AvailableSignals}}
	err := validateBookingProfile(&proposal)
	return proposal, err
}

type bookingRiskHandler struct {
	repo               repository
	auth               bookingAuth
	redis              *redis.Client
	baselineMinSamples int
}

func decodeBookingRiskJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		problem(w, http.StatusBadRequest, "Expected one valid JSON request object.")
		return false
	}
	return true
}

func bookingRiskUnavailable(w http.ResponseWriter, err error) {
	log.Printf("booking risk dependency error: %v", err)
	problem(w, http.StatusServiceUnavailable, "Booking risk storage is temporarily unavailable. Please retry.")
}

func (h bookingRiskHandler) proposeProfile(w http.ResponseWriter, r *http.Request) {
	if h.auth.requireMerchant(w, r) == "" {
		return
	}
	var setup merchantSetup
	if !decodeBookingRiskJSON(w, r, &setup) {
		return
	}
	proposal, err := proposeBookingProfile(setup)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	respond(w, http.StatusOK, proposal)
}

func (h bookingRiskHandler) approveProfile(w http.ResponseWriter, r *http.Request) {
	merchant := h.auth.requireMerchant(w, r)
	if merchant == "" {
		return
	}
	var profile bookingProfile
	if !decodeBookingRiskJSON(w, r, &profile) {
		return
	}
	if err := validateBookingProfile(&profile); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	version, err := h.repo.activateBookingProfile(r.Context(), merchant, profile)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]string{"profileVersion": version})
}

func (r repository) activateBookingProfile(ctx context.Context, merchant string, profile bookingProfile) (string, error) {
	raw, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	version := "profile-v1-" + hex.EncodeToString(sum[:])
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO booking_profiles (merchant_id, version, approved_profile)
			VALUES ($1, $2, $3::jsonb) ON CONFLICT DO NOTHING`, merchant, version, raw)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO booking_active_profiles (merchant_id, version) VALUES ($1, $2)
			ON CONFLICT (merchant_id) DO UPDATE SET version = EXCLUDED.version`, merchant, version)
		return err
	})
	return version, err
}
