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
	live          bool
	key           string
	model         string
	client        *http.Client
	fallbackModel string
	rubric        string
	timeout       time.Duration
	endpoint      string
}

func newMerchantJev(mode, key, model string) (merchantJev, error) {
	switch strings.TrimSpace(mode) {
	case "", "mock":
		return merchantJev{key: strings.TrimSpace(key)}, nil
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

func (j merchantJev) withConfiguration(config jevAIConfiguration) merchantJev {
	j.live, j.model, j.fallbackModel, j.rubric = config.Provider == "openrouter", config.Model, config.FallbackModel, config.Rubric
	j.timeout = time.Duration(config.TimeoutMS) * time.Millisecond
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if j.client != nil {
		client.Transport = j.client.Transport
	}
	j.client = &client
	return j
}

type merchantJevState struct {
	AmountMinor          *int64                             `json:"booking_amount_minor"`
	ReviewThresholdMinor *int64                             `json:"review_threshold_minor,omitempty"`
	ReviewRuleMatched    bool                               `json:"configured_review_rule_matched"`
	BookingComparison    *merchantBookingComparisonEvidence `json:"booking_comparison,omitempty"`
}

type merchantJevChoice struct {
	Type       string   `json:"type"`
	Choice     string   `json:"choice"`
	Confidence *float64 `json:"confidence"`
}

type merchantJevQuestionDefinition struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type merchantJevQuestions map[string]merchantJevQuestionDefinition

type merchantJevResponse struct {
	FallbackUsed bool                         `json:"-"`
	ID           string                       `json:"id"`
	Model        string                       `json:"model"`
	Answers      map[string]merchantJevChoice `json:"answers"`
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
		AmountMinor:          input.AmountMinor,
		ReviewThresholdMinor: &scenario.ThresholdMinor,
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

func evaluateJevWithoutScenario(ctx context.Context, input mockTransactionInput, rules jevRulesSnapshot, baseReview bool, jev merchantJev, comparison *merchantBookingComparison) mockTransaction {
	result := mockTransaction{
		ID: input.ID, Source: input.Source, Product: input.Product, AmountMinor: input.AmountMinor,
		Outcome: "LOW_RISK", ReviewState: "NONE", RuleHits: []string{},
		JevStatus: "MOCKED", JevVersion: "mock-jev-v1", simulateTimeout: input.SimulateTimeout,
	}
	result.applyMerchantJevRules(rules)
	reviewRuleMatched := result.Outcome == "REVIEW"
	switch {
	case input.SimulateTimeout:
		result.JevStatus, result.JevReason = "TIMEOUT", "Mock Jev timed out before returning a decision."
	case input.AmountMinor == nil && comparison == nil:
		result.JevStatus, result.JevReason = "NOT_RUN", "Jev requires the transaction amount to assess risk."
		if jev.live {
			result.JevVersion = jev.model
		}
	case jev.live:
		assessLiveWithoutScenario(ctx, &result, baseReview || reviewRuleMatched, jev, comparison)
	default:
		comparisonReview := comparison != nil && comparison.mock()
		result.JevAnswer = "LOW_RISK"
		result.JevReason = "Mock Jev found no review signal in the supplied evidence."
		if baseReview || reviewRuleMatched {
			result.JevAnswer = "REVIEW"
			result.JevReason = "Mock Jev received a matching review signal."
		} else if comparisonReview {
			result.JevAnswer = "REVIEW"
			result.JevReason = "Mock Jev found a booking change following a prior manual rejection."
		}
	}
	if comparison != nil {
		comparison.Status = result.JevStatus
	}
	if reviewRuleMatched {
		return result
	}
	result.Reason = result.JevReason
	switch result.JevAnswer {
	case "REVIEW":
		result.Outcome, result.ReviewState = "REVIEW", "NEEDS_DECISION"
	case "":
		result.Outcome, result.ReviewState = "UNRESOLVED", "NEEDS_DECISION"
	}
	return result
}

func assessLiveWithoutScenario(ctx context.Context, result *mockTransaction, reviewRuleMatched bool, jev merchantJev, comparison *merchantBookingComparison) {
	result.JevVersion = jev.model
	state := merchantJevState{AmountMinor: result.AmountMinor, ReviewRuleMatched: reviewRuleMatched}
	if comparison != nil {
		state.BookingComparison = &comparison.merchantBookingComparisonEvidence
	}
	response, resultStatus, reason := jev.requestDecision(ctx, state)
	result.JevStatus, result.JevReason = resultStatus, reason
	if resultStatus == "LIVE" {
		choice := response.Answers[merchantJevQuestion]
		result.JevVersion, result.JevDecisionID = response.Model, response.ID
		result.JevAnswer, result.JevConfidence = choice.Choice, choice.Confidence
		if comparison != nil {
			answer := response.Answers[merchantBookingSimilarityQuestion]
			comparison.Similarity, comparison.Confidence = answer.Choice, answer.Confidence
		}
		result.JevReason = "Live Jev assessed the transaction as low risk."
		if choice.Choice == "REVIEW" {
			result.JevReason = "Live Jev recommended review based on the transaction evidence."
		}
	}
}

func (j merchantJev) requestDecision(ctx context.Context, state merchantJevState) (merchantJevResponse, string, string) {
	riskQuestion := merchantJevQuestionDefinition{
		Type:         "choice",
		Instructions: "Assess this synthetic booking using only the provided evidence and the configured review rule result.",
		Criteria: map[string]string{
			"LOW_RISK": "The amount is below the review threshold and no configured review rule matched.",
			"REVIEW":   "The configured review rule matched, including when the amount meets or exceeds the review threshold.",
		},
	}
	if state.ReviewThresholdMinor == nil {
		riskQuestion.Instructions = "Assess this synthetic transaction using only the observed amount and the configured review rule result. Apply no assumed review threshold."
		riskQuestion.Criteria = map[string]string{
			"LOW_RISK": "The supplied evidence shows no elevated risk and no configured review rule matched.",
			"REVIEW":   "The supplied evidence shows elevated risk or a configured review rule matched.",
		}
	}
	questions := make(merchantJevQuestions, 2)
	if state.BookingComparison != nil {
		riskQuestion.Instructions = "Assess this synthetic transaction using the configured rule result and the supplied prior and current booking evidence. A shared mailbox provides comparison context only. Product, amount, or metadata changes alone can be legitimate. Treat null amounts and unknown comparisons as missing evidence. Never invent a threshold or assume missing amounts are zero."
		riskQuestion.Criteria = map[string]string{
			"LOW_RISK": "Available evidence shows no material risk and no configured review rule matched.",
			"REVIEW":   "A configured review rule matched or the booking comparison supplies corroborated elevated risk, such as a prior manual rejection together with a changed product or amount.",
		}
		questions[merchantBookingSimilarityQuestion] = merchantJevQuestionDefinition{
			Type:         "choice",
			Instructions: "Compare only known product types, known amounts, and the locally evaluated description and custom-field comparisons in booking_comparison. Treat null amounts and UNKNOWN as missing. Similarity is independent of risk and of the prior manual disposition.",
			Criteria: map[string]string{
				"SIMILAR":      "Two or more known fields agree, or meaningful description or registered metadata agrees, without a known difference.",
				"DIFFERENT":    "At least one comparable product, amount, description, or custom field differs.",
				"INSUFFICIENT": "There is too little comparable booking evidence to establish similarity or a difference.",
			},
		}
	}
	if instructions := jevAdvisoryRubrics[j.rubric]; instructions != "" {
		if state.ReviewThresholdMinor == nil {
			riskQuestion.Instructions += " " + instructions
		} else {
			riskQuestion.Instructions = instructions
		}
	}
	questions[merchantJevQuestion] = riskQuestion
	return j.requestChoices(ctx, state, questions)
}

func (j merchantJev) requestChoices(ctx context.Context, state any, questions merchantJevQuestions) (merchantJevResponse, string, string) {
	if j.timeout == 0 || j.key == "" {
		return j.requestChoicesOnce(ctx, state, questions)
	}
	ctx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()
	primaryBudget := j.timeout
	if j.fallbackModel != "" {
		primaryBudget /= 2
	}
	primary, stop := context.WithTimeout(ctx, primaryBudget)
	answer, status, reason := j.requestChoicesOnce(primary, state, questions)
	stop()
	if status == "LIVE" || j.fallbackModel == "" || ctx.Err() != nil {
		return answer, status, reason
	}
	j.model = j.fallbackModel
	answer, status, reason = j.requestChoicesOnce(ctx, state, questions)
	answer.FallbackUsed = true
	return answer, status, reason
}

func (j merchantJev) requestChoicesOnce(ctx context.Context, state any, questions merchantJevQuestions) (merchantJevResponse, string, string) {
	var result merchantJevResponse
	if j.key == "" {
		return result, "UNAVAILABLE", "Live Jev is missing its OpenRouter API key. Review is required."
	}
	payload := struct {
		Model     string               `json:"model"`
		State     any                  `json:"state"`
		Questions merchantJevQuestions `json:"questions"`
	}{j.model, state, questions}
	body, err := json.Marshal(payload)
	if err != nil {
		return result, "UNAVAILABLE", "Live Jev request could not be prepared. Review is required."
	}
	endpoint := j.endpoint
	if endpoint == "" {
		endpoint = openRouterDecisionsURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
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
	// Bound the response by the number of questions our server requested.
	maxResponse := max(merchantJevMaxResponse, 2048*len(questions))
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(maxResponse)+1))
	if err != nil {
		status, reason := merchantJevRequestFailure(err)
		return result, status, reason
	}
	if len(data) > maxResponse || json.Unmarshal(data, &result) != nil || !result.validFor(j.model, questions) {
		return merchantJevResponse{}, "UNAVAILABLE", "Live Jev returned an invalid decision. Review is required."
	}
	return result, "LIVE", ""
}

func (r merchantJevResponse) validFor(model string, questions merchantJevQuestions) bool {
	if (r.ID != "" && !merchantJevIDPattern.MatchString(r.ID)) || !merchantJevModelPattern.MatchString(r.Model) ||
		(r.Model != model && !strings.HasPrefix(r.Model, model+"-")) || len(r.Answers) != len(questions) {
		return false
	}
	for key, question := range questions {
		answer := r.Answers[key]
		if _, ok := question.Criteria[answer.Choice]; !ok || answer.Type != question.Type ||
			(answer.Confidence != nil && (*answer.Confidence < 0 || *answer.Confidence > 1)) {
			return false
		}
	}
	return true
}

func merchantJevRequestFailure(err error) (status, reason string) {
	var networkErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkErr) && networkErr.Timeout()) {
		return "TIMEOUT", "Live Jev timed out before returning a decision. Review is required."
	}
	return "UNAVAILABLE", "Live Jev could not be reached. Review is required."
}
