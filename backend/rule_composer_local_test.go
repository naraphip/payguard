package main

import (
	"context"
	"encoding/json"
	"errors"
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

func TestManagedAuthoringWithoutComposerPostgres(t *testing.T) {
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
	call := func(instruction string) composerInterpretation {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"scope": "MERCHANT", "instruction": instruction})
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
	if result := call("Block physical goods when email domain equals example.test"); result.Status != "READY" || !result.Validation.Valid {
		t.Fatalf("local policy draft: %+v", result)
	}
	const customInstruction = "Block ticket ที่เป็น *disney* land hongkong และ ip มาจาก india"
	originalDraft := call(customInstruction)
	if originalDraft.Status != "READY" || !originalDraft.Validation.Valid || originalDraft.SemanticDraft != nil {
		t.Fatalf("eligible unsupported instruction should produce a directly savable draft: %+v", originalDraft)
	}
	if result := call("Block theme park tickets for 2 min"); result.Status != "CLARIFICATION" || result.Proposal != nil {
		t.Fatalf("unsupported expiry should require clarification: %+v", result)
	}
	if result := call(`{"scope":"MERCHANT","unknownField":"Block theme park tickets"}`); result.Status != "CLARIFICATION" || result.Proposal != nil {
		t.Fatalf("invalid JSON must remain a validation error: %+v", result)
	}
	providerCases := []struct {
		name, payload string
		err           error
	}{
		{name: "provider unavailable", err: errors.New("test provider timeout")},
		{name: "provider invalid response", payload: `{"status":"READY"}`},
		{name: "provider clarification", payload: `{"status":"CLARIFICATION","message":"Country field is unavailable","proposal":null}`},
	}
	for _, test := range providerCases {
		t.Run(test.name, func(t *testing.T) {
			composer.provider = testComposerProvider(func(_ context.Context, scope, _, instruction string) ([]byte, error) {
				if scope != composerMerchant || instruction != customInstruction {
					t.Errorf("unexpected provider input %s %q", scope, instruction)
				}
				return []byte(test.payload), test.err
			})
			result := call(customInstruction)
			if after, _ := json.Marshal(result.Proposal); result.Status != "READY" || string(after) != proposalJSON(t, originalDraft) {
				t.Errorf("draft should use the original instruction: %+v", result)
			}
			if native := call("Block physical goods when amount is at least 100 minor units and email domain equals example.test"); native.Status != "READY" ||
				strings.Contains(proposalJSON(t, native), semanticOperator) {
				t.Errorf("supported conditions should remain local: %+v", native)
			}
		})
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
