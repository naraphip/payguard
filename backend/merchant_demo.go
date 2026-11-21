package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const merchantDemoPath = "/api/v1/booking/demo/merchants"

var (
	errDemoNameInUse        = errors.New("A scenario with this name already exists for this merchant.")
	errDemoReplayConflict   = errors.New("Transaction ID is already used for a different request.")
	errDemoDecisionConflict = errors.New("This transaction does not have an open review for that decision.")
)

type mockMerchantService struct {
	repo         repository
	adminHash    [sha256.Size]byte
	adminEnabled bool
}

type mockMerchantActor struct {
	Admin      bool
	MerchantID string
}

type mockMerchant struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Scenarios []mockScenario `json:"scenarios"`
}

type mockScenario struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ThresholdMinor  int64  `json:"thresholdMinor"`
	ScenarioVersion int    `json:"scenarioVersion"`
	RuleVersion     string `json:"ruleVersion"`
}

type mockTransactionInput struct {
	ID              string `json:"id"`
	ScenarioID      string `json:"scenarioId"`
	Source          string `json:"source"`
	Product         string `json:"product,omitempty"`
	AmountMinor     *int64 `json:"amountMinor"`
	SimulateTimeout bool   `json:"simulateTimeout"`
}

type mockTransaction struct {
	ID              string     `json:"id"`
	MerchantID      string     `json:"merchantId"`
	ScenarioID      string     `json:"scenarioId"`
	ScenarioName    string     `json:"scenarioName"`
	ScenarioVersion int        `json:"scenarioVersion"`
	RuleVersion     string     `json:"ruleVersion"`
	JevRuleVersion  string     `json:"jevRuleVersion,omitempty"`
	ThresholdMinor  int64      `json:"thresholdMinor"`
	Source          string     `json:"source"`
	Product         string     `json:"product,omitempty"`
	AmountMinor     *int64     `json:"amountMinor"`
	Outcome         string     `json:"outcome"`
	Reason          string     `json:"reason"`
	RuleHits        []string   `json:"ruleHits"`
	JevStatus       string     `json:"jevStatus"`
	JevVersion      string     `json:"jevVersion"`
	ReviewState     string     `json:"reviewState"`
	CreatedAt       time.Time  `json:"createdAt"`
	Disposition     *string    `json:"disposition,omitempty"`
	DecidedBy       *string    `json:"decidedBy,omitempty"`
	DecidedAt       *time.Time `json:"decidedAt,omitempty"`
}

func registerMerchantDemo(mux *http.ServeMux, repo repository, adminToken string) {
	service := mockMerchantService{repo: repo, adminHash: sha256.Sum256([]byte(adminToken)), adminEnabled: adminToken != ""}
	mux.HandleFunc("GET "+merchantDemoPath, service.listMerchants)
	mux.HandleFunc("POST "+merchantDemoPath, service.createMerchant)
	mux.HandleFunc("PUT "+merchantDemoPath+"/{merchantId}", service.updateMerchant)
	mux.HandleFunc("POST "+merchantDemoPath+"/{merchantId}/scenarios", service.saveScenario)
	mux.HandleFunc("PUT "+merchantDemoPath+"/{merchantId}/scenarios/{scenarioId}", service.saveScenario)
	mux.HandleFunc("GET "+merchantDemoPath+"/{merchantId}/transactions", service.listTransactions)
	mux.HandleFunc("POST "+merchantDemoPath+"/{merchantId}/transactions", service.createTransaction)
	mux.HandleFunc("GET "+merchantDemoPath+"/{merchantId}/transactions/{transactionId}", service.getTransaction)
	mux.HandleFunc("POST "+merchantDemoPath+"/{merchantId}/transactions/{transactionId}/decision", service.decideTransaction)
	service.registerRules(mux)
}

func (s mockMerchantService) authorize(w http.ResponseWriter, r *http.Request, adminOnly bool) (mockMerchantActor, bool) {
	headers := r.Header.Values("Authorization")
	if len(headers) == 1 {
		scheme, value, ok := strings.Cut(headers[0], " ")
		value = strings.TrimLeft(value, " ")
		if ok && strings.EqualFold(scheme, "Bearer") && bookingTokenPattern.MatchString(value) {
			hash := sha256.Sum256([]byte(value))
			if s.adminEnabled && subtle.ConstantTimeCompare(s.adminHash[:], hash[:]) == 1 {
				return mockMerchantActor{Admin: true}, true
			}
			var merchantID string
			err := s.repo.db.QueryRow(r.Context(), `SELECT id::text FROM merchant_demo_merchants WHERE token_hash=$1`, hash[:]).Scan(&merchantID)
			switch {
			case err == nil:
				if adminOnly {
					problem(w, http.StatusForbidden, "An administrator is required for this operation.")
					return mockMerchantActor{}, false
				}
				if target := r.PathValue("merchantId"); target != "" && target != merchantID {
					problem(w, http.StatusNotFound, "Merchant not found.")
					return mockMerchantActor{}, false
				}
				return mockMerchantActor{MerchantID: merchantID}, true
			case !errors.Is(err, pgx.ErrNoRows):
				bookingRiskUnavailable(w, err)
				return mockMerchantActor{}, false
			}
		}
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	problem(w, http.StatusUnauthorized, "A valid demo administrator or merchant bearer token is required.")
	return mockMerchantActor{}, false
}

func demoPathID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id := r.PathValue(name)
	if !validID.MatchString(id) {
		problem(w, http.StatusNotFound, "Resource not found.")
		return "", false
	}
	return id, true
}

func demoWriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Merchant, scenario, or transaction not found.")
	case errors.Is(err, errDemoNameInUse), errors.Is(err, errDemoReplayConflict), errors.Is(err, errDemoDecisionConflict):
		problem(w, http.StatusConflict, err.Error())
	default:
		bookingRiskUnavailable(w, err)
	}
}

func validDemoName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= 100 &&
		strings.IndexFunc(name, unicode.IsControl) == -1
}

func readDemoName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeBookingRiskJSON(w, r, &input) {
		return "", false
	}
	name := strings.TrimSpace(input.Name)
	if !validDemoName(name) {
		problem(w, http.StatusUnprocessableEntity, "Enter a name of 1 to 100 characters without control characters.")
		return "", false
	}
	return name, true
}

func (s mockMerchantService) listMerchants(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.authorize(w, r, false)
	if !ok {
		return
	}
	merchants, err := s.repo.listMockMerchants(r.Context(), actor.MerchantID)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	role := "merchant"
	if actor.Admin {
		role = "admin"
	}
	respond(w, http.StatusOK, map[string]any{"merchants": merchants, "role": role})
}

func (s mockMerchantService) createMerchant(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(w, r, true); !ok {
		return
	}
	name, ok := readDemoName(w, r)
	if !ok {
		return
	}
	merchant, token, err := s.repo.createMockMerchant(r.Context(), name)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusCreated, map[string]any{"merchant": merchant, "token": token})
}

func (s mockMerchantService) updateMerchant(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(w, r, true); !ok {
		return
	}
	id, ok := demoPathID(w, r, "merchantId")
	if !ok {
		return
	}
	name, ok := readDemoName(w, r)
	if !ok {
		return
	}
	merchant, err := s.repo.renameMockMerchant(r.Context(), id, name)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"merchant": merchant})
}

func (s mockMerchantService) saveScenario(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(w, r, true); !ok {
		return
	}
	merchantID, ok := demoPathID(w, r, "merchantId")
	if !ok {
		return
	}
	scenarioID := r.PathValue("scenarioId")
	if scenarioID != "" {
		if _, ok := demoPathID(w, r, "scenarioId"); !ok {
			return
		}
	}
	var input struct {
		Name           string `json:"name"`
		ThresholdMinor *int64 `json:"thresholdMinor"`
	}
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if !validDemoName(input.Name) || input.ThresholdMinor == nil || !validDemoAmount(input.ThresholdMinor) {
		problem(w, http.StatusUnprocessableEntity, "Enter a scenario name and a nonnegative integer review threshold in minor units.")
		return
	}
	scenario, err := s.repo.saveMockScenario(r.Context(), merchantID, scenarioID, input.Name, *input.ThresholdMinor)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	status := http.StatusOK
	if scenarioID == "" {
		status = http.StatusCreated
	}
	respond(w, status, map[string]any{"scenario": scenario})
}

func validDemoAmount(amount *int64) bool {
	return amount == nil || (*amount >= 0 && *amount <= jevRuleMaxValue)
}

func validMockTransaction(input mockTransactionInput) bool {
	return validID.MatchString(input.ID) && validID.MatchString(input.ScenarioID) &&
		(input.Source == "checkout" || input.Source == "simulation") &&
		(validBookingProduct(input.Product) || (input.Source == "simulation" && input.Product == "payment_gateway")) &&
		validDemoAmount(input.AmountMinor)
}

func (s mockMerchantService) listTransactions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(w, r, false); !ok {
		return
	}
	id, ok := demoPathID(w, r, "merchantId")
	if !ok {
		return
	}
	transactions, err := s.repo.listMockTransactions(r.Context(), id)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"transactions": transactions})
}

func (s mockMerchantService) createTransaction(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(w, r, false); !ok {
		return
	}
	id, ok := demoPathID(w, r, "merchantId")
	if !ok {
		return
	}
	var input mockTransactionInput
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if !validMockTransaction(input) {
		problem(w, http.StatusUnprocessableEntity, "Enter valid transaction and scenario IDs, source, product, and optional amount in minor units.")
		return
	}
	transaction, created, err := s.repo.saveMockTransaction(r.Context(), id, input)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respond(w, status, map[string]any{"transaction": transaction})
}

func (s mockMerchantService) getTransaction(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(w, r, false); !ok {
		return
	}
	merchantID, ok := demoPathID(w, r, "merchantId")
	if !ok {
		return
	}
	transactionID, ok := demoPathID(w, r, "transactionId")
	if !ok {
		return
	}
	result, err := readMockTransaction(r.Context(), s.repo.db, merchantID, transactionID)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"transaction": result})
}

func (s mockMerchantService) decideTransaction(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.authorize(w, r, false)
	if !ok {
		return
	}
	merchantID, ok := demoPathID(w, r, "merchantId")
	if !ok {
		return
	}
	transactionID, ok := demoPathID(w, r, "transactionId")
	if !ok {
		return
	}
	var input struct {
		Decision string `json:"decision"`
	}
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if input.Decision != "cleared" && input.Decision != "rejected" {
		problem(w, http.StatusUnprocessableEntity, "Choose cleared or rejected.")
		return
	}
	decidedBy := "merchant"
	if actor.Admin {
		decidedBy = "administrator"
	}
	result, err := s.repo.decideMockTransaction(r.Context(), merchantID, transactionID, input.Decision, decidedBy)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"transaction": result})
}
