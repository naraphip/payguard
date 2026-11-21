package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestReadMockMerchantInput(t *testing.T) {
	for _, test := range []struct {
		name  string
		json  string
		valid bool
		setup *merchantSetup
	}{
		{
			name:  "legacy name only",
			json:  `{"name":"  Demo  "}`,
			valid: true,
		},
		{
			name: "setup signal order canonicalized",
			json: `{"name":"Demo","setup":{"schemaVersion":1,"businessModel":"ota","stage":"booking",
				"availableSignals":["amountMinor","occurredAt"]}}`,
			valid: true,
			setup: &merchantSetup{SchemaVersion: 1, BusinessModel: "ota", Stage: "booking",
				AvailableSignals: []string{"occurredAt", "amountMinor"}},
		},
		{
			name: "booking timestamp required",
			json: `{"name":"Demo","setup":{"schemaVersion":1,"businessModel":"ecommerce","stage":"booking",
				"availableSignals":["amountMinor"]}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, merchantDemoPath, strings.NewReader(test.json))
			input, valid := readMockMerchantInput(w, r)
			if valid != test.valid {
				t.Fatalf("valid=%t, want %t: %s", valid, test.valid, w.Body.String())
			}
			if valid && (input.Name != "Demo" || !reflect.DeepEqual(input.Setup, test.setup)) {
				t.Fatalf("unexpected decoded input: %#v", input)
			}
			if !valid && w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("unexpected failure status: %d", w.Code)
			}
		})
	}
}

func TestMockJevOutcomesAndScenarioPinning(t *testing.T) {
	zero, amount := int64(0), int64(50000)
	everyday := mockScenario{ID: "everyday", Name: "Everyday bookings", ThresholdMinor: 100000, ScenarioVersion: 3, RuleVersion: "rules-v3"}
	highValue := mockScenario{ID: "high-value", Name: "High-value bookings", ThresholdMinor: 50000, ScenarioVersion: 2, RuleVersion: "rules-v2"}
	for _, test := range []struct {
		name      string
		scenario  mockScenario
		amount    *int64
		timeout   bool
		outcome   string
		review    string
		ruleHits  []string
		jevStatus string
	}{
		{"everyday", everyday, &amount, false, "LOW_RISK", "NONE", []string{}, "MOCKED"},
		{"inclusive threshold", highValue, &amount, false, "REVIEW", "NEEDS_DECISION", []string{"AMOUNT_REVIEW_THRESHOLD"}, "MOCKED"},
		{"timeout", highValue, &amount, true, "UNRESOLVED", "NEEDS_DECISION", []string{}, "TIMEOUT"},
		{"missing", highValue, nil, false, "UNRESOLVED", "NEEDS_DECISION", []string{}, "MOCKED"},
		{"zero", highValue, &zero, false, "LOW_RISK", "NONE", []string{}, "MOCKED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := evaluateMockJev(mockTransactionInput{ID: "event", Source: "checkout", Product: "hotel_room",
				AmountMinor: test.amount, SimulateTimeout: test.timeout}, test.scenario)
			if result.Outcome != test.outcome || result.ReviewState != test.review || result.JevStatus != test.jevStatus ||
				!reflect.DeepEqual(result.RuleHits, test.ruleHits) {
				t.Fatalf("unexpected mock decision: %#v", result)
			}
			if result.ScenarioID != test.scenario.ID || result.ScenarioVersion != test.scenario.ScenarioVersion ||
				result.RuleVersion != test.scenario.RuleVersion || !sameBookingAmount(result.ThresholdMinor, &test.scenario.ThresholdMinor) ||
				!sameBookingAmount(result.AmountMinor, test.amount) || result.Product != "hotel_room" {
				t.Fatalf("missing pinned evidence: %#v", result)
			}
		})
	}
}

func TestMockMerchantAdminRequiresConfiguredCredential(t *testing.T) {
	for _, test := range []struct {
		adminToken string
		header     string
		wantCode   int
	}{
		{"", "", http.StatusUnauthorized},
		{"preview-admin", "", http.StatusUnauthorized},
		{"preview-admin", "Bearer preview-admin", http.StatusOK},
	} {
		mux := http.NewServeMux()
		registerMerchantDemo(mux, repository{}, test.adminToken, merchantJev{})
		// The body is intentionally malformed so successful authentication is
		// observable without invoking the repository.
		request := httptest.NewRequest(http.MethodPost, merchantDemoPath, http.NoBody)
		if test.header != "" {
			request.Header.Set("Authorization", test.header)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, request)
		expected := test.wantCode
		if expected == http.StatusOK {
			expected = http.StatusBadRequest
		}
		if w.Code != expected {
			t.Fatalf("status = %d, want %d: %s", w.Code, expected, w.Body.String())
		}
	}
}
