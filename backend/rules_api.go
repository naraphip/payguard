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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	drafts, err := a.repo.draftRules(ctx, merchantID)
	if err != nil {
		unavailable(w, err)
		return
	}
	runs, err := a.repo.previewRuns(ctx, merchantID)
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
	rule, err := a.repo.createDraft(ctx, merchantID, input)
	if err != nil {
		unavailable(w, err)
		return
	}
	respond(w, 201, rule)
}

func (a app) updateRule(w http.ResponseWriter, r *http.Request) {
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
	rule, err := a.repo.updateDraft(ctx, merchantID, id, request.Version, request.ruleInput)
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

func (a app) previewRules(w http.ResponseWriter, r *http.Request) {
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
	saved, err := a.repo.get(ctx, merchantID, request.RunID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "Run not found.")
	case err != nil:
		unavailable(w, err)
	default:
		respond(w, 200, previewRule(saved, request.Rule))
	}
}
