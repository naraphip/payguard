package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type merchantJevTransport func(*http.Request) (*http.Response, error)

func (f merchantJevTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testMerchantJev(t *testing.T, transport merchantJevTransport) merchantJev {
	t.Helper()
	jev, err := newMerchantJev("live", "test-key", "")
	if err != nil {
		t.Fatal(err)
	}
	jev.client.Transport = transport
	return jev
}

func testJevResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}
}

func TestMerchantJevSendsOnlyRuleFactsAndPreservesLocalReview(t *testing.T) {
	amount := int64(50000)
	input := mockTransactionInput{Source: "checkout", Product: "flight_ticket", AmountMinor: &amount, ID: "transaction", ScenarioID: "scenario"}
	scenario := mockScenario{ID: "scenario", Name: "internal scenario", ThresholdMinor: amount}
	jev := testMerchantJev(t, func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		if r.Method != http.MethodPost || r.URL.String() != openRouterDecisionsURL ||
			r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect request method, URL, or headers")
		}
		var body struct {
			Model     string         `json:"model"`
			State     map[string]any `json:"state"`
			Questions map[string]struct {
				Type         string            `json:"type"`
				Instructions string            `json:"instructions"`
				Criteria     map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			t.Fatal(err)
		}
		wantState := map[string]any{"booking_amount_minor": float64(amount),
			"review_threshold_minor": float64(amount), "configured_review_rule_matched": true}
		question := body.Questions[merchantJevQuestion]
		if body.Model != defaultMerchantJevModel || !reflect.DeepEqual(body.State, wantState) ||
			len(body.Questions) != 1 || question.Type != "choice" || question.Instructions == "" || len(question.Criteria) != 2 ||
			question.Criteria["LOW_RISK"] == "" || question.Criteria["REVIEW"] == "" {
			t.Error("typed request did not contain the expected rule facts and choices")
		}
		return testJevResponse(200, `{"id":"gen-dec-test","model":"typesafe/jev-1.14-20261015","answers":{"booking_risk":{"type":"choice","choice":"LOW_RISK","confidence":0.8}}}`), nil
	})
	result := evaluateMerchantJev(context.Background(), input, scenario, jevRulesSnapshot{Version: "rules-v2"}, jev)
	if result.JevStatus != "LIVE" || result.JevVersion != "typesafe/jev-1.14-20261015" ||
		result.JevAnswer != "LOW_RISK" || result.JevDecisionID != "gen-dec-test" ||
		result.JevConfidence == nil || *result.JevConfidence != 0.8 ||
		result.Outcome != "REVIEW" || result.ReviewState != "NEEDS_DECISION" || result.JevRuleVersion != "rules-v2" {
		t.Fatalf("incorrect result: %+v", result)
	}
}

func TestMerchantJevDecisionAndFailures(t *testing.T) {
	amount := int64(0)
	input := mockTransactionInput{AmountMinor: &amount}
	scenario := mockScenario{ThresholdMinor: 50000}
	reviewAnswer := `{"model":"typesafe/jev-1.14","answers":{"booking_risk":{"type":"choice","choice":"REVIEW"}}}`
	tests := []struct {
		name       string
		statusCode int
		body       string
		status     string
		outcome    string
	}{
		{"review", 200, reviewAnswer, "LIVE", "REVIEW"},
		{"malformed", 200, "{", "UNAVAILABLE", "UNRESOLVED"},
		{"refusal", 200, `{"model":"typesafe/jev-1.14","answers":{"booking_risk":{"type":"refusal"}}}`, "UNAVAILABLE", "UNRESOLVED"},
		{"wrong model", 200, strings.ReplaceAll(reviewAnswer, "jev-1.14", "jev-1.13"), "UNAVAILABLE", "UNRESOLVED"},
		{"unknown choice", 200, strings.ReplaceAll(reviewAnswer, "REVIEW", "ALLOW"), "UNAVAILABLE", "UNRESOLVED"},
		{"too large", 200, strings.Repeat(" ", merchantJevMaxResponse) + reviewAnswer, "UNAVAILABLE", "UNRESOLVED"},
		{"invalid key", 401, "provider error", "UNAVAILABLE", "UNRESOLVED"},
		{"limited", 429, "provider error", "UNAVAILABLE", "UNRESOLVED"},
		{"upstream timeout", 524, "provider error", "TIMEOUT", "UNRESOLVED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jev := testMerchantJev(t, func(*http.Request) (*http.Response, error) {
				return testJevResponse(test.statusCode, test.body), nil
			})
			got := evaluateMerchantJev(context.Background(), input, scenario, jevRulesSnapshot{}, jev)
			if got.JevStatus != test.status || got.Outcome != test.outcome || got.ReviewState != "NEEDS_DECISION" {
				t.Fatalf("incorrect result: %+v", got)
			}
			if got.JevStatus != "LIVE" && (got.JevAnswer != "" || strings.Contains(got.Reason, "provider error")) {
				t.Fatalf("unvalidated provider evidence: %+v", got)
			}
		})
	}
}

func TestMerchantJevSkippedCallsAndRealTimeoutReplay(t *testing.T) {
	amount := int64(0)
	input := mockTransactionInput{AmountMinor: &amount}
	scenario := mockScenario{ThresholdMinor: 50000}
	calls := 0
	jev := testMerchantJev(t, func(r *http.Request) (*http.Response, error) {
		calls++
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	jev.client.Timeout = 20 * time.Millisecond
	timedOut := evaluateMerchantJev(context.Background(), input, scenario, jevRulesSnapshot{}, jev)
	if timedOut.JevStatus != "TIMEOUT" || timedOut.JevVersion != defaultMerchantJevModel ||
		timedOut.Outcome != "UNRESOLVED" || !timedOut.matchesRequest(input) {
		t.Fatalf("real timeout was not replayable: %+v", timedOut)
	}

	input.SimulateTimeout = true
	simulated := evaluateMerchantJev(context.Background(), input, scenario, jevRulesSnapshot{}, jev)
	if simulated.JevStatus != "TIMEOUT" || simulated.JevVersion != "mock-jev-v1" ||
		!simulated.matchesRequest(input) || timedOut.matchesRequest(input) {
		t.Fatalf("simulated timeout was not distinguished: %+v", simulated)
	}

	missing := evaluateMerchantJev(context.Background(), mockTransactionInput{}, scenario, jevRulesSnapshot{}, jev)
	if missing.JevStatus != "NOT_RUN" || missing.Outcome != "UNRESOLVED" || calls != 1 {
		t.Fatalf("missing amount or simulated timeout reached provider: %+v", missing)
	}
}

func TestMerchantJevModeAndMissingKey(t *testing.T) {
	amount := int64(0)
	input := mockTransactionInput{AmountMinor: &amount}
	scenario := mockScenario{ThresholdMinor: 50000}
	mock, err := newMerchantJev("", "present-key", "")
	if err != nil {
		t.Fatal(err)
	}
	got := evaluateMerchantJev(context.Background(), input, scenario, jevRulesSnapshot{}, mock)
	if got.JevStatus != "MOCKED" || got.Outcome != "LOW_RISK" {
		t.Fatalf("default mock mode: %+v", got)
	}
	live, err := newMerchantJev("live", "", "")
	if err != nil {
		t.Fatal(err)
	}
	got = evaluateMerchantJev(context.Background(), input, scenario, jevRulesSnapshot{}, live)
	if got.JevStatus != "UNAVAILABLE" || got.Outcome != "UNRESOLVED" {
		t.Fatalf("live mode without key must require review: %+v", got)
	}
	if _, err := newMerchantJev("live", "", "other/model"); err == nil {
		t.Fatal("accepted a non-Jev model")
	}
}
