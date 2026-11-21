package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

const merchantPolicyPath = "/api/v1/booking/demo/merchant-policy"

type merchantPolicyValidationError struct{ error }

func (s mockMerchantService) registerMerchantPolicies(mux *http.ServeMux) {
	mux.HandleFunc("GET "+merchantPolicyPath+"/profile", s.policyProfile)
	mux.HandleFunc("GET "+merchantPolicyPath+"/rules", s.policyList)
	mux.HandleFunc("GET "+merchantPolicyPath+"/rules/{id}", s.policyHistory)
	mux.HandleFunc("POST "+merchantPolicyPath+"/rules", s.policyMutate)
	mux.HandleFunc("PUT "+merchantPolicyPath+"/rules/{id}", s.policyMutate)
	for _, operation := range []string{"activate", "disable", "rollback", "simulate"} {
		mux.HandleFunc("POST "+merchantPolicyPath+"/rules/{id}/"+operation, s.policyMutate)
	}
}

func (s mockMerchantService) policyActor(w http.ResponseWriter, r *http.Request) (string, bool) {
	actor, ok := s.authorize(w, r, false)
	if ok && (actor.Admin || actor.MerchantID == "") {
		problem(w, http.StatusForbidden, "A credential belonging to the merchant is required.")
		return "", false
	}
	return actor.MerchantID, ok
}

func merchantPolicyError(w http.ResponseWriter, err error) {
	var validation merchantPolicyValidationError
	switch {
	case errors.As(err, &validation):
		problem(w, http.StatusUnprocessableEntity, validation.Error())
	case errors.Is(err, errPolicyConflict), errors.Is(err, errPolicyVersion), errors.Is(err, errPolicyRuleLimit):
		problem(w, http.StatusConflict, err.Error())
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Merchant policy not found.")
	default:
		bookingRiskUnavailable(w, err)
	}
}

func (s mockMerchantService) policyProfile(w http.ResponseWriter, r *http.Request) {
	merchant, ok := s.policyActor(w, r)
	if !ok {
		return
	}
	profile, err := s.merchantPolicyProfile(r.Context(), merchant)
	if err != nil {
		merchantPolicyError(w, err)
		return
	}
	respond(w, http.StatusOK, profile)
}

func (s mockMerchantService) policyList(w http.ResponseWriter, r *http.Request) {
	merchant, ok := s.policyActor(w, r)
	if !ok {
		return
	}
	s.respondMerchantPolicies(w, r, merchant)
}
func (s mockMerchantService) respondMerchantPolicies(w http.ResponseWriter, r *http.Request, merchant string) {
	rules, err := readMerchantPolicies(r.Context(), s.repo.db, merchant, false)
	if err != nil {
		merchantPolicyError(w, err)
		return
	}
	effective, err := readMerchantPolicies(r.Context(), s.repo.db, merchant, true)
	if err != nil {
		merchantPolicyError(w, err)
		return
	}
	for i := range rules {
		for _, current := range effective {
			if rules[i].ID == current.ID {
				rules[i].ActiveVersion = current.Version
				break
			}
		}
	}
	respond(w, http.StatusOK, map[string]any{"rules": rules})
}

func (s mockMerchantService) policyHistory(w http.ResponseWriter, r *http.Request) {
	merchant, ok := s.policyActor(w, r)
	if !ok {
		return
	}
	s.respondMerchantPolicyHistory(w, r, merchant)
}
func (s mockMerchantService) respondMerchantPolicyHistory(w http.ResponseWriter, r *http.Request, merchant string) {
	id, ok := demoPathID(w, r, "id")
	if !ok {
		return
	}

	rows, err := s.repo.db.Query(r.Context(), `SELECT `+merchantPolicyColumns+`
		FROM merchant_policy_rule_versions WHERE merchant_id=$1::text::uuid AND rule_id=$2::text::uuid ORDER BY version DESC LIMIT 100`,
		merchant, id)
	if err != nil {
		merchantPolicyError(w, err)
		return
	}
	defer rows.Close()
	history := []merchantPolicyRule{}
	for rows.Next() {
		item, err := scanMerchantPolicy(rows)
		if err != nil {
			merchantPolicyError(w, err)
			return
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		merchantPolicyError(w, err)
		return
	}
	if len(history) == 0 {
		merchantPolicyError(w, pgx.ErrNoRows)
		return
	}
	respond(w, http.StatusOK, map[string]any{"rule": history[0], "versions": history})
}

func (s mockMerchantService) policyMutate(w http.ResponseWriter, r *http.Request) {
	merchant, ok := s.policyActor(w, r)
	if !ok {
		return
	}
	input := struct {
		Proposal        merchantPolicyProposal `json:"proposal"`
		Version         int                    `json:"version"`
		ExpectedVersion int                    `json:"expectedVersion"`
	}{}
	operation := "save"
	id := r.PathValue("id")
	if id != "" {
		if _, valid := demoPathID(w, r, "id"); !valid {
			return
		}
		if r.Method == http.MethodPut {
			operation = "edit"
		} else {
			operation = r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		}
	}
	if operation == "simulate" {
		rule, err := scanMerchantPolicy(s.repo.db.QueryRow(r.Context(), `SELECT `+merchantPolicyColumns+`
			FROM merchant_policy_rule_versions WHERE merchant_id=$1::text::uuid AND rule_id=$2::text::uuid ORDER BY version DESC LIMIT 1`, merchant, id))
		if err != nil {
			merchantPolicyError(w, err)
			return
		}
		_, evidence, err := readMerchantPolicyContext(r.Context(), s.repo.db, merchant)
		if err != nil {
			merchantPolicyError(w, err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"preview": previewMerchantPolicy(rule.Proposal, evidence)})
		return
	}
	if !decodeBookingRiskJSON(w, r, &input) {
		return
	}
	if id != "" && input.ExpectedVersion <= 0 {
		problem(w, http.StatusUnprocessableEntity, "The latest expectedVersion is required.")
		return
	}
	if operation == "rollback" && (input.Version <= 0 || input.Version >= input.ExpectedVersion) {
		problem(w, http.StatusUnprocessableEntity, "Select an older rule version to restore.")
		return
	}
	rule, err := s.repo.mutateMerchantPolicy(r.Context(), merchant, merchantPolicyMutation{
		Kind: operation, ID: id, ExpectedVersion: input.ExpectedVersion, RollbackVersion: input.Version, Proposal: input.Proposal,
	})
	if err != nil {
		merchantPolicyError(w, err)
		return
	}
	status := http.StatusOK
	if operation == "save" {
		status = http.StatusCreated
	}
	respond(w, status, map[string]any{"rule": rule, "status": rule.Status})
}
