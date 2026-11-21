package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const productionRBACOrigin = "https://payguard-rbac.test"
const productionRBACIssuer = "https://identity-rbac.test"

type productionRBACFixture struct {
	repo    repository
	auth    *productionAuth
	handler http.Handler
}

func newProductionRBACFixture(t *testing.T, mux *http.ServeMux) productionRBACFixture {
	t.Helper()
	db := openPolicyTestDatabase(t)
	auth := &productionAuth{store: productionAuthStore{db: db}, origin: productionRBACOrigin}
	auth.register(mux)
	return productionRBACFixture{repository{db}, auth, auth.protect(mux)}
}

type productionRBACAccount struct {
	actor   productionActor
	raw     string
	subject string
}

func (f productionRBACFixture) account(t *testing.T, role string, merchantID *string) productionRBACAccount {
	t.Helper()
	subject := randomAuthValue()
	if _, err := f.repo.db.Exec(t.Context(), `INSERT INTO production_auth_identities(issuer,subject,role,merchant_id)
        VALUES($1,$2,$3,$4::text::uuid)`, productionRBACIssuer, subject, role, merchantID); err != nil {
		t.Fatal(err)
	}
	return f.signIn(t, subject)
}

func (f productionRBACFixture) signIn(t *testing.T, subject string) productionRBACAccount {
	t.Helper()
	raw := randomAuthValue()
	actor, err := f.auth.store.issue(t.Context(), productionRBACIssuer, subject, authHash(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	return productionRBACAccount{actor, raw, subject}
}

func (f productionRBACFixture) request(t *testing.T, account productionRBACAccount, method, path string, body any, want int, result any) {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequestWithContext(t.Context(), method, productionRBACOrigin+path, &payload)
	if account.raw != "" {
		request.AddCookie(authCookie(productionSessionName, account.raw, productionSessionTTL))
	}
	if method != http.MethodGet {
		request.Header.Set("Origin", productionRBACOrigin)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s %s as %q: got %d, want %d: %s", method, path, account.actor.Role, response.Code, want, response.Body.String())
	}
	if result != nil {
		if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionRBACMerchantScopePostgres(t *testing.T) {
	mux := http.NewServeMux()
	f := newProductionRBACFixture(t, mux)
	repo := f.repo
	newMerchant := func(name string) mockMerchant {
		t.Helper()
		merchant, _, err := repo.createMockMerchant(t.Context(), mockMerchantInput{
			Name:              name,
			PolicyIntegration: &merchantPolicyIntegration{Products: []string{"flight_ticket"}, EmailDomain: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return merchant
	}
	first, second := newMerchant("Production RBAC merchant A"), newMerchant("Production RBAC merchant B")
	merchantA := f.account(t, productionRoleMerchant, &first.ID)
	merchantB := f.account(t, productionRoleMerchant, &second.ID)
	admin := f.account(t, productionRoleAdmin, nil)
	checker := f.account(t, productionRoleChecker, nil)

	registerMerchantDemo(mux, repo, "synthetic-sandbox-admin")
	registerJevAIConfiguration(mux, repo, "", merchantJev{})
	mux.HandleFunc("GET /api/v1/simulation/runs/{id}", productionRoles(app{repo: repo}.readRun, productionRoleAdmin))
	var sandboxAttempt = httptest.NewRequestWithContext(t.Context(), http.MethodGet, productionRBACOrigin+merchantDemoPath, nil)
	sandboxAttempt.Header.Set("Authorization", "Bearer synthetic-sandbox-admin")
	denied := httptest.NewRecorder()
	f.handler.ServeHTTP(denied, sandboxAttempt)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("production accepted a sandbox bearer: %d %s", denied.Code, denied.Body.String())
	}
	mux.HandleFunc("GET /api/v1/system/rules", productionRoles(app{repo: repo}.readRules, productionRoleAdmin, productionRoleChecker))
	mux.HandleFunc("POST /api/v1/system/rules", platformRuleWrite(repo, "", app{repo: repo}.createRule))

	getDirectory := func(actor productionRBACAccount) []mockMerchant {
		t.Helper()
		var directory struct {
			Merchants []mockMerchant
			Role      string
		}
		f.request(t, actor, http.MethodGet, merchantDemoPath, nil, http.StatusOK, &directory)
		return directory.Merchants
	}
	for _, fixture := range []struct {
		actor    productionRBACAccount
		merchant mockMerchant
	}{{merchantA, first}, {merchantB, second}} {
		directory := getDirectory(fixture.actor)
		if len(directory) != 1 || directory[0].ID != fixture.merchant.ID {
			t.Fatalf("expected only the assigned merchant, got %+v", directory)
		}
	}
	adminMerchants := make(map[string]bool)
	for _, merchant := range getDirectory(admin) {
		adminMerchants[merchant.ID] = true
	}
	if !adminMerchants[first.ID] || !adminMerchants[second.ID] {
		t.Fatal("platform directory omitted a merchant")
	}
	f.request(t, checker, http.MethodGet, merchantDemoPath, nil, http.StatusForbidden, nil)
	f.request(t, merchantA, http.MethodGet, merchantDemoPath+"/"+first.ID, nil, http.StatusOK, nil)
	f.request(t, merchantA, http.MethodGet, merchantDemoPath+"/"+second.ID, nil, http.StatusNotFound, nil)
	f.request(t, merchantA, http.MethodGet, merchantDemoPath+"/"+first.ID+"/transactions", nil, http.StatusOK, nil)
	f.request(t, merchantA, http.MethodGet, merchantDemoPath+"/"+second.ID+"/transactions", nil, http.StatusNotFound, nil)
	f.request(t, merchantA, http.MethodPost, merchantDemoPath, nil, http.StatusForbidden, nil)
	f.request(t, merchantA, http.MethodGet, "/api/v1/system/rules", nil, http.StatusForbidden, nil)
	f.request(t, merchantA, http.MethodPost, "/api/v1/system/rules", nil, http.StatusForbidden, nil)
	f.request(t, merchantA, http.MethodGet, jevAIConfigurationPath, nil, http.StatusForbidden, nil)
	f.request(t, merchantA, http.MethodGet, "/api/v1/simulation/runs/3a64b64b-c2cf-4141-b9cb-8a4255c87210", nil, http.StatusForbidden, nil)
	f.request(t, checker, http.MethodGet, "/api/v1/system/rules", nil, http.StatusOK, nil)

	type policyResponse struct {
		Rule merchantPolicyRule
	}
	var saved policyResponse
	f.request(t, merchantA, http.MethodPost, merchantPolicyPath+"/rules",
		map[string]any{"proposal": exampleMerchantPolicy()}, http.StatusCreated, &saved)
	if saved.Rule.ID == "" {
		t.Fatal("merchant policy creation did not return its ID")
	}
	f.request(t, merchantA, http.MethodGet, merchantPolicyPath+"/rules/"+saved.Rule.ID, nil, http.StatusOK, nil)
	f.request(t, merchantB, http.MethodGet, merchantPolicyPath+"/rules/"+saved.Rule.ID, nil, http.StatusNotFound, nil)

	if _, err := repo.db.Exec(t.Context(), "UPDATE merchant_demo_merchants SET deleted_at=now() WHERE id=$1::text::uuid", first.ID); err != nil {
		t.Fatal(err)
	}
	f.request(t, merchantA, http.MethodGet, merchantDemoPath, nil, http.StatusUnauthorized, nil)
}

func TestProductionRBACPlatformMakerCheckerPostgres(t *testing.T) {
	mux := http.NewServeMux()
	f := newProductionRBACFixture(t, mux)
	repo := f.repo
	registerPlatformRuleGovernance(mux, repo, "", "")
	maker := f.account(t, productionRoleAdmin, nil)
	reviewer := f.account(t, productionRoleChecker, nil)
	newRule := func() (systemRule, string) {
		t.Helper()
		input := ruleInput{Name: "Production governance fixture", Stage: "PAYMENT", Action: "REVIEW",
			Conditions: []ruleCondition{{Signal: "account_age_days", Operator: "gte", Value: float64(5)}}}
		draft, err := repo.createDraft(t.Context(), merchantID, input)
		if err != nil {
			t.Fatal(err)
		}
		var runID string
		if err := repo.db.QueryRow(t.Context(), "SELECT gen_random_uuid()::text").Scan(&runID); err != nil {
			t.Fatal(err)
		}
		_, err = repo.save(t.Context(), merchantID, run{ID: runID, RuleVersion: ruleVersion,
			Decisions: []decision{{Event: event{ID: "fixture-event", Stage: "PAYMENT", AgeDays: 7}}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return draft, runID
	}
	draft, runID := newRule()
	base := fmt.Sprintf("%s/platform/%s", composerPath, draft.ID)
	submission := map[string]any{"draftVersion": draft.Version, "runId": runID, "reason": "Submit quality check"}
	var pending platformRuleRevision
	f.request(t, reviewer, http.MethodPost, base+"/submit", submission, http.StatusForbidden, nil)
	f.request(t, maker, http.MethodPost, base+"/submit", submission, http.StatusCreated, &pending)
	if pending.ActorID == nil || *pending.ActorID != maker.actor.ID {
		t.Fatalf("maker identity was not recorded: %+v", pending)
	}
	approval := map[string]any{"version": pending.Version, "reason": "Reviewed quality check"}
	f.request(t, maker, http.MethodPost, base+"/approve", approval, http.StatusForbidden, nil)
	if _, err := repo.db.Exec(t.Context(), "UPDATE production_auth_identities SET role=$1 WHERE id=$2::text::uuid",
		productionRoleChecker, maker.actor.ID); err != nil {
		t.Fatal(err)
	}
	f.request(t, maker, http.MethodPost, base+"/approve", approval, http.StatusUnauthorized, nil)
	maker = f.signIn(t, maker.subject)
	f.request(t, maker, http.MethodPost, base+"/approve", approval, http.StatusForbidden, nil)
	var approved platformRuleRevision
	f.request(t, reviewer, http.MethodPost, base+"/approve", approval, http.StatusCreated, &approved)
	if approved.ActorID == nil || *approved.ActorID != reviewer.actor.ID || approved.SourceVersion == nil || *approved.SourceVersion != pending.Version {
		t.Fatalf("checker identity or source revision was not recorded: %+v", approved)
	}
	f.request(t, reviewer, http.MethodPost, base+"/approve", approval, http.StatusOK, nil)

	if err := initialize(t.Context(), repo.db); err != nil {
		t.Fatal(err)
	}
	persisted, err := readPlatformRevision(t.Context(), repo.db, "WHERE rule_id=$1 AND version=$2", draft.ID, approved.Version)
	if err != nil || persisted.ActorID == nil || *persisted.ActorID != reviewer.actor.ID {
		t.Fatalf("reinitialization changed governed history: %+v, %v", persisted, err)
	}
}
