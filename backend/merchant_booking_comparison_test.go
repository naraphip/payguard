package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestMerchantBookingComparisonJev(t *testing.T) {
	zero := int64(0)
	for _, fixture := range []struct {
		name, similarity, risk, wantStatus, wantOutcome string
		responseCode                                    int
		rules                                           jevRulesSnapshot
		platform                                        string
	}{
		{"similar without amount", "SIMILAR", "LOW_RISK", "LIVE", "LOW_RISK", 200, jevRulesSnapshot{}, "ALLOW"},
		{"insufficient without amount", "INSUFFICIENT", "LOW_RISK", "LIVE", "UNRESOLVED", 200, jevRulesSnapshot{}, "ALLOW"},
		{"amount required", "SIMILAR", "LOW_RISK", "LIVE", "UNRESOLVED", 200, jevRulesSnapshot{Rules: []publishedJevRule{{}}}, "ALLOW"},
		{"provider review", "DIFFERENT", "REVIEW", "LIVE", "REVIEW", 200, jevRulesSnapshot{}, "ALLOW"},
		{"hard block", "SIMILAR", "LOW_RISK", "LIVE", "HIGH_RISK", 200, jevRulesSnapshot{}, "BLOCK"},
		{"missing answer", "", "LOW_RISK", "UNAVAILABLE", "UNRESOLVED", 200, jevRulesSnapshot{}, "ALLOW"},
		{"provider timeout", "", "", "TIMEOUT", "UNRESOLVED", 504, jevRulesSnapshot{}, "ALLOW"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			calls := 0
			comparison := &merchantBookingComparison{merchantBookingComparisonEvidence: merchantBookingComparisonEvidence{
				Prior:                 merchantComparisonBooking{Product: "physical", AmountMinor: &zero},
				Current:               merchantComparisonBooking{Product: "physical"},
				DescriptionComparison: "SAME",
				CustomFieldComparison: &merchantCustomFieldComparison{Same: 1, Changed: 1},
			}}
			jev := testMerchantJev(t, func(request *http.Request) (*http.Response, error) {
				calls++
				var payload struct {
					State     map[string]any       `json:"state"`
					Questions merchantJevQuestions `json:"questions"`
				}
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				wantState := map[string]any{
					"booking_amount_minor": nil, "configured_review_rule_matched": false,
					"booking_comparison": map[string]any{
						"prior":                 map[string]any{"product": "physical", "amountMinor": float64(0)},
						"current":               map[string]any{"product": "physical", "amountMinor": nil},
						"descriptionComparison": "SAME",
						"customFieldComparison": map[string]any{"same": float64(1), "changed": float64(1)},
					},
				}
				if !reflect.DeepEqual(payload.State, wantState) {
					t.Fatalf("risk provider state = %v, want %v", payload.State, wantState)
				}
				riskQuestion, similarityQuestion := payload.Questions[merchantJevQuestion], payload.Questions[merchantBookingSimilarityQuestion]
				if len(payload.Questions) != 2 || riskQuestion.Type != "choice" || similarityQuestion.Type != "choice" ||
					len(riskQuestion.Criteria) != 2 || len(similarityQuestion.Criteria) != 3 ||
					!strings.Contains(riskQuestion.Instructions, jevAdvisoryRubrics["standard-v1"]) {
					t.Fatalf("provider did not receive both constrained questions and configured rubric: %v", payload.Questions)
				}
				choices := map[string]merchantJevChoice{merchantJevQuestion: {Type: "choice", Choice: fixture.risk}}
				if fixture.similarity != "" {
					confidence := 0.8
					choices[merchantBookingSimilarityQuestion] = merchantJevChoice{Type: "choice", Choice: fixture.similarity, Confidence: &confidence}
				}
				body, err := json.Marshal(merchantJevResponse{Model: defaultMerchantJevModel, ID: "compare-test", Answers: choices})
				if err != nil {
					t.Fatal(err)
				}
				return testJevResponse(fixture.responseCode, string(body)), nil
			})
			jev.rubric = "standard-v1"
			input := mockTransactionInput{Product: "physical", CustomerEmailDomain: "example.test", CustomerID: "private-reference",
				ID: "an-internal-transaction", CustomSignals: map[string]json.RawMessage{policyDescription: json.RawMessage(`"A stored product description"`)}}
			result := resolveMerchantRuleDecision(t.Context(), input, policyDecisionPart{Action: fixture.platform}, "rules-v1",
				policyDecisionPart{Action: "NOT_APPLICABLE"}, fixture.rules, jev, comparison)
			if calls != 1 || result.Outcome != fixture.wantOutcome || comparison.Status != fixture.wantStatus ||
				comparison.Similarity != fixture.similarity || result.PolicyDecision.BookingComparison != comparison {
				t.Fatalf("comparison=%+v outcome=%+v calls=%d", comparison, result, calls)
			}
			if wantReview := fixture.wantOutcome == "REVIEW" || fixture.wantOutcome == "UNRESOLVED"; (result.ReviewState == "NEEDS_DECISION") != wantReview {
				t.Fatalf("outcome %s produced review state %s", result.Outcome, result.ReviewState)
			}
			if fixture.wantStatus == "LIVE" && (comparison.Confidence == nil || *comparison.Confidence != 0.8 ||
				result.JevAnswer != fixture.risk || result.JevDecisionID != "compare-test") {
				t.Fatalf("validated comparison or risk reply was lost: %+v", result)
			}
		})
	}
}

func TestMerchantBookingComparisonPostgres(t *testing.T) {
	db := openPolicyTestDatabase(t)
	repo := repository{db}
	ctx := t.Context()
	createMerchant := func(name string) mockMerchant {
		t.Helper()
		merchant, _, err := repo.createMockMerchant(ctx, mockMerchantInput{Name: name, PolicyIntegration: &merchantPolicyIntegration{
			Products: []string{"physical", "digital"}, CustomFields: []customSignalDefinition{
				{Key: "custom_units", Label: "Units", Type: "number"},
				{Key: "custom_title", Label: "Ticket title", Type: "text"},
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return merchant
	}
	first, second := createMerchant("Guest comparison one"), createMerchant("Guest comparison two")
	makeInput := func(source, product string, amount int64) mockTransactionInput {
		t.Helper()
		var id pgtype.UUID
		if err := db.QueryRow(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
			t.Fatal(err)
		}
		return mockTransactionInput{ID: id.String(), Source: source, Product: product, AmountMinor: &amount,
			CustomerEmailDomain: "example.test", CustomerMailboxDigest: strings.Repeat("ab", 32)}
	}
	save := func(merchant mockMerchant, input mockTransactionInput, wantCreated bool) mockTransaction {
		t.Helper()
		result, created, err := repo.saveMockTransaction(ctx, merchant.ID, input)
		if err != nil || created != wantCreated {
			t.Fatalf("save: created=%v err=%v", created, err)
		}
		return result
	}
	initial := makeInput("checkout", "physical", 1400)
	initial.CustomSignals = map[string]json.RawMessage{
		policyDescription: json.RawMessage(`"Local title A"`), "custom_units": json.RawMessage(`2`), "custom_title": json.RawMessage(`"First title"`),
	}
	if result := save(first, initial, true); result.PolicyDecision.BookingComparison != nil {
		t.Fatal("first guest booking unexpectedly got a comparison")
	}
	if result := save(second, makeInput("simulation", "physical", 1400), true); result.PolicyDecision.BookingComparison != nil {
		t.Fatal("same mailbox digest in another merchant got a comparison")
	}
	domainOnly := makeInput("simulation", "digital", 9999)
	domainOnly.CustomerMailboxDigest = ""
	if result := save(first, domainOnly, true); result.PolicyDecision.BookingComparison != nil {
		t.Fatal("domain-only booking got a comparison")
	}
	changed := makeInput("simulation", "physical", 1400)
	changed.CustomerID = "any-registered-id"
	changed.CustomSignals = map[string]json.RawMessage{
		policyDescription: json.RawMessage(`"Local title B"`), "custom_units": json.RawMessage(`2`), "custom_title": json.RawMessage(`"Second title"`),
	}
	result := save(first, changed, true)
	comparison := result.PolicyDecision.BookingComparison
	if comparison == nil || comparison.Status != "MOCKED" || comparison.Similarity != "DIFFERENT" ||
		comparison.DescriptionComparison != "CHANGED" || comparison.CustomFieldComparison == nil ||
		*comparison.CustomFieldComparison != (merchantCustomFieldComparison{Same: 1, Changed: 1}) ||
		comparison.Prior.AmountMinor == nil || *comparison.Prior.AmountMinor != 1400 || result.Outcome != "LOW_RISK" {
		t.Fatalf("same-mailbox lookup did not compare expected booking details: %+v", comparison)
	}
	timedOut := makeInput("checkout", "physical", 5200)
	timedOut.SimulateTimeout = true
	timeoutResult := save(first, timedOut, true)
	if timeoutResult.PolicyDecision.BookingComparison.Status != "TIMEOUT" || timeoutResult.Outcome != "UNRESOLVED" {
		t.Fatalf("incorrect timeout comparison: %+v", timeoutResult)
	}
	if _, err := repo.decideMockTransaction(ctx, first.ID, timedOut.ID, "rejected", "platform-admin"); err != nil {
		t.Fatal(err)
	}
	afterRejection := save(first, makeInput("simulation", "digital", 6000), true)
	comparison = afterRejection.PolicyDecision.BookingComparison
	score := afterRejection.PolicyDecision.RiskScore
	if comparison == nil || comparison.Prior.Disposition == nil || *comparison.Prior.Disposition != "rejected" ||
		comparison.Prior.AmountMinor == nil || *comparison.Prior.AmountMinor != 5200 ||
		afterRejection.Outcome != "REVIEW" || afterRejection.ReviewState != "NEEDS_DECISION" ||
		score == nil || score.Value == nil || *score.Value != 55 {
		t.Fatalf("prior rejection and change did not trigger the Jev review floor: %+v", afterRejection)
	}
	replay := save(first, changed, false)
	originalEvidence, err := json.Marshal(result.PolicyDecision)
	if err != nil {
		t.Fatal(err)
	}
	replayedEvidence, err := json.Marshal(replay.PolicyDecision)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(originalEvidence, replayedEvidence) {
		t.Fatalf("replay changed the original evidence: first=%s replay=%s", originalEvidence, replayedEvidence)
	}
}
