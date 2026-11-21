package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func localComposerTestProfile() merchantPolicyProfile {
	return merchantPolicyProfile{
		Products: []string{"physical", "flight_ticket"},
		Stages:   []string{"BOOKING", "CHECKOUT"},
		Signals:  []string{policyProduct, policyAmount, policyDomain, policyMailbox, policyDescription, "custom_ticket_title", "custom_refundable"},
		FieldRegistry: []merchantCapability{
			{Name: policyDescription, policyField: merchantPolicyFields[policyDescription], Products: []string{"physical", "flight_ticket"}},
			{Name: "custom_ticket_title", policyField: policyField{DataType: "text", Operators: []string{"eq", "contains", semanticOperator}}, Products: []string{"physical"}},
			{Name: "custom_refundable", policyField: policyField{DataType: "boolean", Operators: []string{"eq", "ne"}}, Products: []string{"physical"}},
		},
	}
}

func TestLocalMerchantRuleAuthoring(t *testing.T) {
	for _, tc := range []struct {
		instruction, action, field, operator, value string
	}{
		{"Review all products when amount is at least 50000 minor units", "REVIEW", policyAmount, "gte", "50000"},
		{"ส่งตรวจสอบสินค้าทั่วไปเมื่อยอดอย่างน้อย 12.50 บาท", "REVIEW", policyAmount, "gte", "1250"},
		{"Block physical goods when email domain equals *@Example.TEST.", "BLOCK", policyDomain, "eq", `"example.test"`},
		{"If a customer books a flight ticket using an email address from skycannner.com, block the booking.", "BLOCK", policyDomain, "eq", `"skycannner.com"`},
		{`Review physical goods when transaction description means "Hong Kong Disneyland admission ticket"`, "REVIEW", policyDescription, semanticOperator, `"Hong Kong Disneyland admission ticket"`},
		{`บล็อกสินค้าทั่วไปเมื่อรายละเอียดธุรกรรมหมายถึง "ตั๋วดิสนีย์แลนด์ฮ่องกง"`, "BLOCK", policyDescription, semanticOperator, `"ตั๋วดิสนีย์แลนด์ฮ่องกง"`},
		{`Challenge physical goods when custom_ticket_title means "Disneyland admission"`, "CHALLENGE", "custom_ticket_title", semanticOperator, `"Disneyland admission"`},
	} {
		t.Run(tc.instruction, func(t *testing.T) {
			out, handled := interpretLocalMerchantRule(tc.instruction, localComposerTestProfile())
			proposal, ready := out.Proposal.(merchantPolicyProposal)
			if !handled || !ready || out.Status != "READY" || !out.Validation.Valid || len(proposal.Conditions.Rules) != 1 {
				t.Fatalf("expected a validated draft, got %+v", out)
			}
			condition := proposal.Conditions.Rules[0]
			if proposal.Action != tc.action || condition.Field != tc.field || condition.Operator != tc.operator || string(condition.Value) != tc.value {
				t.Fatalf("unexpected condition %v on rule with action %s", condition, proposal.Action)
			}
		})
	}

	for _, instruction := range []string{
		"Review physical goods when amount is at least 100",
		"Review physical goods when amount is at least 100 minor units and email domain equals example.test",
		"Review when amount is at least 100 minor units",
		"Review physical goods when amount is at least 12.34 minor units",
		`Block physical goods from India when transaction description means "Disneyland"`,
		`Block physical goods when transaction description means "Disneyland" or the country is India`,
	} {
		if draft, handled := interpretLocalMerchantRule(instruction, localComposerTestProfile()); handled {
			t.Fatalf("ambiguous instruction %q was interpreted as %+v", instruction, draft)
		}
	}
}

const mixedMerchantComposition = `{
	"scope":"MERCHANT","name":"Ticket meaning and refundability","product_types":["physical"],"stages":["CHECKOUT"],"action":"BLOCK",
	"conditions":{"logic":"AND","rules":[
		{"field":"custom_ticket_title","operator":"semantic_match","value":"Hong Kong Disneyland admission ticket"},
		{"field":"custom_refundable","operator":"eq","value":false}
	]}
}`

func TestComposerMerchantSchemaRoutesSupportedFieldTypes(t *testing.T) {
	profile := localComposerTestProfile()
	schema := proposalSchema(composerMerchant, profile)
	proposal := schema.Properties["proposal"].AnyOf[0].Properties
	options := proposal["conditions"].Properties["rules"].Items.AnyOf
	catalog := make(map[string]composerSchema, len(options))
	for _, option := range options {
		catalog[option.Properties["field"].Enum[0]] = option
	}
	if got := proposal["product_types"].Items.Enum; !slices.Equal(got, profile.Products) {
		t.Fatalf("product schema: %v", got)
	}
	if _, ok := catalog[policyMailbox]; ok {
		t.Fatal("mailbox reference must stay outside the composer schema")
	}
	title := catalog["custom_ticket_title"].Properties
	refundable := catalog["custom_refundable"].Properties
	if !slices.Contains(title["operator"].Enum, semanticOperator) || !slices.Contains(title["operator"].Enum, "contains") ||
		title["value"].Type != "string" || refundable["value"].Type != "boolean" {
		t.Fatal("custom field schema did not preserve field operators and types")
	}

	remote := []byte(`{"status":"READY","message":"","proposal":` + mixedMerchantComposition + `}`)
	result, err := parseComposerInterpretation(remote, composerMerchant, profile)
	local, handled := interpretLocalMerchantRule(mixedMerchantComposition, profile)
	if err != nil || !result.Validation.Valid || !handled || !local.Validation.Valid {
		t.Fatalf("mixed rule should pass the same validation in both paths: remote=%+v local=%+v err=%v", result, local, err)
	}
	for _, invalid := range []string{
		strings.Replace(mixedMerchantComposition, "custom_ticket_title", "custom_undeclared", 1),
		strings.Replace(mixedMerchantComposition, "custom_ticket_title", "custom_refundable", 1),
		strings.Replace(mixedMerchantComposition, `"Hong Kong Disneyland admission ticket"`, `"raw 198.51.100.7"`, 1),
		strings.Replace(mixedMerchantComposition, `"action":"BLOCK"`, `"action":"BLOCK","durationMinutes":2`, 1),
		mixedMerchantComposition + "{}",
	} {
		result, handled := interpretLocalMerchantRule(invalid, profile)
		if !handled || result.Status != "CLARIFICATION" || result.Proposal != nil {
			t.Fatalf("invalid draft was accepted: %+v", result)
		}
	}
}

func TestManagedComposerRoutingPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	merchant, _, err := repo.createMockMerchant(t.Context(), mockMerchantInput{
		Name:              "Standalone local authoring",
		PolicyIntegration: &merchantPolicyIntegration{Products: []string{"physical"}, EmailDomain: true, AmountMinor: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	const admin = "standalone-authoring-test-admin"
	composer := ruleComposer{access: composerAuthorization(repo, admin)}
	endpoint := merchantDemoPath + "/" + merchant.ID + "/policy-interpret"
	mux.HandleFunc("POST "+merchantDemoPath+"/{merchantId}/policy-interpret", func(w http.ResponseWriter, r *http.Request) {
		composer.interpret(w, r)
	})
	call := func(t *testing.T, payload map[string]any) composerInterpretation {
		t.Helper()
		payload["scope"] = composerMerchant
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+admin)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
		}
		var result composerInterpretation
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	interpret := func(t *testing.T, instruction string) composerInterpretation {
		t.Helper()
		return call(t, map[string]any{"instruction": instruction})
	}
	const domainInstruction = "Block physical goods when email domain equals example.test"
	if result := interpret(t, domainInstruction); result.Status != "READY" || !result.Validation.Valid ||
		result.Source != composerSourceLocalFallback || !strings.Contains(result.Message, "not configured") || result.Model != "" {
		t.Fatalf("local policy draft: %+v", result)
	}
	const customInstruction = "Block ticket ที่เป็น *disney* land hongkong และ ip มาจาก india"
	originalDraft := interpret(t, customInstruction)
	if originalDraft.Status != "READY" || !originalDraft.Validation.Valid || originalDraft.SemanticDraft != nil {
		t.Fatalf("eligible unsupported instruction should produce a directly savable draft: %+v", originalDraft)
	}
	if result := interpret(t, "Block theme park tickets for 2 min"); result.Status != "CLARIFICATION" || result.Proposal != nil ||
		result.Source != composerSourceLocal {
		t.Fatalf("unsupported expiry should require clarification: %+v", result)
	}
	if result := interpret(t, `{"scope":"MERCHANT","unknownField":"Block theme park tickets"}`); result.Status != "CLARIFICATION" || result.Proposal != nil ||
		result.Source != composerSourceLocal {
		t.Fatalf("invalid JSON must remain a validation error: %+v", result)
	}

	const domainDraft = `{"status":"READY","message":"","proposal":{"scope":"MERCHANT","name":"OpenAI domain draft","product_types":["physical"],"stages":["CHECKOUT"],"conditions":{"logic":"AND","rules":[{"field":"customer.email_domain","operator":"eq","value":"example.test"}]},"action":"BLOCK"}}`
	const thaiDraft = `{"status":"READY","message":"","proposal":{"scope":"MERCHANT","name":"ตรวจสอบยอดสินค้าจริง","product_types":["physical"],"stages":["CHECKOUT"],"conditions":{"logic":"AND","rules":[{"field":"order.amount_minor","operator":"gte","value":50000}]},"action":"REVIEW"}}`
	const emailRisk = "บล็อกสินค้าทั่วไปเมื่อเป็นอีเมลชั่วคราวหรือชื่อก่อน @ ดูสุ่ม"
	const emailDraft = `{"status":"READY","message":"","proposal":{"scope":"MERCHANT","name":"Email risk","product_types":["physical"],"stages":["CHECKOUT"],"conditions":{"logic":"OR","rules":[{"field":"customer.email_local_part_random","operator":"eq","value":true},{"field":"customer.email_disposable","operator":"eq","value":true}]},"action":"BLOCK"}}`
	composer.model = "gpt-4.1-mini"
	for _, test := range []struct {
		name, instruction, response, source, status, detail string
		err                                                 error
	}{
		{name: "native rule reaches model", instruction: domainInstruction, response: domainDraft, source: composerSourceOpenAI, status: "READY", detail: "OpenAI domain draft"},
		{name: "natural Thai reaches model", instruction: "ส่งรายการสินค้าจริงให้ตรวจสอบเมื่อยอดตั้งแต่ 50000 หน่วยย่อย", response: thaiDraft, source: composerSourceOpenAI, status: "READY", detail: "ตรวจสอบยอดสินค้าจริง"},
		{name: "Thai email risk reaches model", instruction: emailRisk, response: emailDraft, source: composerSourceOpenAI, status: "READY", detail: policyRandomEmail},
		{name: "email conditions cannot be dropped", instruction: emailRisk, response: strings.Replace(emailDraft, `{"field":"customer.email_disposable","operator":"eq","value":true}`, `{"field":"product.type","operator":"eq","value":"physical"}`, 1), source: composerSourceLocalFallback, status: "READY", detail: "did not pass PayGuard validation"},
		{name: "provider unavailable", instruction: customInstruction, err: composerProviderTimeout, source: composerSourceLocalFallback, status: "READY", detail: "timed out"},
		{name: "provider invalid response", instruction: customInstruction, response: `{"status":"READY"}`, source: composerSourceLocalFallback, status: "READY", detail: "did not pass PayGuard validation"},
		{name: "provider clarification takes precedence", instruction: customInstruction, response: `{"status":"CLARIFICATION","message":"Country field is unavailable","proposal":null}`, source: composerSourceOpenAI, status: "CLARIFICATION", detail: "Country field is unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			composer.provider = testComposerProvider(func(_ context.Context, scope, _, instruction string) ([]byte, error) {
				calls++
				if scope != composerMerchant || instruction != test.instruction {
					t.Errorf("unexpected provider input %s %q", scope, instruction)
				}
				return []byte(test.response), test.err
			})
			result := interpret(t, test.instruction)
			if calls != 1 || result.Source != test.source || result.Status != test.status ||
				!strings.Contains(result.Message+proposalJSON(t, result), test.detail) {
				t.Fatalf("expected %s from %s in one provider call, got %+v with %d calls", test.status, test.source, result, calls)
			}
			wantModel := ""
			if test.source == composerSourceOpenAI {
				wantModel = composer.model
			}
			if result.Model != wantModel || (result.Status == "CLARIFICATION" && result.Proposal != nil) {
				t.Fatalf("incorrect model provenance or clarification: %+v", result)
			}
			if test.source == composerSourceLocalFallback && test.instruction == customInstruction &&
				proposalJSON(t, result) != proposalJSON(t, originalDraft) {
				t.Fatalf("fallback changed the original local draft: %+v", result)
			}
		})
	}

	composer.provider = testComposerProvider(func(context.Context, string, string, string) ([]byte, error) {
		t.Error("private or structured authoring reached OpenAI")
		return nil, nil
	})
	if result := call(t, map[string]any{"mailboxBlock": map[string]any{
		"mailboxDigest": strings.Repeat("a", 64), "durationMinutes": 2,
	}}); result.Status != "READY" || result.Source != composerSourceLocal || result.Model != "" {
		t.Fatalf("exact mailbox draft must stay local: %+v", result)
	}
	if result := interpret(t, `{"scope":"MERCHANT","unknownField":"Block theme park tickets"}`); result.Status != "CLARIFICATION" ||
		result.Source != composerSourceLocal {
		t.Fatalf("explicit structured text must be checked locally: %+v", result)
	}
	if result := interpret(t, "Block physical goods when the email is disposable for two minutes"); result.Status != "CLARIFICATION" ||
		result.Source != composerSourceLocal {
		t.Fatalf("a temporary email-risk rule must request clarification: %+v", result)
	}
}

func proposalJSON(t *testing.T, result composerInterpretation) string {
	t.Helper()
	data, err := json.Marshal(result.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
