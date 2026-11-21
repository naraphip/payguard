package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type rulesSettings struct {
	Rules       []systemRule `json:"rules"`
	Signals     []signal     `json:"signals"`
	Runs        []previewRun `json:"runs"`
	RuleVersion string       `json:"ruleVersion"`
}

func decodeRuleRequest(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		problem(w, 400, "Expected valid rule JSON.")
		return false
	}
	return true
}

func (a app) readRules(w http.ResponseWriter, r *http.Request) {
	a.readTenantRules(w, r, merchantID)
}

func (a app) readTenantRules(w http.ResponseWriter, r *http.Request, tenant string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	drafts, err := a.repo.draftRules(ctx, tenant)
	if err != nil {
		unavailable(w, err)
		return
	}
	runs, err := a.repo.previewRuns(ctx, tenant)
	if err != nil {
		unavailable(w, err)
		return
	}
	rules := make([]systemRule, 0, len(builtinRules)+len(drafts))
	for _, rule := range builtinRules {
		rules = append(rules, rule.systemRule)
	}
	rules = append(rules, drafts...)
	respond(w, 200, rulesSettings{rules, ruleSignals, runs, ruleVersion})
}

func (a app) createRule(w http.ResponseWriter, r *http.Request) {
	a.createTenantRule(w, r, merchantID)
}

func (a app) createTenantRule(w http.ResponseWriter, r *http.Request, tenant string) {
	var input ruleInput
	if !decodeRuleRequest(w, r, &input) {
		return
	}
	if err := input.validate(); err != nil {
		problem(w, 422, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rule, err := a.repo.createDraft(ctx, tenant, input)
	if err != nil {
		unavailable(w, err)
		return
	}
	respond(w, 201, rule)
}

func (a app) updateRule(w http.ResponseWriter, r *http.Request) {
	a.updateTenantRule(w, r, merchantID)
}

func (a app) updateTenantRule(w http.ResponseWriter, r *http.Request, tenant string) {
	id := r.PathValue("id")
	for _, rule := range builtinRules {
		if id == rule.ID {
			problem(w, 403, "Active platform rules are read-only. Create a draft to try a change.")
			return
		}
	}
	var request struct {
		ruleInput
		Version int64 `json:"version"`
	}
	if !decodeRuleRequest(w, r, &request) {
		return
	}
	if request.Version < 1 {
		problem(w, 422, "Expected a positive draft version.")
		return
	}
	if err := request.ruleInput.validate(); err != nil {
		problem(w, 422, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rule, err := a.repo.updateDraft(ctx, tenant, id, request.Version, request.ruleInput)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "Rule draft not found.")
	case errors.Is(err, errStaleRule):
		problem(w, 409, "This draft changed since you opened it. Reload it before editing.")
	case err != nil:
		unavailable(w, err)
	default:
		respond(w, 200, rule)
	}
}

func (a app) deleteRule(w http.ResponseWriter, r *http.Request) {
	a.deleteTenantRule(w, r, merchantID)
}

func (a app) deleteTenantRule(w http.ResponseWriter, r *http.Request, tenant string) {
	id := r.PathValue("id")
	for _, rule := range builtinRules {
		if rule.ID == id {
			problem(w, http.StatusForbidden, "Built-in safeguards cannot be deleted.")
			return
		}
	}
	var input struct {
		Version int64 `json:"version"`
	}
	if !decodeRuleRequest(w, r, &input) {
		return
	}
	if input.Version < 1 {
		problem(w, http.StatusUnprocessableEntity, "Expected a positive draft version.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var protected = errors.New("This draft has an approval history. Manage its published versions in Platform rule approval.")
	err := pgx.BeginFunc(ctx, a.repo.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "payguard-platform-revision:"+id); err != nil {
			return err
		}
		var version int64
		if err := tx.QueryRow(ctx, `SELECT version FROM system_rule_drafts WHERE tenant_id=$1 AND rule_id=$2 FOR UPDATE`, tenant, id).Scan(&version); err != nil {
			return err
		}
		if version != input.Version {
			return errStaleRule
		}
		if tenant == merchantID {
			var governed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_rule_revisions WHERE rule_id=$1)`, id).Scan(&governed); err != nil {
				return err
			}
			if governed {
				return protected
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM system_rule_drafts WHERE tenant_id=$1 AND rule_id=$2`, tenant, id)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Rule draft not found.")
	case errors.Is(err, errStaleRule), errors.Is(err, protected):
		problem(w, http.StatusConflict, err.Error())
	case err != nil:
		unavailable(w, err)
	default:
		respond(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func (a app) previewRules(w http.ResponseWriter, r *http.Request) {
	a.previewTenantRules(w, r, merchantID)
}

func (a app) previewTenantRules(w http.ResponseWriter, r *http.Request, tenant string) {
	var request struct {
		RunID string    `json:"runId"`
		Rule  ruleInput `json:"rule"`
	}
	if !decodeRuleRequest(w, r, &request) {
		return
	}
	if !validID.MatchString(request.RunID) {
		problem(w, 422, "Choose a saved simulation run.")
		return
	}
	if err := request.Rule.validate(); err != nil {
		problem(w, 422, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	saved, err := a.repo.get(ctx, tenant, request.RunID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "Run not found.")
	case err != nil:
		unavailable(w, err)
	default:
		respond(w, 200, previewRule(saved, request.Rule))
	}
}
