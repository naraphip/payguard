package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s mockMerchantService) registerRules(mux *http.ServeMux) {
	platform := merchantDemoPath + "/{merchantId}/platform/rules"
	mux.HandleFunc("GET "+platform, s.ruleHandler(app.readTenantRules))
	mux.HandleFunc("POST "+platform, s.ruleHandler(app.createTenantRule))
	mux.HandleFunc("PUT "+platform+"/{id}", s.ruleHandler(app.updateTenantRule))
	mux.HandleFunc("DELETE "+platform+"/{id}", s.ruleHandler(app.deleteTenantRule))
	mux.HandleFunc("POST "+platform+"/preview", s.ruleHandler(app.previewTenantRules))

	jev := merchantDemoPath + "/{merchantId}/jev/rules"
	mux.HandleFunc("GET "+jev, s.ruleHandler(app.readMockJevRules))
	mux.HandleFunc("POST "+jev, s.ruleHandler(app.publishMockJevRule))
	mux.HandleFunc("POST "+jev+"/preview", s.ruleHandler(app.previewMockJevRule))
	mux.HandleFunc("PUT "+jev+"/{id}", s.ruleHandler(app.reviseMockJevRule))
	mux.HandleFunc("DELETE "+jev+"/{id}", s.ruleHandler(app.reviseMockJevRule))
}

func (s mockMerchantService) ruleHandler(handle func(app, http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.authorize(w, r, true); !ok {
			return
		}
		merchantID, ok := demoPathID(w, r, "merchantId")
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := s.repo.db.QueryRow(ctx, `
			SELECT id::text FROM merchant_demo_merchants WHERE id=$1::text::uuid AND deleted_at IS NULL
		`, merchantID).Scan(&merchantID); err != nil {
			demoWriteError(w, err)
			return
		}
		handle(app{repo: s.repo}, w, r.WithContext(ctx), merchantID)
	}
}

func decodeMockJevRule(w http.ResponseWriter, r *http.Request, definition *jevRuleDefinition) bool {
	if !decodeBookingRiskJSON(w, r, definition) {
		return false
	}
	return validateMockJevRule(w, definition)
}

func validateMockJevRule(w http.ResponseWriter, definition *jevRuleDefinition) bool {
	if err := definition.validate(); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return false
	}
	for _, condition := range definition.Conditions {
		if condition.Signal != bookingAmount {
			problem(w, http.StatusUnprocessableEntity, "Merchant Jev review rules support amountMinor conditions.")
			return false
		}
	}
	return true
}

func (a app) readMockJevRules(w http.ResponseWriter, r *http.Request, merchantID string) {
	snapshot, err := readJevRules(r.Context(), a.repo.db, merchantID)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, snapshot)
}

func (a app) publishMockJevRule(w http.ResponseWriter, r *http.Request, merchantID string) {
	var definition jevRuleDefinition
	if !decodeMockJevRule(w, r, &definition) {
		return
	}
	snapshot, err := a.repo.publishJevRule(r.Context(), merchantID, definition)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, snapshot)
}

func (a app) reviseMockJevRule(w http.ResponseWriter, r *http.Request, merchantID string) {
	id := r.PathValue("id")
	if !strings.HasPrefix(id, jevRuleIDPrefix) || len(id) > 128 {
		problem(w, http.StatusNotFound, "Jev rule not found.")
		return
	}
	var revision struct {
		ExpectedVersion string `json:"expectedVersion"`
	}
	var definition *jevRuleDefinition
	if r.Method == http.MethodPut {
		var edit struct {
			ExpectedVersion string             `json:"expectedVersion"`
			Definition      *jevRuleDefinition `json:"definition"`
		}
		if !decodeBookingRiskJSON(w, r, &edit) {
			return
		}
		if edit.Definition == nil {
			problem(w, http.StatusUnprocessableEntity, "Include a Jev rule definition.")
			return
		}
		if !validateMockJevRule(w, edit.Definition) {
			return
		}
		revision.ExpectedVersion = edit.ExpectedVersion
		definition = edit.Definition
	} else if !decodeBookingRiskJSON(w, r, &revision) {
		return
	}
	if strings.TrimSpace(revision.ExpectedVersion) == "" {
		problem(w, http.StatusUnprocessableEntity, "Include the current Jev ruleset version.")
		return
	}
	snapshot, err := a.repo.reviseMerchantJevRule(r.Context(), merchantID, id, revision.ExpectedVersion, definition)
	if err != nil {
		if errors.Is(err, errJevRulesVersion) || errors.Is(err, errJevRuleExists) {
			problem(w, http.StatusConflict, err.Error())
		} else {
			demoWriteError(w, err)
		}
		return
	}
	respond(w, http.StatusOK, snapshot)
}

func (a app) previewMockJevRule(w http.ResponseWriter, r *http.Request, merchantID string) {
	var definition jevRuleDefinition
	if !decodeMockJevRule(w, r, &definition) {
		return
	}
	preview, err := a.repo.previewMerchantJevRule(r.Context(), merchantID, definition)
	if err != nil {
		bookingRiskUnavailable(w, err)
		return
	}
	respond(w, http.StatusOK, preview)
}

func mockJevSignals(amount *int64) map[string]int64 {
	if amount == nil {
		return nil
	}
	return map[string]int64{bookingAmount: *amount}
}

func (r repository) previewMerchantJevRule(ctx context.Context, merchantID string, definition jevRuleDefinition) (jevRulePreview, error) {
	var preview jevRulePreview
	rows, err := r.db.Query(ctx, `
		SELECT amount_minor FROM merchant_demo_transactions WHERE merchant_id=$1::text::uuid
		ORDER BY created_at DESC, id DESC LIMIT $2`, merchantID, jevRulePreviewLimit)
	if err != nil {
		return preview, err
	}
	defer rows.Close()
	for rows.Next() {
		var amount *int64
		if err := rows.Scan(&amount); err != nil {
			return preview, err
		}
		matched, missing := definition.matches(mockJevSignals(amount))
		preview.Evaluated++
		if matched {
			preview.Matched++
		}
		if missing {
			preview.Missing++
		}
	}
	return preview, rows.Err()
}

func (result *mockTransaction) applyMerchantJevRules(snapshot jevRulesSnapshot) {
	result.JevRuleVersion = snapshot.Version
	if result.JevStatus == "TIMEOUT" || result.AmountMinor == nil {
		return
	}
	observed := mockJevSignals(result.AmountMinor)
	var names []string
	for _, rule := range snapshot.Rules {
		if matched, _ := rule.matches(observed); matched {
			result.RuleHits = append(result.RuleHits, rule.ID)
			names = append(names, rule.Name)
		}
	}
	if len(names) == 0 {
		return
	}
	reason := fmt.Sprintf("Matched merchant Jev rules: %s.", strings.Join(names, ", "))
	if result.Outcome == "REVIEW" {
		result.Reason += " " + reason
	} else {
		result.Reason = reason
	}
	result.Outcome, result.ReviewState = "REVIEW", "NEEDS_DECISION"
}
