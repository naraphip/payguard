package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	omiseTestRunID = "498ab33a-c10d-45f9-8db9-7715f2e0f8b4"
	omiseTestBody  = `{"runId":"` + omiseTestRunID + `","token":"tokn_test_fixture"}`
)

type omiseTestTransport func(*http.Request) (*http.Response, error)

func (f omiseTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func omiseProviderFixture(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestOmiseConfigIsTestOnly(t *testing.T) {
	for _, input := range []struct {
		public, secret string
		enabled        bool
	}{
		{"pkey_test_fixture", "skey_test_fixture", true},
		{"pkey_live_fixture", "skey_test_fixture", false},
		{"pkey_test_fixture", "skey_live_fixture", false},
		{"pkey_test_fixture", "", false},
		{"pkey_test_fixture-extra", "skey_test_fixture", false},
	} {
		checkout := newOmiseCheckout(nil, input.public, input.secret)
		w := httptest.NewRecorder()
		checkout.config(w, httptest.NewRequest(http.MethodGet, "/api/v1/simulation/checkout/config", nil))
		var got struct {
			Enabled   bool
			PublicKey string
			Currency  string
			Prices    map[string]int64
		}
		err := json.Unmarshal(w.Body.Bytes(), &got)
		if err != nil || w.Code != 200 || got.Enabled != input.enabled || w.Header().Get("Cache-Control") != "no-store" ||
			got.Currency != "THB" || len(got.Prices) != 4 || got.Prices["flight_ticket"] != 459000 || got.Prices["hotel_room"] != 325000 ||
			got.Prices["physical"] != 249000 || got.Prices["digital"] != 75000 ||
			(input.enabled && got.PublicKey != input.public) || (!input.enabled && got.PublicKey != "") ||
			strings.Contains(w.Body.String(), "skey_") {
			t.Fatalf("unexpected checkout configuration: status %d, decode %v", w.Code, err)
		}
	}
}

func TestOmiseChargeProviderRequest(t *testing.T) {
	calls := 0
	client := omiseClient{secretKey: "skey_test_fixture", http: &http.Client{Transport: omiseTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		key, password, authenticated := req.BasicAuth()
		if req.Method != "POST" || req.URL.String() != "https://api.omise.co/charges" ||
			req.Header.Get("Omise-Version") != "2019-05-29" || req.GetBody != nil ||
			req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" ||
			!authenticated || key != "skey_test_fixture" || password != "" {
			t.Fatal("unexpected provider request")
		}
		if err := req.ParseForm(); err != nil || len(req.PostForm) != 4 ||
			req.PostForm.Get("amount") != "918000" || req.PostForm.Get("currency") != "THB" ||
			req.PostForm.Get("card") != "tokn_test_fixture" || req.PostForm.Get("ref_id") != omiseTestRunID {
			t.Fatal("unexpected provider form")
		}
		return omiseProviderFixture(200, `{"object":"charge","id":"chrg_test_fixture","livemode":false,"amount":918000,
			"currency":"thb","status":"successful","paid":true,"ref_id":"`+omiseTestRunID+`","card":{"name":"private"}}`), nil
	})}}
	got := client.charge(context.Background(), omiseTestRunID, "tokn_test_fixture", 918000)
	if calls != 1 || got != (omiseChargeResult{Status: "successful", ChargeID: "chrg_test_fixture", Currency: "THB", Amount: 918000}) {
		t.Fatalf("unexpected provider result: calls = %d, result = %+v", calls, got)
	}
	client.secretKey = "skey_live_fixture"
	if got := client.charge(context.Background(), omiseTestRunID, "tokn_test_fixture", 918000); got.Status != "unknown" || calls != 1 {
		t.Fatal("live API key reached the transport")
	}
}

func TestOmiseProviderClassifiesOnlyValidatedResults(t *testing.T) {
	known := `{"object":"charge","id":"chrg_test_fixture","livemode":false,"amount":75000,"currency":"thb","status":"successful","paid":true}`
	for _, test := range []struct {
		name                      string
		httpStatus                int
		body                      string
		resultStatus, failureCode string
	}{
		{"valid", 200, known, "successful", ""},
		{"pending", 200, strings.ReplaceAll(strings.ReplaceAll(known, "successful", "pending"), "true", "false"), "pending", ""},
		{"failed", 200, strings.ReplaceAll(strings.ReplaceAll(known, `"successful"`, `"failed","failure_code":"failed_processing"`), "true", "false"), "failed", "failed_processing"},
		{"wrong mode", 200, strings.ReplaceAll(known, "false", "true"), "unknown", ""},
		{"missing mode", 200, strings.ReplaceAll(known, `"livemode":false,`, ""), "unknown", ""},
		{"wrong amount", 200, strings.ReplaceAll(known, "75000", "1"), "unknown", ""},
		{"wrong currency", 200, strings.ReplaceAll(known, "thb", "USD"), "unknown", ""},
		{"wrong ref", 200, strings.ReplaceAll(known, `"paid":true`, `"paid":true,"ref_id":"another-run"`), "unknown", ""},
		{"live ID", 200, strings.ReplaceAll(known, "chrg_test_", "chrg_"), "unknown", ""},
		{"contradictory paid", 200, strings.ReplaceAll(known, "true", "false"), "unknown", ""},
		{"unknown status", 200, strings.ReplaceAll(known, "successful", "settled"), "unknown", ""},
		{"trailing payload", 200, known + " {}", "unknown", ""},
		{"large body", 200, known + strings.Repeat(" ", maxOmiseResponse), "unknown", ""},
		{"no redirect", 302, known, "unknown", ""},
		{"rejected token", 400, `{"object":"error","code":"invalid_card_token","message":"private"}`, "failed", "invalid_card_token"},
		{"ambiguous upstream rejection", 400, `{"object":"error","code":"backend_error","message":"private"}`, "unknown", ""},
		{"limited", 429, `{"object":"error","code":"invalid_card_token"}`, "unknown", ""},
		{"gateway failure", 503, known, "unknown", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := omiseClient{secretKey: "skey_test_fixture", http: &http.Client{Transport: omiseTestTransport(func(req *http.Request) (*http.Response, error) {
				return omiseProviderFixture(test.httpStatus, test.body), nil
			})}}
			got := client.charge(context.Background(), omiseTestRunID, "tokn_test_fixture", 75000)
			if got.Status != test.resultStatus || got.FailureCode != test.failureCode || got.Currency != "THB" ||
				got.Amount != 75000 || (got.Status == "unknown" && got.ChargeID != "") {
				t.Fatalf("unexpected normalized response: %+v", got)
			}
		})
	}
}

func omiseTestRun(product string, quantity int, action string) run {
	stage := "PAYMENT"
	if product == "flight_ticket" || product == "hotel_room" {
		stage = "PRE_PAYMENT"
	}
	return run{ID: omiseTestRunID, Config: config{Product: product, Quantity: quantity}, Decisions: []decision{
		{Event: event{Stage: "BOOKING", OrderID: "first", Product: product, Quantity: quantity}, Action: "ALLOW"},
		{Event: event{Stage: stage, OrderID: "first", Product: product, Quantity: quantity}, Action: action},
		{Event: event{Stage: stage, OrderID: "other", Product: product, Quantity: quantity}, Action: "ALLOW"},
	}}
}

func TestCheckoutAmountUsesOnlyFirstOrderPaymentDecision(t *testing.T) {
	for _, tc := range []struct {
		product, action string
		quantity        int
		want            int64
	}{
		{"flight_ticket", "ALLOW", 2, 918000},
		{"hotel_room", "ALLOW", 3, 975000},
		{"physical", "ALLOW", 1, 249000},
		{"digital", "ALLOW", 20, 1500000},
		{"digital", "REVIEW", 2, 0},
		{"flight_ticket", "BLOCK", 2, 0},
		{"flight_ticket", "UNSUCCESSFUL", 2, 0},
		{"payment_gateway", "ALLOW", 2, 0},
		{"unknown", "ALLOW", 2, 0},
		{"physical", "ALLOW", 0, 0},
	} {
		got, allowed := checkoutAmount(omiseTestRun(tc.product, tc.quantity, tc.action))
		if got != tc.want || allowed != (tc.want > 0) {
			t.Errorf("checkoutAmount(%q, %q, %d) = %d, %t", tc.product, tc.action, tc.quantity, got, allowed)
		}
	}
}

type omiseTestStore struct {
	saved       run
	mu          sync.Mutex
	record      *omiseChargeRecord
	onDuplicate func()
}

func (store *omiseTestStore) get(_ context.Context, tenant, id string) (run, error) {
	if id != store.saved.ID || tenant != merchantID {
		return run{}, pgx.ErrNoRows
	}
	return store.saved, nil
}

func (store *omiseTestStore) reserveOmiseCharge(_ context.Context, _, _ string, amount int64) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.record != nil {
		if store.onDuplicate != nil {
			store.onDuplicate()
		}
		return false, nil
	}
	store.record = &omiseChargeRecord{Result: unknownOmiseCharge(amount), StartedAt: time.Now()}
	return true, nil
}

func (store *omiseTestStore) readOmiseCharge(_ context.Context, _, _ string) (omiseChargeRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return *store.record, nil
}

func (store *omiseTestStore) finishOmiseCharge(_ context.Context, _, _ string, result omiseChargeResult) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := time.Now()
	store.record.Result = result
	store.record.FinishedAt = &now
	return nil
}

func TestCheckoutConcurrentRequestsReplayOneProviderCharge(t *testing.T) {
	duplicateReserved := make(chan struct{})
	store := &omiseTestStore{
		saved:       omiseTestRun("digital", 1, "ALLOW"),
		onDuplicate: func() { close(duplicateReserved) },
	}
	checkout := newOmiseCheckout(store, "pkey_test_fixture", "skey_test_fixture")
	var calls atomic.Int32
	requestStarted := make(chan struct{})
	providerCanFinish := make(chan struct{})
	checkout.provider.http.Transport = omiseTestTransport(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(requestStarted)
		}
		<-providerCanFinish
		return omiseProviderFixture(200, `{"object":"charge","id":"chrg_test_fixture","livemode":false,"amount":75000,"currency":"thb","status":"successful"}`), nil
	})
	post := func(w *httptest.ResponseRecorder) {
		checkout.create(w, httptest.NewRequest(http.MethodPost, "/api/v1/simulation/checkout/charges", strings.NewReader(omiseTestBody)))
	}
	first := httptest.NewRecorder()
	second := httptest.NewRecorder()
	var requests sync.WaitGroup
	requests.Go(func() { post(first) })
	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		close(providerCanFinish)
		t.Fatal("first request did not reach the provider")
	}
	requests.Go(func() { post(second) })
	select {
	case <-duplicateReserved:
	case <-time.After(3 * time.Second):
		close(providerCanFinish)
		t.Fatal("second request did not replay the in-flight reservation")
	}
	close(providerCanFinish)
	requests.Wait()
	if calls.Load() != 1 || first.Code != 200 || second.Code != 200 || first.Body.String() != second.Body.String() ||
		!strings.Contains(first.Body.String(), `"status":"successful"`) {
		t.Fatalf("expected one replayed provider result, got calls %d, status %d/%d", calls.Load(), first.Code, second.Code)
	}
}

func TestCheckoutInterruptedReservationRemainsUnknown(t *testing.T) {
	store := &omiseTestStore{
		saved:  omiseTestRun("digital", 1, "ALLOW"),
		record: &omiseChargeRecord{Result: unknownOmiseCharge(75000), StartedAt: time.Now().Add(-time.Hour)},
	}
	checkout := newOmiseCheckout(store, "pkey_test_fixture", "skey_test_fixture")
	checkout.provider.http.Transport = omiseTestTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("provider called again for an interrupted reservation")
		return nil, nil
	})
	w := httptest.NewRecorder()
	checkout.create(w, httptest.NewRequest(http.MethodPost, "/api/v1/simulation/checkout/charges", strings.NewReader(omiseTestBody)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"unknown"`) {
		t.Fatalf("expected unknown saved attempt, got %d", w.Code)
	}
}

func TestCheckoutRejectsBlockedRunsBeforeReservation(t *testing.T) {
	store := &omiseTestStore{saved: omiseTestRun("flight_ticket", 1, "BLOCK")}
	handler := newOmiseCheckout(store, "pkey_test_fixture", "skey_test_fixture")
	for _, tc := range []struct {
		body   string
		status int
	}{
		{omiseTestBody, 409},
		{strings.ReplaceAll(omiseTestBody, "tokn_test_", "tokn_"), 400},
		{strings.ReplaceAll(omiseTestBody, omiseTestRunID, "11111111-1111-1111-1111-111111111111"), 404},
		{strings.TrimSuffix(omiseTestBody, "}") + `,"amount":1}`, 400},
	} {
		w := httptest.NewRecorder()
		handler.create(w, httptest.NewRequest(http.MethodPost, "/api/v1/simulation/checkout/charges", strings.NewReader(tc.body)))
		if w.Code != tc.status || store.record != nil || strings.Contains(w.Body.String(), "tokn_") {
			t.Fatalf("expected safe rejection %d before reservation, got %d", tc.status, w.Code)
		}
	}
}

func TestOmiseCheckoutIsDisabledForAuthenticatedWorkspaces(t *testing.T) {
	store := &omiseTestStore{saved: omiseTestRun("flight_ticket", 1, "ALLOW")}
	handler := newOmiseCheckout(store, "pkey_test_fixture", "skey_test_fixture")
	configRequest := httptest.NewRequest(http.MethodGet, "/api/v1/simulation/checkout/config", nil)
	configRequest.Header.Set("Authorization", "Bearer scoped-merchant-fixture")
	configResponse := httptest.NewRecorder()
	handler.config(configResponse, configRequest)
	if configResponse.Code != http.StatusOK || !strings.Contains(configResponse.Body.String(), `"enabled":false`) ||
		strings.Contains(configResponse.Body.String(), "publicKey") {
		t.Fatalf("authenticated workspace received checkout configuration: %d", configResponse.Code)
	}

	chargeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/simulation/checkout/charges", strings.NewReader(omiseTestBody))
	chargeRequest.Header.Set("Authorization", "Bearer scoped-merchant-fixture")
	chargeResponse := httptest.NewRecorder()
	handler.create(chargeResponse, chargeRequest)
	if chargeResponse.Code != http.StatusForbidden || store.record != nil {
		t.Fatalf("authenticated workspace reached checkout storage: %d", chargeResponse.Code)
	}
}
