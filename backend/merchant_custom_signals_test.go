package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestCustomSignalValues(t *testing.T) {
	for _, test := range []struct {
		kind, value string
		valid       bool
	}{
		{"country", `"IN"`, true}, {"country", `"00"`, false}, {"country", `"in"`, false},
		{"text", `"Hong Kong Disneyland"`, true}, {"text", `"buyer@example.test"`, false},
		{"boolean", "false", true}, {"boolean", `"true"`, false}, {"number", "0", true}, {"number", "1.5", false},
	} {
		if got := validCustomValue(test.kind, json.RawMessage(test.value)); got != test.valid {
			t.Fatalf("%s %s returned %v", test.kind, test.value, got)
		}
	}
}

func TestManagedCustomPolicyPostgres(t *testing.T) {
	repo := repository{openPolicyTestDatabase(t)}
	admin := "local-admin-structured-rules"
	registration := mockMerchantInput{Name: "Custom field shop", PolicyIntegration: &merchantPolicyIntegration{Products: []string{"physical"},
		CustomFields: []customSignalDefinition{{"custom_title", "Ticket title", "text"}, {"custom_country", "Origin country", "country"}}}}
	merchant, token, err := repo.createMockMerchant(t.Context(), registration)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := repo.createMockMerchant(t.Context(), mockMerchantInput{Name: "Other shop"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerMerchantDemo(mux, repo, admin)
	call := func(method, path, credential string, input any, wanted int, output any) {
		t.Helper()
		buf := new(bytes.Buffer)
		if input != nil {
			if err := json.NewEncoder(buf).Encode(input); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, buf)
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		got := httptest.NewRecorder()
		mux.ServeHTTP(got, req)
		if got.Code != wanted {
			t.Fatalf("%s %s returned %d, wanted %d: %s", method, path, got.Code, wanted, got.Body.String())
		}
		if output != nil {
			if err := json.Unmarshal(got.Body.Bytes(), output); err != nil {
				t.Fatal(err)
			}
		}
	}
	path := merchantDemoPath + "/" + merchant.ID
	var profile merchantPolicyProfile
	call("GET", path+"/policy-profile", admin, nil, 200, &profile)
	if !slices.Contains(profile.Signals, "custom_title") {
		t.Fatal("registered field missing")
	}
	call("GET", merchantDemoPath+"/"+second.ID+"/policy-profile", token, nil, 403, nil)
	proposal := merchantPolicyProposal{Scope: "MERCHANT", Name: "Ticket country block", ProductTypes: []string{"physical"}, Stages: []string{"CHECKOUT"}, Action: "BLOCK",
		Conditions: merchantPolicyConditions{Logic: "AND", Rules: []merchantPolicyCondition{{"custom_title", "contains", json.RawMessage(`"disney"`)}, {"custom_country", "eq", json.RawMessage(`"IN"`)}}}}
	var saved struct {
		Rule merchantPolicyRule `json:"rule"`
	}
	call("POST", path+"/policies", token, map[string]any{"proposal": proposal}, 403, nil)
	call("POST", path+"/policies", admin, map[string]any{"proposal": proposal}, 201, &saved)
	if saved.Rule.CreatedBy != platformMaker || saved.Rule.Status != "ACTIVE" {
		t.Fatalf("admin author/status: %s/%s", saved.Rule.CreatedBy, saved.Rule.Status)
	}
	rule := path + "/policies/" + saved.Rule.ID
	proposal.Name = "Ticket and originating country"
	call("PUT", rule, admin, map[string]any{"proposal": proposal, "expectedVersion": 1}, 200, &saved)
	amount := int64(100)
	input := mockTransactionInput{ID: "8837788d-a276-41ad-8c42-ee97783d2f45", Source: "checkout", Product: "physical", AmountMinor: &amount, CustomSignals: map[string]json.RawMessage{
		"custom_title": json.RawMessage(`"Hong Kong DISNEYLAND"`), "custom_country": json.RawMessage(`"IN"`)}}
	var savedTx struct {
		Transaction mockTransaction `json:"transaction"`
	}
	call("POST", path+"/transactions", admin, input, 201, &savedTx)
	if savedTx.Transaction.Outcome != "HIGH_RISK" || savedTx.Transaction.PolicyDecision.Merchant.RuleVersions[saved.Rule.ID] != 2 {
		t.Fatalf("rule version did not block transaction: %+v", savedTx.Transaction.PolicyDecision)
	}
	input.ID = "8837788d-a276-41ad-8c42-ee97783d2f46"
	input.Source = "simulation"
	delete(input.CustomSignals, "custom_country")
	call("POST", path+"/transactions", admin, input, 201, &savedTx)
	if !slices.Contains(savedTx.Transaction.PolicyDecision.Merchant.MissingSignals, "custom_country") || savedTx.Transaction.Outcome == "HIGH_RISK" {
		t.Fatal("missing signal matched")
	}
	input.ID = "8837788d-a276-41ad-8c42-ee97783d2f47"
	input.CustomSignals["custom_country"] = json.RawMessage("42")
	call("POST", path+"/transactions", admin, input, 422, nil)
	registration.PolicyIntegration.CustomFields[0].Type = "number"
	call("PUT", path, admin, registration, 422, nil)
	call("DELETE", rule, admin, map[string]int{"expectedVersion": 1}, 409, nil)
	call("DELETE", rule, admin, map[string]int{"expectedVersion": 2}, 200, nil)
	var list struct {
		Rules []merchantPolicyRule `json:"rules"`
	}
	call("GET", merchantPolicyPath+"/rules", token, nil, 200, &list)
	if len(list.Rules) != 0 {
		t.Fatal("deleted policy still visible")
	}
	call("GET", path+"/transactions/8837788d-a276-41ad-8c42-ee97783d2f45", token, nil, 200, &savedTx)
	if savedTx.Transaction.Outcome != "HIGH_RISK" {
		t.Fatal("past transaction changed")
	}
}
