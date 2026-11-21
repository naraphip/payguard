package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	openRouterDecisionsURL  = "https://openrouter.ai/api/alpha/decisions"
	defaultMerchantJevModel = "typesafe/jev-1.14"
	merchantJevTimeout      = 5 * time.Second
	merchantJevMaxResponse  = 16 << 10
	merchantJevQuestion     = "booking_risk"
)

var (
	merchantJevModelPattern = regexp.MustCompile(`^typesafe/jev-[0-9]+\.[0-9]+(-[0-9]{8})?$`)
	merchantJevIDPattern    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,160}$`)
)

type merchantJev struct {
	live   bool
	key    string
	model  string
	client *http.Client
}

func newMerchantJev(mode, key, model string) (merchantJev, error) {
	switch strings.TrimSpace(mode) {
	case "", "mock":
		return merchantJev{}, nil
	case "live":
		model = strings.TrimSpace(model)
		if model == "" {
			model = defaultMerchantJevModel
		}
		if !merchantJevModelPattern.MatchString(model) {
			return merchantJev{}, errors.New("OPENROUTER_MODEL_PRIMARY must identify a versioned TypeSafe Jev model")
		}
		return merchantJev{live: true, key: strings.TrimSpace(key), model: model, client: &http.Client{
			Timeout: merchantJevTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}}, nil
	default:
		return merchantJev{}, errors.New("PAYGUARD_AI_MODE must be mock or live")
	}
}

type merchantJevState struct {
	AmountMinor          int64 `json:"booking_amount_minor"`
	ReviewThresholdMinor int64 `json:"review_threshold_minor"`
	ReviewRuleMatched    bool  `json:"configured_review_rule_matched"`
}

type merchantJevChoice struct {
	Type       string   `json:"type"`
	Choice     string   `json:"choice"`
	Confidence *float64 `json:"confidence"`
}

type merchantJevResponse struct {
	ID      string                       `json:"id"`
	Model   string                       `json:"model"`
	Answers map[string]merchantJevChoice `json:"answers"`
}

func evaluateMerchantJev(ctx context.Context, input mockTransactionInput, scenario mockScenario, rules jevRulesSnapshot, jev merchantJev) mockTransaction {
	result := evaluateMockJev(input, scenario)
	result.applyMerchantJevRules(rules)
	if !jev.live || input.SimulateTimeout {
		return result
	}
	result.JevVersion = jev.model
	if input.AmountMinor == nil {
		result.JevStatus = "NOT_RUN"
		return result
	}

	state := merchantJevState{
		AmountMinor:          *input.AmountMinor,
		ReviewThresholdMinor: scenario.ThresholdMinor,
		ReviewRuleMatched:    result.Outcome == "REVIEW",
	}
	response, status, reason := jev.requestDecision(ctx, state)
	result.JevStatus = status
	if status != "LIVE" {
		result.Outcome, result.ReviewState, result.Reason = "UNRESOLVED", "NEEDS_DECISION", reason
		return result
	}
	choice := response.Answers[merchantJevQuestion]
	result.JevVersion, result.JevDecisionID = response.Model, response.ID
	result.JevAnswer, result.JevConfidence = choice.Choice, choice.Confidence
	if choice.Choice == "REVIEW" && result.Outcome != "REVIEW" {
		result.Outcome, result.ReviewState = "REVIEW", "NEEDS_DECISION"
		result.Reason = "Live Jev recommended review based on the booking evidence."
	}
	return result
}

func (j merchantJev) requestDecision(ctx context.Context, state merchantJevState) (merchantJevResponse, string, string) {
	var result merchantJevResponse
	if j.key == "" {
		return result, "UNAVAILABLE", "Live Jev is missing its OpenRouter API key. Review is required."
	}

	type question struct {
		Type         string            `json:"type"`
		Instructions string            `json:"instructions"`
		Criteria     map[string]string `json:"criteria"`
	}
	payload := struct {
		Model     string              `json:"model"`
		State     merchantJevState    `json:"state"`
		Questions map[string]question `json:"questions"`
	}{
		Model: j.model,
		State: state,
		Questions: map[string]question{
			merchantJevQuestion: {
				Type:         "choice",
				Instructions: "Assess this synthetic booking using only the provided evidence and the configured review rule result.",
				Criteria: map[string]string{
					"LOW_RISK": "The amount is below the review threshold and no configured review rule matched.",
					"REVIEW":   "The configured review rule matched, including when the amount meets or exceeds the review threshold.",
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return result, "UNAVAILABLE", "Live Jev request could not be prepared. Review is required."
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterDecisionsURL, bytes.NewReader(body))
	if err != nil {
		return result, "UNAVAILABLE", "Live Jev request could not be prepared. Review is required."
	}
	req.Header.Set("Authorization", "Bearer "+j.key)
	req.Header.Set("Content-Type", "application/json")
	response, err := j.client.Do(req)
	if err != nil {
		status, reason := merchantJevRequestFailure(err)
		return result, status, reason
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusRequestTimeout, http.StatusGatewayTimeout, 524:
		return result, "TIMEOUT", "Live Jev timed out before returning a decision. Review is required."
	case http.StatusOK:
	default:
		return result, "UNAVAILABLE", fmt.Sprintf("Live Jev returned HTTP %d. Review is required.", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, merchantJevMaxResponse+1))
	if err != nil {
		status, reason := merchantJevRequestFailure(err)
		return result, status, reason
	}
	if len(data) > merchantJevMaxResponse || json.Unmarshal(data, &result) != nil || !result.validFor(j.model) {
		return merchantJevResponse{}, "UNAVAILABLE", "Live Jev returned an invalid decision. Review is required."
	}
	return result, "LIVE", ""
}

func (r merchantJevResponse) validFor(model string) bool {
	choice := r.Answers[merchantJevQuestion]
	return (r.ID == "" || merchantJevIDPattern.MatchString(r.ID)) &&
		merchantJevModelPattern.MatchString(r.Model) && (r.Model == model || strings.HasPrefix(r.Model, model+"-")) &&
		choice.Type == "choice" && (choice.Choice == "LOW_RISK" || choice.Choice == "REVIEW") &&
		(choice.Confidence == nil || (*choice.Confidence >= 0 && *choice.Confidence <= 1))
}

func merchantJevRequestFailure(err error) (status, reason string) {
	var networkErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkErr) && networkErr.Timeout()) {
		return "TIMEOUT", "Live Jev timed out before returning a decision. Review is required."
	}
	return "UNAVAILABLE", "Live Jev could not be reached. Review is required."
}
