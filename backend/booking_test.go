package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testBookingBody = `{"eventId":"498ab33a-c10d-45f9-8db9-7715f2e0f8b4","schemaVersion":1,"stage":"booking","occurredAt":"2026-11-21T09:00:00Z"}`

type bookingTestStore func(context.Context, string, string, bookingInput) (bookingEvent, bool, error)

func (store bookingTestStore) saveBooking(ctx context.Context, merchant, key string, input bookingInput) (bookingEvent, bool, error) {
	return store(ctx, merchant, key, input)
}

func TestBookingEventHandler(t *testing.T) {
	auth, err := newBookingAuth("test-a", "test-b")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, key, body string
		wantStatus             int
		wantMerchant           string
		wantZero               bool
	}{
		{"merchant A with missing amount", "test-a", "request-1", testBookingBody, http.StatusCreated, "demo-merchant-a", false},
		{"merchant B with zero amount", "test-b", "request-1", strings.TrimSuffix(testBookingBody, "}") + `,"amountMinor":0}`, http.StatusCreated, "demo-merchant-b", true},
		{"wrong token", "other", "request-1", testBookingBody, http.StatusUnauthorized, "", false},
		{"missing key", "test-a", "", testBookingBody, http.StatusBadRequest, "", false},
		{"invalid stage", "test-a", "request-1", strings.Replace(testBookingBody, `"booking"`, `"payment"`, 1), http.StatusBadRequest, "", false},
		{"mock checkout metadata", "test-a", "request-1", strings.TrimSuffix(testBookingBody, "}") + `,"product":"hotel_room","sdkStatus":"unavailable"}`, http.StatusCreated, "demo-merchant-a", false},
		{"invalid product", "test-a", "request-1", strings.TrimSuffix(testBookingBody, "}") + `,"product":"unknown"}`, http.StatusBadRequest, "", false},
		{"invalid SDK status", "test-a", "request-1", strings.TrimSuffix(testBookingBody, "}") + `,"sdkStatus":"observedish"}`, http.StatusBadRequest, "", false},
		{"client merchant is rejected", "test-a", "request-1", strings.TrimSuffix(testBookingBody, "}") + `,"merchantId":"demo-merchant-b"}`, http.StatusBadRequest, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := bookingTestStore(func(_ context.Context, merchant, key string, input bookingInput) (bookingEvent, bool, error) {
				if merchant != tc.wantMerchant || key != tc.key {
					t.Fatalf("store received merchant %q and key %q", merchant, key)
				}
				if tc.wantZero && (input.AmountMinor == nil || *input.AmountMinor != 0) {
					t.Fatalf("expected explicit zero amount, got %v", input.AmountMinor)
				}
				if !tc.wantZero && input.AmountMinor != nil {
					t.Fatal("expected amount to remain missing")
				}
				return bookingEvent{bookingInput: input, MerchantID: merchant}, true, nil
			})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/booking/events", strings.NewReader(tc.body))
			request.Header.Set("Authorization", "Bearer "+tc.token)
			if tc.key != "" {
				request.Header.Set("Idempotency-Key", tc.key)
			}
			recorder := httptest.NewRecorder()
			bookingHandler{store, auth}.create(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if tc.wantStatus == http.StatusCreated {
				var result map[string]any
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				amount, present := result["amountMinor"]
				if result["merchantId"] != tc.wantMerchant || result["stage"] != "booking" || present != tc.wantZero || (present && amount != float64(0)) {
					t.Fatalf("unexpected booking JSON: %v", result)
				}
				if tc.name == "mock checkout metadata" {
					if result["product"] != "hotel_room" || result["sdkStatus"] != "unavailable" {
						t.Fatalf("checkout metadata did not round-trip: %v", result)
					}
				} else if result["product"] != nil || result["sdkStatus"] != nil {
					t.Fatalf("legacy booking unexpectedly contains checkout metadata: %v", result)
				}
			}
		})
	}
}
