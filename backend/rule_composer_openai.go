package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	openAIChatEndpoint     = "https://api.openai.com/v1/chat/completions"
	composerResponseLimit  = 64 << 10
	composerOutputLimit    = 12 << 10
	composerAttemptTimeout = 8 * time.Second
)

var (
	errComposerUpstream     = errors.New("rule composer provider unavailable")
	errComposerOutput       = errors.New("invalid rule composer output")
	composerModelID         = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,127}$`)
	composerProviderAuth    = &composerProviderFailure{"PROVIDER_AUTH", "OpenAI rejected the configured access. Check the backend API key and project permissions."}
	composerProviderRoute   = &composerProviderFailure{"PROVIDER_ROUTE", "The configured model is unavailable to this OpenAI project. Check the model name and project permissions."}
	composerProviderRate    = &composerProviderFailure{"PROVIDER_RATE_LIMIT", "OpenAI rate or quota limits prevented interpretation. Check your project limits, retry shortly, or create the rule manually."}
	composerProviderTimeout = &composerProviderFailure{"PROVIDER_TIMEOUT", "OpenAI timed out. Retry or create the rule manually."}
	composerProviderRequest = &composerProviderFailure{"PROVIDER_REQUEST", "OpenAI rejected the structured request. Check the configured model's strict JSON Schema support."}
)

type composerProviderFailure struct {
	outcome string
	message string
}

func (failure *composerProviderFailure) Error() string { return failure.outcome }
func (failure *composerProviderFailure) Unwrap() error { return errComposerUpstream }

type ruleComposerProvider interface {
	interpret(ctx context.Context, scope, policy, instruction string, profile merchantPolicyProfile) ([]byte, error)
}

type openAIComposer struct {
	client         *http.Client
	endpoint       string
	apiKey         string
	model          string
	attemptTimeout time.Duration
	retryDelay     time.Duration
}

func configuredRuleComposer() (*openAIComposer, error) {
	enabled, err := strconv.ParseBool(env("PAYGUARD_RULE_COMPOSER_ENABLED", "false"))
	if err != nil {
		return nil, errors.New("PAYGUARD_RULE_COMPOSER_ENABLED must be a boolean")
	}
	if !enabled {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(env("PAYGUARD_RULE_COMPOSER_PROVIDER", "openai")), "openai") {
		return nil, errors.New("PAYGUARD_RULE_COMPOSER_PROVIDER must be openai")
	}
	model := strings.TrimSpace(os.Getenv("PAYGUARD_RULE_COMPOSER_MODEL"))
	if !validComposerModel(model) {
		return nil, errors.New("PAYGUARD_RULE_COMPOSER_MODEL must identify a direct OpenAI chat model")
	}
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if key == "" || !bookingTokenPattern.MatchString(key) {
		return nil, errors.New("OPENAI_API_KEY is missing or invalid")
	}
	return &openAIComposer{
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		endpoint: openAIChatEndpoint, apiKey: key, model: model,
		attemptTimeout: composerAttemptTimeout, retryDelay: 200 * time.Millisecond,
	}, nil
}

func validComposerModel(model string) bool {
	return composerModelID.MatchString(model) && model != "jev" && !strings.HasPrefix(model, "jev-")
}

type composerChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type composerSchema struct {
	Type                 string                    `json:"type,omitempty"`
	Properties           map[string]composerSchema `json:"properties,omitempty"`
	Required             []string                  `json:"required,omitempty"`
	AdditionalProperties *bool                     `json:"additionalProperties,omitempty"`
	Enum                 []string                  `json:"enum,omitempty"`
	Items                *composerSchema           `json:"items,omitempty"`
	AnyOf                []composerSchema          `json:"anyOf,omitempty"`
}

func composerObject(properties map[string]composerSchema, required ...string) composerSchema {
	strict := false
	return composerSchema{Type: "object", Properties: properties, Required: required, AdditionalProperties: &strict}
}

func composerEnum(values ...string) composerSchema {
	return composerSchema{Type: "string", Enum: values}
}

func composerList(item composerSchema) composerSchema {
	return composerSchema{Type: "array", Items: &item}
}

func merchantConditionSchema(profile merchantPolicyProfile) composerSchema {
	conditions := make([]composerSchema, 0, len(profile.Signals))
	for _, signal := range profile.Signals {
		if signal == policyMailbox {
			continue
		}
		field, known := merchantPolicyFields[signal]
		for _, capability := range profile.FieldRegistry {
			if capability.Name == signal {
				field, known = capability.policyField, true
				break
			}
		}
		if !known || len(field.Operators) == 0 {
			continue
		}
		value := composerSchema{Type: "string"}
		switch field.DataType {
		case "number":
			value.Type = "integer"
		case "boolean":
			value.Type = "boolean"
		}
		if signal == policyProduct && len(profile.Products) > 0 {
			value = composerEnum(profile.Products...)
		}
		conditions = append(conditions, composerObject(map[string]composerSchema{
			"field": composerEnum(signal), "operator": composerEnum(field.Operators...), "value": value,
		}, "field", "operator", "value"))
	}
	return composerSchema{AnyOf: conditions}
}

func proposalSchema(scope string, profile merchantPolicyProfile) composerSchema {
	var proposal composerSchema
	if scope == composerPlatform {
		signals := make([]string, 0, len(ruleSignals))
		for _, signal := range ruleSignals {
			signals = append(signals, signal.ID)
		}
		condition := composerObject(map[string]composerSchema{
			"signal":   composerEnum(signals...),
			"operator": composerEnum("eq", "ne", "gt", "gte", "lt", "lte"),
			"value":    {AnyOf: []composerSchema{{Type: "number"}, {Type: "boolean"}}},
		}, "signal", "operator", "value")
		rule := composerObject(map[string]composerSchema{
			"name":        {Type: "string"},
			"description": {Type: "string"},
			"stage":       composerEnum(allRuleStages...),
			"action":      composerEnum("REVIEW", "CHALLENGE"),
			"conditions":  composerList(condition),
		}, "name", "description", "stage", "action", "conditions")
		proposal = composerObject(map[string]composerSchema{
			"scope": composerEnum(composerPlatform), "rule": rule,
		}, "scope", "rule")
	} else {
		proposal = composerObject(map[string]composerSchema{
			"scope":         composerEnum(composerMerchant),
			"name":          {Type: "string"},
			"product_types": composerList(composerEnum(profile.Products...)),
			"stages":        composerList(composerEnum(profile.Stages...)),
			"conditions": composerObject(map[string]composerSchema{
				"logic": composerEnum("AND", "OR"), "rules": composerList(merchantConditionSchema(profile)),
			}, "logic", "rules"),
			"action": composerEnum("BLOCK", "REVIEW", "CHALLENGE"),
		}, "scope", "name", "product_types", "stages", "conditions", "action")
	}
	return composerObject(map[string]composerSchema{
		"status":   composerEnum("READY", "CLARIFICATION"),
		"message":  {Type: "string"},
		"proposal": {AnyOf: []composerSchema{proposal, {Type: "null"}}},
	}, "status", "message", "proposal")
}

type composerChatBody struct {
	Model               string                `json:"model"`
	Messages            []composerChatMessage `json:"messages"`
	Stream              bool                  `json:"stream"`
	Store               bool                  `json:"store"`
	MaxCompletionTokens int                   `json:"max_completion_tokens"`
	ResponseFormat      struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Name   string         `json:"name"`
			Strict bool           `json:"strict"`
			Schema composerSchema `json:"schema"`
		} `json:"json_schema"`
	} `json:"response_format"`
}

func (provider *openAIComposer) interpret(ctx context.Context, scope, policy, instruction string, profile merchantPolicyProfile) ([]byte, error) {
	body := composerChatBody{
		Model: provider.model, MaxCompletionTokens: 1400,
		Store:    false,
		Messages: []composerChatMessage{{Role: "system", Content: policy}, {Role: "user", Content: instruction}},
	}
	body.ResponseFormat.Type = "json_schema"
	body.ResponseFormat.JSONSchema.Name = "payguard_rule_proposal"
	body.ResponseFormat.JSONSchema.Strict = true
	body.ResponseFormat.JSONSchema.Schema = proposalSchema(scope, profile)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, errComposerUpstream
	}
	output, retry, err := provider.complete(ctx, payload)
	if !retry || ctx.Err() != nil {
		return output, err
	}
	timer := time.NewTimer(provider.retryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, composerTransportFailure(ctx.Err())
	case <-timer.C:
		output, _, err = provider.complete(ctx, payload)
		return output, err
	}
}

func (provider *openAIComposer) complete(ctx context.Context, payload []byte) ([]byte, bool, error) {
	attempt, cancel := context.WithTimeout(ctx, provider.attemptTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(attempt, http.MethodPost, provider.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, errComposerUpstream
	}
	request.Header.Set("Authorization", "Bearer "+provider.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, retryComposerError(err), composerTransportFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, false, composerProviderAuth
		case http.StatusNotFound:
			return nil, false, composerProviderRoute
		case http.StatusTooManyRequests:
			return nil, true, composerProviderRate
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return nil, true, composerProviderTimeout
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return nil, false, composerProviderRequest
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			return nil, true, errComposerUpstream
		default:
			return nil, false, errComposerUpstream
		}
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, composerResponseLimit+1))
	if err != nil {
		return nil, retryComposerError(err), composerTransportFailure(err)
	}
	if len(content) > composerResponseLimit {
		return nil, false, errComposerOutput
	}
	var completion struct {
		Error   *json.RawMessage `json:"error"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role    string `json:"role"`
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(content, &completion) != nil || completion.Error != nil || len(completion.Choices) != 1 {
		return nil, false, errComposerOutput
	}
	choice := completion.Choices[0]
	if choice.FinishReason != "stop" || choice.Message.Role != "assistant" || choice.Message.Refusal != "" ||
		len(choice.Message.Content) == 0 || len(choice.Message.Content) > composerOutputLimit {
		return nil, false, errComposerOutput
	}
	return []byte(choice.Message.Content), false, nil
}

func retryComposerError(err error) bool {
	return composerTransportFailure(err) == composerProviderTimeout ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func composerTransportFailure(err error) error {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return composerProviderTimeout
	}
	return errComposerUpstream
}
