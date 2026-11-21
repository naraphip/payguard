package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBookingProposalCanonicalSignals(t *testing.T) {
	setup := merchantSetup{profileSchema, "ota", bookingStage, []string{bookingTimestamp, bookingAmount}}
	first, err := proposeBookingProfile(setup)
	if err != nil {
		t.Fatal(err)
	}
	setup.AvailableSignals = []string{bookingAmount, bookingTimestamp}
	second, err := proposeBookingProfile(setup)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !slices.Equal(second.Candidate.Signals, []string{bookingTimestamp, bookingAmount}) {
		t.Fatalf("expected canonical deterministic proposal, got %+v and %+v", first, second)
	}
}

func TestBookingProfileSignalValidation(t *testing.T) {
	setup := merchantSetup{profileSchema, "ecommerce", bookingStage, []string{bookingTimestamp}}
	for _, tc := range []struct {
		name    string
		stage   string
		signals []string
	}{
		{"future stage", "payment", []string{bookingTimestamp}},
		{"future signal", bookingStage, []string{bookingTimestamp, "paymentToken"}},
		{"unknown signal", bookingStage, []string{bookingTimestamp, "unknown"}},
		{"unavailable signal", bookingStage, []string{bookingTimestamp, bookingAmount}},
		{"duplicate signal", bookingStage, []string{bookingTimestamp, bookingTimestamp}},
		{"missing timestamp", bookingStage, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := bookingProfile{setup, profileCandidate{profileSchema, tc.stage, tc.signals}}
			if err := validateBookingProfile(&profile); err == nil {
				t.Fatalf("expected invalid profile: %+v", profile)
			}
		})
	}
}

func TestBookingProfileProposalRequiresMerchant(t *testing.T) {
	auth, err := newBookingAuth("test-a", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := bookingRiskHandler{auth: auth}
	body := `{"schemaVersion":1,"businessModel":"ota","stage":"booking","availableSignals":["occurredAt"]}`
	for _, tc := range []struct {
		token, body string
		status      int
	}{
		{"bad-token", body, http.StatusUnauthorized},
		{"test-a", strings.Replace(body, "booking", "payment", 1), http.StatusUnprocessableEntity},
		{"test-a", body + body, http.StatusBadRequest},
		{"test-a", body, http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/booking/profiles/proposal", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		res := httptest.NewRecorder()
		handler.proposeProfile(res, req)
		if res.Code != tc.status {
			t.Fatalf("status %d, want %d: %s", res.Code, tc.status, res.Body)
		}
	}
}

func TestBookingAssessmentEvidence(t *testing.T) {
	booking := bookingEvent{bookingInput: bookingInput{EventID: "event-id", OccurredAt: time.Unix(100, 0)}}
	profile := profileCandidate{profileSchema, bookingStage, []string{bookingTimestamp, bookingAmount}}
	missing := evaluateBooking(booking, "profile-a", profile, 1, bookingJevMock)
	if missing.Features[2].Presence != "MISSING" || missing.Features[2].Value != nil || missing.Jev.Answer != bookingJevLow {
		t.Fatalf("expected explicit missing evidence and a low mock answer: %+v", missing)
	}
	booking.AmountMinor = new(int64)
	zero := evaluateBooking(booking, "profile-a", profile, bookingVelocityReviewThreshold, bookingJevMock)
	if zero.Features[2].Presence != "OBSERVED" || zero.Features[2].Value != int64(0) ||
		zero.Outcome != "REVIEW" || zero.Jev.Answer != bookingJevElevated ||
		!slices.Equal(zero.RuleHits, []string{"BOOKING_VELOCITY_60S"}) {
		t.Fatalf("expected observed zero evidence and velocity-based review: %+v", zero)
	}

	profile.Signals = []string{bookingTimestamp}
	timeout := evaluateBooking(booking, "profile-b", profile, bookingVelocityReviewThreshold, bookingJevTimeout)
	if len(timeout.Features) != 2 || timeout.ProfileVersion != "profile-b" || timeout.Baseline.State != "COLD_START" ||
		timeout.Baseline.Version != nil || timeout.Jev.Status != bookingJevStatusTimeout || timeout.Jev.Answer != bookingJevUnavailable ||
		timeout.Jev.ModelVersion == "" || timeout.Jev.RubricVersion == "" ||
		timeout.Outcome != "UNRESOLVED" || timeout.Handoff != "MANUAL_REVIEW" {
		t.Fatalf("expected pinned evidence with amount excluded and timeout handoff: %+v", timeout)
	}
	raw, err := json.Marshal(timeout)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "paymentAction") || !strings.Contains(string(raw), `"version":null`) {
		t.Fatalf("expected cold-start baseline with no payment action: %s", raw)
	}
}
