package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestJevConfigurationRequiresJevModels(t *testing.T) {
	for _, mutate := range []func(*jevAIConfiguration){
		func(c *jevAIConfiguration) { c.Provider = "openai" },
		func(c *jevAIConfiguration) { c.Model = "gpt-4.1-mini" },
		func(c *jevAIConfiguration) { c.FallbackModel = c.Model },
		func(c *jevAIConfiguration) { c.Rubric = "overrule review" },
		func(c *jevAIConfiguration) { c.TimeoutMS = 30000 },
	} {
		config := defaultJevAIConfiguration()
		mutate(&config)
		if err := config.validate(); err == nil {
			t.Fatalf("accepted invalid configuration %+v", config)
		}
	}
}

func TestJevSyntheticDiagnosticUsesOnlyDecisionEndpoint(t *testing.T) {
	var models []string
	var modelMutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/alpha/decisions" || r.Header.Get("Authorization") != "Bearer synthetic-jev-key" {
			t.Errorf("unexpected synthetic request")
			http.Error(w, "unexpected", 400)
			return
		}
		var payload struct {
			Model     string         `json:"model"`
			State     map[string]any `json:"state"`
			Questions map[string]struct {
				Type         string
				Instructions string
				Criteria     map[string]string
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			http.Error(w, "bad", 400)
			return
		}
		modelMutex.Lock()
		models = append(models, payload.Model)
		modelCount := len(models)
		modelMutex.Unlock()
		question := payload.Questions[merchantJevQuestion]
		if len(payload.State) != 3 || question.Type != "choice" || question.Instructions != jevAdvisoryRubrics["standard-v1"] || len(question.Criteria) != 2 {
			t.Errorf("request must contain only synthetic facts and a typed rubric: %+v", payload)
		}
		if modelCount == 1 {
			http.Error(w, "unavailable", 503)
			return
		}
		io.WriteString(w, `{"model":"typesafe/jev-1.13","answers":{"booking_risk":{"type":"choice","choice":"REVIEW"}}}`)
	}))
	defer server.Close()
	handler := jevAIConfigurationHandler{client: server.Client(), endpoint: server.URL + "/api/alpha/decisions", apiKey: "synthetic-jev-key"}
	config := defaultJevAIConfiguration()
	if got := handler.diagnose(t.Context(), config); got.Status != "MOCKED" {
		t.Fatalf("mock diagnostic reached an external provider: %+v", got)
	}
	config.Provider = "openrouter"
	got := handler.diagnose(t.Context(), config)
	modelMutex.Lock()
	defer modelMutex.Unlock()
	if got.Status != "LIVE" || got.Decision != "REVIEW" || !got.FallbackUsed || !reflect.DeepEqual(models, jevModelChoices) {
		t.Fatalf("wrong diagnostic or fallback: %+v, %v", got, models)
	}
	handler.apiKey = ""
	if got := handler.diagnose(t.Context(), config); got.Status != "UNAVAILABLE" || got.Decision != "" {
		t.Fatalf("missing credential represented as a result: %+v", got)
	}
}

func TestJevConfigurationPreservesReviewRulesPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	admin := "test-config-admin"
	t.Setenv("OPENROUTER_API_KEY", "synthetic-secret-must-not-be-visible")
	merchant, token, err := repo.createMockMerchant(t.Context(), mockMerchantInput{Name: "Jev configuration isolation test"})
	if err != nil {
		t.Fatal(err)
	}
	threshold := int64(100)
	reviewRule := jevRuleDefinition{Name: "Historical review", Action: "REVIEW",
		Conditions: []jevRuleCondition{{Signal: bookingAmount, Operator: "gte", Value: &threshold}}}
	snapshot, err := repo.publishJevRule(t.Context(), merchant.ID, reviewRule)
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := repo.saveMockScenario(t.Context(), merchant.ID, "", "Legacy review lineage", 500)
	if err != nil {
		t.Fatal(err)
	}
	input := mockTransactionInput{ID: "a951ac06-a094-4e23-a383-eac70eccf4c2", ScenarioID: scenario.ID,
		Source: "checkout", Product: "flight_ticket", AmountMinor: &threshold}
	original, created, err := repo.saveMockTransaction(t.Context(), merchant.ID, input)
	if err != nil || !created || original.Outcome != "REVIEW" || original.JevRuleVersion != snapshot.Version {
		t.Fatalf("could not seed legacy review lineage: %+v %v", original, err)
	}

	request := func(mux *http.ServeMux, method, path, token string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		var data bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&data).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &data)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s returned %d (want %d) %s", method, path, w.Code, want, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "synthetic-secret") {
			t.Fatal("Provider key leaked.")
		}
		return w
	}
	mux := http.NewServeMux()
	registerJevAIConfiguration(mux, repo, admin)
	request(mux, http.MethodGet, jevAIConfigurationPath, "", nil, http.StatusUnauthorized)
	request(mux, http.MethodGet, jevAIConfigurationPath, token, nil, http.StatusForbidden)
	request(mux, http.MethodPut, jevAIConfigurationPath, token, map[string]any{}, http.StatusForbidden)
	request(mux, http.MethodPost, jevAIConfigurationPath+"/test", token, map[string]any{}, http.StatusForbidden)
	view := jevAIConfigurationView{}
	response := request(mux, http.MethodGet, jevAIConfigurationPath, admin, nil, http.StatusOK)
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || view.Config.Provider != "mock" {
		t.Fatalf("wrong defaults: %+v, %v", view, err)
	}
	diagnostic := request(mux, http.MethodPost, jevAIConfigurationPath+"/test", admin, map[string]any{}, http.StatusOK)
	if !strings.Contains(diagnostic.Body.String(), `"status":"MOCKED"`) {
		t.Fatal("mock diagnostic must report mock status")
	}
	originalVersion := view.Version
	nextRubric := "conservative-v1"
	if view.Config.Rubric == nextRubric {
		nextRubric = "standard-v1"
	}
	view.Config.Rubric = nextRubric
	update := map[string]any{"config": view.Config, "expectedVersion": originalVersion}
	request(mux, http.MethodPut, jevAIConfigurationPath, admin, update, http.StatusOK)
	request(mux, http.MethodPut, jevAIConfigurationPath, admin, update, http.StatusConflict)
	second := http.NewServeMux()
	registerJevAIConfiguration(second, repo, admin)
	if err := json.Unmarshal(request(second, http.MethodGet, jevAIConfigurationPath, admin, nil, http.StatusOK).Body.Bytes(), &view); err != nil ||
		view.Config.Rubric != nextRubric || view.Version != originalVersion+1 {
		t.Fatalf("configuration not persisted: %+v %v", view, err)
	}
	after, err := readJevRules(t.Context(), db, merchant.ID)
	if err != nil || !reflect.DeepEqual(snapshot, after) {
		t.Fatalf("legacy published review snapshot changed: %+v %+v %v", snapshot, after, err)
	}
	replayed, created, err := repo.saveMockTransaction(t.Context(), merchant.ID, input)
	if err != nil || created || !reflect.DeepEqual(original, replayed) {
		t.Fatalf("legacy review replay changed after AI Configuration: %+v %v", replayed, err)
	}
	mockConfig := view.Config
	view.Config.Provider = "openrouter"
	view.Config.Model = "typesafe/jev-1.13"
	view.Config.FallbackModel = ""
	request(second, http.MethodPut, jevAIConfigurationPath, admin, map[string]any{"config": view.Config, "expectedVersion": view.Version}, http.StatusOK)
	liveVersion := view.Version + 1
	calls := 0
	provider := merchantJev{key: "test-key", client: &http.Client{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload struct {
			Model string           `json:"model"`
			State merchantJevState `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Model != "typesafe/jev-1.13" || !payload.State.ReviewRuleMatched {
			t.Errorf("saved model or deterministic review was not applied: %+v %v", payload, err)
		}
		return testJevResponse(200, `{"model":"typesafe/jev-1.13","answers":{"booking_risk":{"type":"choice","choice":"LOW_RISK"}}}`), nil
	})}}
	replayed, created, err = repo.saveMockTransaction(t.Context(), merchant.ID, input, provider)
	if err != nil || created || calls != 0 || !reflect.DeepEqual(original, replayed) {
		t.Fatal("historical replay consulted new model settings")
	}
	input.ID = "a951ac06-a094-4e23-a383-eac70eccf4c3"
	future, created, err := repo.saveMockTransaction(t.Context(), merchant.ID, input, provider)
	if err != nil || !created || calls != 1 || future.JevStatus != "LIVE" || future.JevVersion != "typesafe/jev-1.13" ||
		future.Outcome != "REVIEW" || future.JevConfigurationVersion != liveVersion {
		t.Fatalf("future decision missing pinned configuration: %+v %v", future, err)
	}
	persisted, err := readMockTransaction(t.Context(), db, merchant.ID, input.ID)
	if err != nil || !reflect.DeepEqual(future, persisted) {
		t.Fatalf("future decision not persisted exactly: %+v %v", persisted, err)
	}
	request(second, http.MethodPut, jevAIConfigurationPath, admin, map[string]any{"config": mockConfig, "expectedVersion": liveVersion}, http.StatusOK)
}

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (transport diagnosticTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestJevDiagnosticTimeoutAndInvalidChoice(t *testing.T) {
	config := defaultJevAIConfiguration()
	config.Provider, config.FallbackModel, config.TimeoutMS = "openrouter", "", 250
	handler := jevAIConfigurationHandler{endpoint: openRouterDecisionsURL, apiKey: "synthetic-key"}
	handler.client = &http.Client{Transport: diagnosticTransport(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	if got := handler.diagnose(t.Context(), config); got.Status != "TIMEOUT" || got.Decision != "" {
		t.Fatalf("timeout represented as a decision: %+v", got)
	}
	handler.client.Transport = diagnosticTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"model":"typesafe/jev-1.14","answers":{"booking_risk":{"type":"choice","choice":"BLOCK"}}}`))}, nil
	})
	if got := handler.diagnose(t.Context(), config); got.Status != "UNAVAILABLE" || got.Decision != "" {
		t.Fatalf("out-of-rubric decision accepted: %+v", got)
	}
}
