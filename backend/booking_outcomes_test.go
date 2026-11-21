package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBookingOutcomeImportValidation(t *testing.T) {
	auth, err := newBookingAuth("test-a", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := bookingRiskHandler{auth: auth}
	eventID := "498ab33a-c10d-45f9-8db9-7715f2e0f8b4"
	valid := `{"outcomeId":"446ac573-baa7-4cf7-86d1-f477cf5e7b4e","source":"verified_demo","sourceReference":"sample-1","label":"normal","occurredAt":"2026-11-21T09:00:00.123456Z"}`
	for _, tc := range []struct {
		name, token, id, body string
		status                int
	}{
		{"authentication", "unknown", eventID, valid, http.StatusUnauthorized},
		{"event IDs", "test-a", "invalid", valid, http.StatusBadRequest},
		{"unknown source", "test-a", eventID, strings.Replace(valid, "verified_demo", "external", 1), http.StatusUnprocessableEntity},
		{"unknown label", "test-a", eventID, strings.Replace(valid, "normal", "approved", 1), http.StatusUnprocessableEntity},
		{"invalid reference", "test-a", eventID, strings.Replace(valid, "sample-1", "invalid reference", 1), http.StatusUnprocessableEntity},
		{"submicrosecond timestamp", "test-a", eventID, strings.Replace(valid, "123456Z", "1234567Z", 1), http.StatusUnprocessableEntity},
		{"eligibility cannot be asserted by the client", "test-a", eventID, strings.TrimSuffix(valid, "}") + `,"eligible":true}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/booking/events/"+tc.id+"/outcomes", strings.NewReader(tc.body))
			req.SetPathValue("id", tc.id)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			res := httptest.NewRecorder()
			handler.importOutcome(res, req)
			if res.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", res.Code, tc.status, res.Body)
			}
		})
	}
}

func TestBaselineSummaryIncludesSampleCount(t *testing.T) {
	id := int64(2)
	tests := []struct {
		id       *int64
		count    int
		expected string
	}{
		{nil, 0, `{"state":"COLD_START","version":null,"sampleCount":0}`},
		{&id, 4, `{"state":"ACTIVE","version":"baseline-v1-2","sampleCount":4}`},
	}
	for _, tc := range tests {
		raw, err := json.Marshal(baselineSummary(tc.id, tc.count))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != tc.expected {
			t.Fatalf("baseline summary = %s, want %s", raw, tc.expected)
		}
	}
}
