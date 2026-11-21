package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const validPlatformComposition = `{
	"status":"READY","message":"","proposal":{"scope":"PLATFORM","rule":{
		"name":"Repeated device","description":"Review payment orders on a reused device.",
		"stage":"PAYMENT","action":"REVIEW","conditions":[{"signal":"device_orders","operator":"gte","value":3}]
	}}
}`

const validThaiComposition = `{
	"status":"READY","message":"","proposal":{
		"scope":"MERCHANT","name":"บล็อกการจองตั๋วตามโดเมน",
		"product_types":["flight_ticket"],"stages":["BOOKING"],"action":"BLOCK",
		"conditions":{"logic":"AND","rules":[{"field":"customer.email_domain","operator":"eq","value":"Example.TEST."}]}
	}
}`

func TestComposerInterpretsStructuredProposals(t *testing.T) {
	platform, err := parseComposerInterpretation([]byte(validPlatformComposition), composerPlatform, merchantPolicyProfile{})
	if err != nil || !platform.Validation.Valid || platform.Status != "READY" {
		t.Fatalf("English platform proposal: %+v, %v", platform, err)
	}
	profile := merchantPolicyProfile{
		Products: []string{"flight_ticket"}, Stages: []string{"BOOKING"},
		Signals: []string{policyDomain, policyProduct},
	}
	merchant, err := parseComposerInterpretation([]byte(validThaiComposition), composerMerchant, profile)
	if err != nil || !merchant.Validation.Valid || merchant.Status != "READY" {
		t.Fatalf("Thai merchant proposal: %+v, %v", merchant, err)
	}
	proposal := merchant.Proposal.(merchantPolicyProposal)
	if proposal.Name != "บล็อกการจองตั๋วตามโดเมน" || string(proposal.Conditions.Rules[0].Value) != `"example.test"` {
		t.Fatalf("unexpected normalization: %+v", proposal)
	}
}

func TestComposerRejectsUnexpectedStructuredOutput(t *testing.T) {
	for _, testCase := range []struct {
		name, output string
	}{
		{"unknown top level", strings.Replace(validPlatformComposition, `"status":"READY"`, `"status":"READY","approved":true`, 1)},
		{"unknown condition field", strings.Replace(validPlatformComposition, `"value":3`, `"value":3,"bypass":true`, 1)},
		{"cross scope", strings.Replace(validPlatformComposition, `"scope":"PLATFORM"`, `"scope":"MERCHANT"`, 1)},
		{"multiple replies", validPlatformComposition + `{}`},
		{"missing message", strings.Replace(validPlatformComposition, `"message":"",`, "", 1)},
		{"proposal with clarification", strings.Replace(validPlatformComposition, `"status":"READY"`, `"status":"CLARIFICATION"`, 1)},
		{"markdown output", "```json\n" + validPlatformComposition + "\n```"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := parseComposerInterpretation([]byte(testCase.output), composerPlatform, merchantPolicyProfile{}); !errors.Is(err, errComposerOutput) {
				t.Fatalf("expected invalid output, got %v", err)
			}
		})
	}
}

func TestComposerRequestsClarificationForUnsupportedRule(t *testing.T) {
	invalid := strings.Replace(validPlatformComposition, `"action":"REVIEW"`, `"action":"BLOCK"`, 1)
	output, err := parseComposerInterpretation([]byte(invalid), composerPlatform, merchantPolicyProfile{})
	if err != nil || output.Status != "CLARIFICATION" || output.Proposal != nil || len(output.Validation.Errors) != 1 {
		t.Fatalf("expected an actionable validation error: %+v, %v", output, err)
	}
	clarification := `{"status":"CLARIFICATION","message":"ต้องการกำหนดยอดขั้นต่ำเท่าไร","proposal":null}`
	output, err = parseComposerInterpretation([]byte(clarification), composerMerchant, merchantPolicyProfile{})
	if err != nil || output.Status != "CLARIFICATION" || !strings.Contains(output.Message, "ยอดขั้นต่ำ") || output.Proposal != nil {
		t.Fatalf("expected a localized clarification: %+v, %v", output, err)
	}
}

func TestSensitiveComposerText(t *testing.T) {
	for _, input := range []string{
		"Review demo.person@example.test", "Handle demo+label@ตัวอย่าง.test", "Look up 198.51.100.7",
		"Filter 2001:db8::1", "Use the synthetic PAN 4242-4242-4242-4242", "Bearer demo-merchant-token",
		"Review customer*@example.test",
		"Use sk-or-v1-fake-secret-for-tests", "Use sk-proj-fake-secret-for-tests", "Use skey_test_fake-secret",
		"CVV: 123", "CVC is 8877", "card security code equals 432",
		`{"cvv2":"045"}`, "รหัสความปลอดภัยคือ 123",
	} {
		if !sensitiveComposerText(input) {
			t.Errorf("did not detect sensitive pattern in %q", input)
		}
	}
	for _, input := range []string{
		"Block flight bookings for example.test", "Block @example.test", "Review 3 orders in 60 seconds",
		"บล็อกการจองเที่ยวบินของ *@skycannner.com",
		"ตรวจสอบยอด 5000 บาท", "Review 12.50 THB",
		"Review a payment amount of 123 satang", "Review repeated security code failures",
	} {
		if sensitiveComposerText(input) {
			t.Errorf("unexpected rejection of %q", input)
		}
	}
}

type testComposerProvider func(context.Context, string, string, string) ([]byte, error)

func (provider testComposerProvider) interpret(ctx context.Context, scope, policy, instruction string) ([]byte, error) {
	return provider(ctx, scope, policy, instruction)
}

func TestComposerAuthorizationAndEgress(t *testing.T) {
	calls := 0
	composer := ruleComposer{
		access: composerAuthorization(repository{}, "test-admin-secret"),
		provider: testComposerProvider(func(_ context.Context, scope, policy, instruction string) ([]byte, error) {
			calls++
			if scope != composerPlatform || !strings.Contains(policy, "device_orders") || instruction != "Review payment when a device has 3 orders." {
				t.Error("unexpected platform instruction or capability catalog")
			}
			return []byte(validPlatformComposition), nil
		}),
	}
	for _, testCase := range []struct {
		name, auth, body string
		want             int
	}{
		{"missing credential", "", `{"scope":"PLATFORM","instruction":"Review payments."}`, 401},
		{"admin merchant scope", "Bearer test-admin-secret", `{"scope":"MERCHANT","instruction":"Block flight bookings."}`, 403},
		{"sensitive data", "Bearer test-admin-secret", `{"scope":"PLATFORM","instruction":"Review demo.person@example.test."}`, 422},
		{"unknown input", "Bearer test-admin-secret", `{"scope":"PLATFORM","instruction":"Review.","merchantId":"spoof"}`, 400},
		{"authorized", "Bearer test-admin-secret", `{"scope":"PLATFORM","instruction":"Review payment when a device has 3 orders."}`, 200},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, composerPath+"/interpret", strings.NewReader(testCase.body))
			if testCase.auth != "" {
				request.Header.Set("Authorization", testCase.auth)
			}
			response := httptest.NewRecorder()
			composer.interpret(response, request)
			if response.Code != testCase.want {
				t.Fatalf("got HTTP %d, want %d: %s", response.Code, testCase.want, response.Body.String())
			}
		})
	}
	if calls != 1 {
		t.Fatalf("provider called %d times", calls)
	}
}

func TestPlatformRuleWriteRequiresAdmin(t *testing.T) {
	next := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	handler := platformRuleWrite(repository{}, "test-admin-secret", next)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/rules", nil)
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("unexpected unauthorized response: %d", response.Code)
	}
	request.Header.Set("Authorization", "Bearer test-admin-secret")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("admin token rejected with status %d", response.Code)
	}
}

func TestOpenAIComposerRequestsStrictSchemaAndRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			composerChatBody
			Store *bool `json:"store"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-openai-key" ||
			r.Header.Get("Content-Type") != "application/json" || decoder.Decode(&body) != nil {
			t.Error("expected authorized JSON POST")
		}
		if body.Model != "gpt-4.1-mini" || body.Stream || body.Store == nil || *body.Store ||
			body.ResponseFormat.Type != "json_schema" || !body.ResponseFormat.JSONSchema.Strict {
			t.Errorf("invalid structured output configuration: %+v", body.ResponseFormat)
		}
		schema := body.ResponseFormat.JSONSchema.Schema
		if schema.AdditionalProperties == nil || *schema.AdditionalProperties || len(schema.Required) != 3 {
			t.Errorf("expected closed schema with required fields: %+v", schema)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Content != "ตรวจสอบการจอง" {
			t.Error("unexpected provider prompt roles or translation")
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeComposerCompletion(w, validPlatformComposition, "stop")
	}))
	defer server.Close()

	provider := openAIComposer{client: server.Client(), endpoint: server.URL, apiKey: "test-openai-key",
		model: "gpt-4.1-mini", attemptTimeout: time.Second}
	result, err := provider.interpret(t.Context(), composerPlatform, "Allowed fields only", "ตรวจสอบการจอง")
	if err != nil || string(result) != validPlatformComposition || calls.Load() != 2 {
		t.Fatalf("expected successful retry, calls=%d error=%v", calls.Load(), err)
	}
}

func writeComposerCompletion(w http.ResponseWriter, content, finishReason string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"finish_reason": finishReason, "message": map[string]string{"role": "assistant", "content": content}}},
	})
}

func TestOpenAIComposerFailureHandling(t *testing.T) {
	for _, testCase := range []struct {
		name, content, finishReason string
		status, calls               int
		want                        error
	}{
		{"credentials", "", "", http.StatusUnauthorized, 1, composerProviderAuth},
		{"account policy", "", "", http.StatusForbidden, 1, composerProviderAuth},
		{"provider route", "", "", http.StatusNotFound, 1, composerProviderRoute},
		{"unsupported schema", "", "", http.StatusBadRequest, 1, composerProviderRequest},
		{"transient exhausted", "", "", http.StatusTooManyRequests, 2, composerProviderRate},
		{"timed out", "", "", http.StatusGatewayTimeout, 2, composerProviderTimeout},
		{"truncated", "{", "length", http.StatusOK, 1, errComposerOutput},
		{"wrong body", "", "", http.StatusOK, 1, errComposerOutput},
		{"oversized content", strings.Repeat("a", composerOutputLimit+1), "stop", http.StatusOK, 1, errComposerOutput},
		{"oversized envelope", strings.Repeat("a", composerResponseLimit+1), "stop", http.StatusOK, 1, errComposerOutput},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if testCase.status != http.StatusOK {
					w.WriteHeader(testCase.status)
					return
				}
				writeComposerCompletion(w, testCase.content, testCase.finishReason)
			}))
			defer server.Close()
			provider := openAIComposer{client: server.Client(), endpoint: server.URL, attemptTimeout: time.Second}
			result, err := provider.interpret(t.Context(), composerPlatform, "System", "Instruction")
			if len(result) != 0 || !errors.Is(err, testCase.want) || calls.Load() != int32(testCase.calls) {
				t.Fatalf("calls=%d error=%v output length=%d", calls.Load(), err, len(result))
			}
		})
	}
}

func TestConfiguredRuleComposerUsesOnlyDirectOpenAI(t *testing.T) {
	t.Setenv("PAYGUARD_RULE_COMPOSER_ENABLED", "false")
	t.Setenv("PAYGUARD_RULE_COMPOSER_PROVIDER", "openai")
	t.Setenv("PAYGUARD_RULE_COMPOSER_MODEL", "~typesafe/jev-latest")
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("OPENROUTER_API_KEY", "test-router-key")
	if provider, err := configuredRuleComposer(); provider != nil || err != nil {
		t.Fatalf("disabled configuration was not ignored: %+v %v", provider, err)
	}
	t.Setenv("PAYGUARD_RULE_COMPOSER_ENABLED", "true")
	if provider, err := configuredRuleComposer(); provider != nil || err == nil {
		t.Fatal("a Jev decision model was accepted for authoring")
	}
	t.Setenv("PAYGUARD_RULE_COMPOSER_MODEL", "gpt-4.1-mini")
	if provider, err := configuredRuleComposer(); err != nil || provider == nil ||
		provider.endpoint != "https://api.openai.com/v1/chat/completions" || provider.apiKey != "test-openai-key" {
		t.Fatalf("failed to configure the structured chat model: %v", err)
	}
	t.Setenv("OPENAI_API_KEY", "")
	if provider, err := configuredRuleComposer(); provider != nil || err == nil {
		t.Fatal("composer fell back to the OpenRouter API key")
	}
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("PAYGUARD_RULE_COMPOSER_PROVIDER", "openrouter")
	if provider, err := configuredRuleComposer(); provider != nil || err == nil {
		t.Fatal("OpenRouter is selectable for authoring")
	}
}
