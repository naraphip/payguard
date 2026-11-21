package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMailboxBlockCannotBeInterpretedByProvider(t *testing.T) {
	profile := merchantPolicyProfile{
		Signals: []string{policyMailbox, policyDomain},
		FieldRegistry: []merchantCapability{
			{Name: policyMailbox, policyField: merchantPolicyFields[policyMailbox]},
			{Name: policyDomain, policyField: merchantPolicyFields[policyDomain]},
		},
	}
	if prompt := composerMerchantPrompt(profile); strings.Contains(prompt, policyMailbox) || !strings.Contains(prompt, policyDomain) {
		t.Fatal("Provider prompt exposes the local mailbox capability or omits domain support.")
	}
	for _, providerResponse := range []string{
		strings.Replace(validThaiComposition, `"action":"BLOCK"`, `"action":"BLOCK","durationHours":12`, 1),
		strings.Replace(validThaiComposition, `"action":"BLOCK"`, `"action":"BLOCK","durationMinutes":2`, 1),
		strings.Replace(validThaiComposition, policyDomain, policyMailbox, 1),
	} {
		if _, err := parseComposerInterpretation([]byte(providerResponse), composerMerchant, profile); !errors.Is(err, errComposerOutput) {
			t.Fatalf("Expected provider mailbox proposal to be rejected, got %v.", err)
		}
	}
	composer := ruleComposer{access: composerAuthorization(repository{}, "local-mailbox-test-admin")}
	for _, testCase := range []struct {
		scope, token string
		status       int
	}{
		{composerMerchant, "", http.StatusUnauthorized},
		{composerMerchant, "local-mailbox-test-admin", http.StatusForbidden},
		{composerPlatform, "local-mailbox-test-admin", http.StatusUnprocessableEntity},
	} {
		request := httptest.NewRequest(http.MethodPost, composerPath+"/interpret", strings.NewReader(
			`{"scope":"`+testCase.scope+`","mailboxBlock":{"mailboxDigest":"`+strings.Repeat("a", 64)+`","durationHours":12}}`,
		))
		if testCase.token != "" {
			request.Header.Set("Authorization", "Bearer "+testCase.token)
		}
		response := httptest.NewRecorder()
		composer.interpret(response, request)
		if response.Code != testCase.status {
			t.Fatalf("Mailbox interpretation for %s returned %d, expected %d.", testCase.scope, response.Code, testCase.status)
		}
	}
}

func TestScopedMailboxInterpretationAndTransactionPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	admin := "scoped-mailbox-test-admin"
	const address = "buyer@example.test"
	mailboxDigest := func(mailbox string) string {
		sum := sha256.Sum256([]byte(mailbox))
		return hex.EncodeToString(sum[:])
	}
	digest := mailboxDigest(address)
	peerDigest := mailboxDigest("other@example.test")
	var firstRef string
	jevCalls := 0
	jev := testMerchantJev(t, func(r *http.Request) (*http.Response, error) {
		jevCalls++
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		for _, value := range []string{address, "example.test", digest, peerDigest, firstRef, `"mailbox"`} {
			if value != "" && strings.Contains(string(data), value) {
				t.Error("Jev received private mailbox evidence.")
			}
		}
		var payload struct {
			Questions merchantJevQuestions `json:"questions"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		answers := map[string]merchantJevChoice{merchantJevQuestion: {Type: "choice", Choice: "LOW_RISK"}}
		if _, requested := payload.Questions[merchantBookingSimilarityQuestion]; requested {
			answers[merchantBookingSimilarityQuestion] = merchantJevChoice{Type: "choice", Choice: "SIMILAR"}
		}
		body, err := json.Marshal(merchantJevResponse{Model: defaultMerchantJevModel, ID: "scoped-mailbox-test", Answers: answers})
		if err != nil {
			return nil, err
		}
		return testJevResponse(http.StatusOK, string(body)), nil
	})
	configuration, err := readJevAIConfiguration(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	liveConfig := defaultJevAIConfiguration()
	liveConfig.Provider, liveConfig.Model = "openrouter", defaultMerchantJevModel
	version := configuration.Version + 1
	if _, err := db.Exec(t.Context(), `INSERT INTO jev_ai_configuration_versions(version,config,created_by)
		VALUES ($1,$2,'scoped-mailbox-test')`, version, liveConfig); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(context.Background(), `DELETE FROM jev_ai_configuration_versions
			WHERE version=$1 AND created_by='scoped-mailbox-test'`, version); err != nil {
			t.Error(err)
		}
	})
	mux := http.NewServeMux()
	registerMerchantDemo(mux, repo, admin, jev)
	composer := ruleComposer{
		access: composerAuthorization(repo, admin),
		provider: testComposerProvider(func(context.Context, string, string, string) ([]byte, error) {
			t.Error("Exact mailbox blocks must be interpreted locally.")
			return nil, errComposerOutput
		}),
	}
	mux.HandleFunc("POST "+composerPath+"/interpret", composer.interpret)
	request := func(method, path, token string, input any, status int, output any) {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s returned %d, expected %d: %s", method, path, res.Code, status, res.Body.String())
		}
		if strings.Contains(res.Body.String(), digest) || strings.Contains(res.Body.String(), address) ||
			strings.Contains(path, "/transactions") && firstRef != "" && strings.Contains(res.Body.String(), firstRef) {
			t.Fatal("API response exposed private mailbox evidence.")
		}
		if output != nil {
			if err := json.Unmarshal(res.Body.Bytes(), output); err != nil {
				t.Fatal(err)
			}
		}
	}
	call := func(path, token string, input any, status int, output any) {
		t.Helper()
		request(http.MethodPost, path, token, input, status, output)
	}
	createMerchant := func(name string) (mockMerchant, mockScenario, string) {
		t.Helper()
		merchant, token, err := repo.createMockMerchant(t.Context(), mockMerchantInput{
			Name: name, PolicyIntegration: &merchantPolicyIntegration{Products: []string{"flight_ticket"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		scenario, err := repo.saveMockScenario(t.Context(), merchant.ID, "", "Scoped mailbox test", 1000)
		if err != nil {
			t.Fatal(err)
		}
		return merchant, scenario, token
	}
	first, firstScenario, firstToken := createMerchant("Scoped mailbox test merchant A")
	second, secondScenario, secondToken := createMerchant("Scoped mailbox test merchant B")
	interpretPath := composerPath + "/interpret"
	block := mailboxBlockInstruction{MailboxDigest: digest, DurationHours: 12}
	call(interpretPath, firstToken, map[string]any{"scope": composerMerchant, "instruction": "Block email " + address + " 12 h."}, http.StatusUnprocessableEntity, nil)
	call(interpretPath, firstToken, map[string]any{"scope": composerMerchant, "mailboxBlock": block, "instruction": "extra"}, http.StatusUnprocessableEntity, nil)
	for _, invalid := range []mailboxBlockInstruction{
		{MailboxDigest: "bad digest", DurationHours: 12},
		{MailboxDigest: digest, DurationHours: 0},
		{MailboxDigest: digest, DurationHours: maximumMailboxBlockHours + 1},
		{MailboxDigest: digest, DurationMinutes: -1},
		{MailboxDigest: digest, DurationMinutes: maximumMailboxBlockMinutes + 1},
		{MailboxDigest: digest, DurationHours: 1, DurationMinutes: 2},
	} {
		call(interpretPath, firstToken, map[string]any{"scope": composerMerchant, "mailboxBlock": invalid}, http.StatusUnprocessableEntity, nil)
	}
	interpret := func(token string, block mailboxBlockInstruction) (merchantPolicyProposal, string) {
		t.Helper()
		var output struct {
			Status     string
			Validation composerValidation
			Proposal   merchantPolicyProposal
		}
		call(interpretPath, token, map[string]any{"scope": composerMerchant, "mailboxBlock": block}, http.StatusOK, &output)
		proposal := output.Proposal
		if output.Status != "READY" || !output.Validation.Valid || proposal.Action != "BLOCK" ||
			len(proposal.Conditions.Rules) != 1 || proposal.Conditions.Rules[0].Field != policyMailbox {
			t.Fatal("Expected a ready, exact email block.")
		}
		var ref string
		if err := json.Unmarshal(proposal.Conditions.Rules[0].Value, &ref); err != nil || !validMailboxDigest(ref) || ref == digest {
			t.Fatal("Expected a derived mailbox reference.")
		}
		return proposal, ref
	}
	proposal, ref := interpret(firstToken, block)
	firstRef = ref
	if proposal.DurationHours != 12 || proposal.DurationMinutes != 0 || !strings.HasSuffix(proposal.Name, "(12 h)") {
		t.Fatal("Expected the legacy 12-hour block representation.")
	}
	equivalent, _ := interpret(firstToken, mailboxBlockInstruction{MailboxDigest: digest, DurationMinutes: 720})
	if !reflect.DeepEqual(proposal, equivalent) {
		t.Fatal("An equivalent minute instruction must preserve the rule identity.")
	}
	if _, otherRef := interpret(secondToken, block); firstRef == otherRef {
		t.Fatal("Same mailbox reference was used for different merchants.")
	}
	var legacy struct{ Rule merchantPolicyRule }
	call(merchantPolicyPath+"/rules", firstToken, map[string]any{"proposal": proposal}, http.StatusCreated, &legacy)
	amount := int64(50)
	const sharedID = "8de823e4-9254-4d5a-b1c7-2db0ed189480"
	transaction := func(merchant mockMerchant, scenario mockScenario, id, source, digest string, wantStatus int, wantAction string) {
		t.Helper()
		var result struct{ Transaction mockTransaction }
		call(merchantDemoPath+"/"+merchant.ID+"/transactions", admin, mockTransactionInput{
			ID: id, Source: source, Product: "flight_ticket", ScenarioID: scenario.ID, AmountMinor: &amount,
			CustomerEmailDomain: "example.test", CustomerMailboxDigest: digest,
		}, wantStatus, &result)
		if wantAction != "" && (result.Transaction.PolicyDecision == nil || result.Transaction.PolicyDecision.Final.Action != wantAction) {
			t.Fatalf("Expected final action %s, got %+v.", wantAction, result.Transaction.PolicyDecision)
		}
	}
	transaction(first, firstScenario, sharedID, "checkout", digest, http.StatusCreated, "BLOCK")
	transaction(first, firstScenario, sharedID, "checkout", digest, http.StatusOK, "BLOCK")
	transaction(first, firstScenario, sharedID, "checkout", peerDigest, http.StatusConflict, "")
	transaction(first, firstScenario, "8de823e4-9254-4d5a-b1c7-2db0ed189481", "simulation", digest, http.StatusCreated, "BLOCK")
	transaction(first, firstScenario, "8de823e4-9254-4d5a-b1c7-2db0ed189482", "simulation", peerDigest, http.StatusCreated, "ALLOW")
	transaction(first, firstScenario, "8de823e4-9254-4d5a-b1c7-2db0ed189483", "checkout", "", http.StatusCreated, "ALLOW")
	transaction(second, secondScenario, sharedID, "checkout", digest, http.StatusCreated, "ALLOW")
	if _, err := db.Exec(t.Context(), `UPDATE merchant_policy_rule_versions SET created_at=clock_timestamp()-INTERVAL '12 hours'
		WHERE merchant_id=$1::text::uuid`, first.ID); err != nil {
		t.Fatal(err)
	}
	transaction(first, firstScenario, "8de823e4-9254-4d5a-b1c7-2db0ed189484", "checkout", digest, http.StatusCreated, "ALLOW")
	transaction(first, firstScenario, sharedID, "checkout", digest, http.StatusOK, "BLOCK")
	if jevCalls != 6 {
		t.Fatalf("Jev received %d requests, expected one per new transaction and none for replay.", jevCalls)
	}
	var storedRef string
	if err := db.QueryRow(t.Context(), `SELECT customer_mailbox_ref FROM merchant_demo_transactions
		WHERE merchant_id=$1::text::uuid AND id=$2::text::uuid`, first.ID, sharedID).Scan(&storedRef); err != nil || storedRef != firstRef {
		t.Fatal("Transaction did not store the merchant-scoped mailbox reference.")
	}

	twoMinutes := mailboxBlockInstruction{MailboxDigest: digest, DurationMinutes: 2}
	minuteProposal, _ := interpret(firstToken, twoMinutes)
	if minuteProposal.DurationMinutes != 2 || minuteProposal.DurationHours != 0 || !strings.HasSuffix(minuteProposal.Name, "(2 min)") {
		t.Fatal("The preview did not retain the two-minute duration.")
	}
	assertTwoMinuteWindow := func(rule merchantPolicyRule, version int) {
		t.Helper()
		if rule.Status != "ACTIVE" || rule.Version != version || rule.ExpiresAt == nil || rule.ExpiresAt.Sub(rule.CreatedAt) != 2*time.Minute {
			t.Fatalf("Expected version %d to activate for two minutes, got %+v.", version, rule)
		}
	}
	var edited struct{ Rule merchantPolicyRule }
	request(http.MethodPut, merchantPolicyPath+"/rules/"+legacy.Rule.ID, firstToken,
		map[string]any{"proposal": minuteProposal, "expectedVersion": legacy.Rule.Version}, http.StatusOK, &edited)
	assertTwoMinuteWindow(edited.Rule, 2)
	const minuteCheckoutID = "8de823e4-9254-4d5a-b1c7-2db0ed189485"
	transaction(first, mockScenario{}, minuteCheckoutID, "checkout", digest, http.StatusCreated, "BLOCK")
	transaction(first, mockScenario{}, "8de823e4-9254-4d5a-b1c7-2db0ed189486", "simulation", digest, http.StatusCreated, "BLOCK")

	freshMinuteProposal, _ := interpret(secondToken, twoMinutes)
	saveRequest := map[string]any{"proposal": freshMinuteProposal}
	var created, retried struct{ Rule merchantPolicyRule }
	call(merchantPolicyPath+"/rules", secondToken, saveRequest, http.StatusCreated, &created)
	assertTwoMinuteWindow(created.Rule, 1)
	call(merchantPolicyPath+"/rules", secondToken, saveRequest, http.StatusCreated, &retried)
	if retried.Rule.ID != created.Rule.ID || retried.Rule.Version != created.Rule.Version ||
		retried.Rule.ExpiresAt == nil || !retried.Rule.ExpiresAt.Equal(*created.Rule.ExpiresAt) {
		t.Fatal("Retrying the same new minute rule extended its block or created another version.")
	}

	// Place the revision at its cutoff without making the integration test sleep.
	if _, err := db.Exec(t.Context(), `UPDATE merchant_policy_rule_versions
		SET created_at=clock_timestamp()-interval '2 minutes'
		WHERE merchant_id=$1::uuid AND rule_id=$2::uuid AND version=$3`, first.ID, edited.Rule.ID, edited.Rule.Version); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, merchantPolicyPath+"/rules/"+legacy.Rule.ID, firstToken, nil, http.StatusOK, &edited)
	if edited.Rule.Status != "EXPIRED" {
		t.Fatalf("Expected the minute rule to expire at the cutoff, got %s.", edited.Rule.Status)
	}
	transaction(first, mockScenario{}, "8de823e4-9254-4d5a-b1c7-2db0ed189487", "checkout", digest, http.StatusCreated, "ALLOW")
	transaction(first, mockScenario{}, minuteCheckoutID, "checkout", digest, http.StatusOK, "BLOCK")
}
