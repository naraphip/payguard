package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const jevAIConfigurationPath = "/api/v1/booking/ai-configuration/jev"

const jevAIConfigurationSchema = `
ALTER TABLE merchant_demo_transactions ADD COLUMN IF NOT EXISTS jev_configuration_version bigint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS jev_ai_configuration_versions (
	version bigint PRIMARY KEY CHECK (version > 0),
	config jsonb NOT NULL,
	created_by text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
`

var jevModelChoices = []string{"typesafe/jev-1.14", "typesafe/jev-1.13"}

var jevAdvisoryRubrics = map[string]string{
	"standard-v1":     "Request review when a known risk signal is materially elevated. Choose insufficient evidence when transaction signals are absent.",
	"conservative-v1": "Request review for any elevated risk signal. Choose insufficient evidence when known signals cannot support an assessment.",
}

type jevAIConfiguration struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	FallbackModel string `json:"fallbackModel"`
	Rubric        string `json:"rubric"`
	TimeoutMS     int    `json:"timeoutMs"`
}

func defaultJevAIConfiguration() jevAIConfiguration {
	return jevAIConfiguration{"mock", "typesafe/jev-1.14", "typesafe/jev-1.13", "standard-v1", 2500}
}

func (config *jevAIConfiguration) validate() error {
	config.Provider = strings.TrimSpace(config.Provider)
	config.Model = strings.TrimSpace(config.Model)
	config.FallbackModel = strings.TrimSpace(config.FallbackModel)
	config.Rubric = strings.TrimSpace(config.Rubric)
	if config.Provider != "mock" && config.Provider != "openrouter" {
		return errors.New("Choose Mock or OpenRouter Jev.")
	}
	if !slices.Contains(jevModelChoices, config.Model) {
		return errors.New("Choose an allowed TypeSafe Jev model.")
	}
	if config.FallbackModel != "" && (!slices.Contains(jevModelChoices, config.FallbackModel) || config.FallbackModel == config.Model) {
		return errors.New("Choose a different TypeSafe Jev fallback model or leave fallback empty.")
	}
	if _, exists := jevAdvisoryRubrics[config.Rubric]; !exists {
		return errors.New("Choose a supported Jev rubric.")
	}
	if config.TimeoutMS < 250 || config.TimeoutMS > 10000 {
		return errors.New("Timeout must be between 250 and 10,000 milliseconds.")
	}
	return nil
}

type jevAIConfigurationView struct {
	Config     jevAIConfiguration `json:"config"`
	Version    int64              `json:"version"`
	Configured bool               `json:"configured"`
	UpdatedAt  *time.Time         `json:"updatedAt,omitempty"`
}

type jevAIConfigurationHandler struct {
	access     mockMerchantService
	client     *http.Client
	endpoint   string
	apiKey     string
	deployment merchantJev
}

func registerJevAIConfiguration(mux *http.ServeMux, repo repository, adminToken string, options ...merchantJev) {
	handler := jevAIConfigurationHandler{
		access: composerAuthorization(repo, adminToken), endpoint: openRouterDecisionsURL,
		apiKey: strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	if len(options) > 0 {
		handler.deployment = options[0]
	}
	mux.HandleFunc("GET "+jevAIConfigurationPath, handler.authorized(handler.read))
	mux.HandleFunc("PUT "+jevAIConfigurationPath, handler.authorized(handler.save))
	mux.HandleFunc("POST "+jevAIConfigurationPath+"/test", handler.authorized(handler.test))
}

func (h jevAIConfigurationHandler) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if _, ok := h.access.authorize(w, r, true); ok {
			next(w, r)
		}
	}
}

func readJevAIConfiguration(ctx context.Context, db systemDraftWriter) (jevAIConfigurationView, error) {
	var result jevAIConfigurationView
	err := db.QueryRow(ctx, `SELECT version,config,created_at FROM jev_ai_configuration_versions ORDER BY version DESC LIMIT 1`).
		Scan(&result.Version, &result.Config, &result.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return jevAIConfigurationView{Config: defaultJevAIConfiguration()}, nil
	}
	return result, err
}

func (h jevAIConfigurationHandler) effectiveView(result jevAIConfigurationView) jevAIConfigurationView {
	if result.Version == 0 && h.deployment.live {
		result.Config = jevAIConfiguration{"openrouter", h.deployment.model, "", "standard-v1", int(merchantJevTimeout / time.Millisecond)}
	}
	return result
}
func (h jevAIConfigurationHandler) reply(w http.ResponseWriter, result jevAIConfigurationView) {
	result = h.effectiveView(result)
	result.Configured = result.Config.Provider == "mock" || (h.apiKey != "" && bookingTokenPattern.MatchString(h.apiKey))
	respond(w, http.StatusOK, result)
}

func (h jevAIConfigurationHandler) read(w http.ResponseWriter, r *http.Request) {
	result, err := readJevAIConfiguration(r.Context(), h.access.repo.db)
	if err != nil {
		unavailable(w, err)
		return
	}
	h.reply(w, result)
}

var errJevAIConfigurationChanged = errors.New("AI Configuration changed. Reload the latest version.")

func (h jevAIConfigurationHandler) save(w http.ResponseWriter, r *http.Request) {
	input := struct {
		Config          jevAIConfiguration `json:"config"`
		ExpectedVersion *int64             `json:"expectedVersion"`
	}{}
	if !decodeRuleRequest(w, r, &input) {
		return
	}
	if input.ExpectedVersion == nil || *input.ExpectedVersion < 0 {
		problem(w, http.StatusUnprocessableEntity, "A nonnegative expectedVersion is required.")
		return
	}
	if err := input.Config.validate(); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var result jevAIConfigurationView
	ctx := r.Context()
	err := pgx.BeginFunc(ctx, h.access.repo.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(426380583)`); err != nil {
			return err
		}
		current, err := readJevAIConfiguration(ctx, tx)
		if err != nil {
			return err
		}
		current = h.effectiveView(current)
		if current.Version != *input.ExpectedVersion {
			return errJevAIConfigurationChanged
		}
		if current.Config == input.Config {
			result = current
			return nil
		}
		return tx.QueryRow(ctx, `INSERT INTO jev_ai_configuration_versions(version,config,created_by)
			VALUES ($1,$2,'platform-admin') RETURNING version,config,created_at`,
			current.Version+1, input.Config).Scan(&result.Version, &result.Config, &result.UpdatedAt)
	})
	if errors.Is(err, errJevAIConfigurationChanged) {
		problem(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		unavailable(w, err)
		return
	}
	h.reply(w, result)
}

type jevAdvisoryResult struct {
	Status       string `json:"status"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Rubric       string `json:"rubric"`
	Decision     string `json:"decision,omitempty"`
	FallbackUsed bool   `json:"fallbackUsed"`
	Message      string `json:"message,omitempty"`
}

func (h jevAIConfigurationHandler) diagnose(ctx context.Context, config jevAIConfiguration) jevAdvisoryResult {
	result := jevAdvisoryResult{Status: "UNAVAILABLE", Provider: config.Provider, Model: config.Model, Rubric: config.Rubric}
	if config.Provider == "mock" {
		result.Status, result.Model, result.Decision = "MOCKED", jevVersion, "REVIEW"
		return result
	}
	if h.apiKey == "" || !bookingTokenPattern.MatchString(h.apiKey) {
		result.Message = "Configure OPENROUTER_API_KEY on the Go server for Jev Decisions."
		return result
	}
	provider := merchantJev{key: h.apiKey, client: h.client, endpoint: h.endpoint}.withConfiguration(config)
	threshold := int64(10000)
	answer, status, reason := provider.requestDecision(ctx, merchantJevState{AmountMinor: 12000, ReviewThresholdMinor: &threshold, ReviewRuleMatched: true})
	result.Status, result.FallbackUsed, result.Message = status, answer.FallbackUsed, reason
	if status == "LIVE" {
		result.Model, result.Decision = answer.Model, answer.Answers[merchantJevQuestion].Choice
	}
	return result
}

func (h jevAIConfigurationHandler) test(w http.ResponseWriter, r *http.Request) {
	if !decodeRuleRequest(w, r, &struct{}{}) {
		return
	}
	result, err := readJevAIConfiguration(r.Context(), h.access.repo.db)
	if err != nil {
		unavailable(w, err)
		return
	}
	respond(w, http.StatusOK, h.diagnose(r.Context(), h.effectiveView(result).Config))
}
