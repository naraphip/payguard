package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const platformRuleGovernanceSchema = `
CREATE TABLE IF NOT EXISTS platform_rule_revisions (
  rule_id text NOT NULL,
  version bigint NOT NULL CHECK (version > 0),
  draft_version bigint NOT NULL CHECK (draft_version > 0),
  action text NOT NULL CHECK (action IN ('SUBMITTED','APPROVED','ROLLED_BACK','DISABLED')),
  rule_input jsonb NOT NULL,
  preview jsonb NOT NULL,
  source_version bigint,
  reason text NOT NULL,
  actor text NOT NULL CHECK (actor IN ('platform-admin','platform-checker')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (rule_id, version),
  FOREIGN KEY (rule_id, source_version) REFERENCES platform_rule_revisions (rule_id, version)
);
CREATE INDEX IF NOT EXISTS platform_rule_active_revisions ON platform_rule_revisions (rule_id, version DESC)
  WHERE action IN ('APPROVED','ROLLED_BACK','DISABLED');
`

const (
	platformMaker   = "platform-admin"
	platformChecker = "platform-checker"
)

type platformRuleRevision struct {
	ID            string      `json:"id"`
	Version       int64       `json:"version"`
	DraftVersion  int64       `json:"draftVersion"`
	Status        string      `json:"status"`
	Action        string      `json:"action"`
	Rule          ruleInput   `json:"rule"`
	Preview       rulePreview `json:"preview"`
	SourceVersion *int64      `json:"sourceVersion,omitempty"`
	Reason        string      `json:"reason"`
	Actor         string      `json:"actor"`
	ActorID       *string     `json:"actorId,omitempty"`
	CreatedAt     time.Time   `json:"createdAt"`
}

type platformRuleGovernance struct {
	access         mockMerchantService
	checkerHash    [sha256.Size]byte
	checkerEnabled bool
}

func registerPlatformRuleGovernance(mux *http.ServeMux, repo repository, adminToken, checkerToken string) {
	access := composerAuthorization(repo, adminToken)
	checkerHash := sha256.Sum256([]byte(checkerToken))
	governance := platformRuleGovernance{
		access: access, checkerHash: checkerHash,
		checkerEnabled: bookingTokenPattern.MatchString(adminToken) && bookingTokenPattern.MatchString(checkerToken) &&
			subtle.ConstantTimeCompare(access.adminHash[:], checkerHash[:]) == 0,
	}
	path := composerPath + "/platform/{id}"
	mux.HandleFunc("POST "+path+"/submit", governance.submit)
	mux.HandleFunc("POST "+path+"/approve", governance.approve)
	mux.HandleFunc("POST "+path+"/rollback", governance.rollback)
	mux.HandleFunc("GET "+path+"/versions", governance.versions)
}

func (g platformRuleGovernance) credentialMatches(r *http.Request, want [sha256.Size]byte) bool {
	headers := r.Header.Values("Authorization")
	if len(headers) != 1 {
		return false
	}
	scheme, value, ok := strings.Cut(headers[0], " ")
	value = strings.TrimLeft(value, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || !bookingTokenPattern.MatchString(value) {
		return false
	}
	hash := sha256.Sum256([]byte(value))
	return subtle.ConstantTimeCompare(hash[:], want[:]) == 1
}

func (g platformRuleGovernance) requireChecker(w http.ResponseWriter, r *http.Request) bool {
	if actor, ok := productionActorFromRequest(r); ok {
		if actor.Role == productionRoleChecker {
			return true
		}
		problem(w, http.StatusForbidden, "An independent platform checker is required for this operation.")
		return false
	}
	if g.checkerEnabled && g.credentialMatches(r, g.checkerHash) {
		return true
	}
	if g.access.adminEnabled && g.credentialMatches(r, g.access.adminHash) {
		problem(w, http.StatusForbidden, "An independent platform checker is required for this operation.")
		return false
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	problem(w, http.StatusUnauthorized, "A valid platform checker bearer token is required.")
	return false
}

const platformRevisionColumns = "rule_id,version,draft_version,action,rule_input,preview,source_version,reason,actor,actor_id::text,created_at"

var platformDraftID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func platformPathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !platformDraftID.MatchString(id) {
		problem(w, http.StatusNotFound, "Platform draft not found.")
		return "", false
	}
	return id, true
}

func (revision *platformRuleRevision) scan(row pgx.Row) error {
	err := row.Scan(&revision.ID, &revision.Version, &revision.DraftVersion, &revision.Action, &revision.Rule,
		&revision.Preview, &revision.SourceVersion, &revision.Reason, &revision.Actor, &revision.ActorID, &revision.CreatedAt)
	switch revision.Action {
	case "SUBMITTED":
		revision.Status = "PENDING"
	case "APPROVED", "ROLLED_BACK":
		revision.Status = "ACTIVE"
	case "DISABLED":
		revision.Status = "DISABLED"
	}
	return err
}

func readPlatformRevision(ctx context.Context, db systemDraftWriter, condition string, args ...any) (platformRuleRevision, error) {
	var result platformRuleRevision
	err := result.scan(db.QueryRow(ctx, "SELECT "+platformRevisionColumns+" FROM platform_rule_revisions "+condition, args...))
	return result, err
}

func insertPlatformRevision(ctx context.Context, tx pgx.Tx, next platformRuleRevision) (platformRuleRevision, error) {
	var result platformRuleRevision
	err := result.scan(tx.QueryRow(ctx, `INSERT INTO platform_rule_revisions
		(rule_id,version,draft_version,action,rule_input,preview,source_version,reason,actor,actor_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::text::uuid) RETURNING `+platformRevisionColumns,
		next.ID, next.Version, next.DraftVersion, next.Action, next.Rule, next.Preview, next.SourceVersion, next.Reason, next.Actor, next.ActorID))
	return result, err
}

func platformAccountID(r *http.Request) *string {
	if actor, ok := productionActorFromRequest(r); ok {
		return &actor.ID
	}
	return nil
}

func samePlatformAccount(first, second *string) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return *first == *second
}

func platformChangeReason(value string) (string, bool) {
	reason := strings.TrimSpace(value)
	return reason, reason != "" && utf8.RuneCountInString(reason) <= 250 &&
		strings.IndexFunc(reason, unicode.IsControl) == -1 && !sensitiveComposerText(reason)
}

var (
	errPlatformDraftStale = errors.New("The draft version has changed. Reload it before requesting approval.")
	errPlatformNoEvidence = errors.New("The selected shared simulation run has no eligible events for this rule.")
	errPlatformNoPending  = errors.New("That pending revision is no longer eligible for approval.")
	errPlatformNotCurrent = errors.New("The active revision has changed. Reload the version history before rolling back.")
	errPlatformBadTarget  = errors.New("Choose an earlier active revision or disable the rule.")
	errPlatformSelfCheck  = errors.New("Approval requires a recorded submitter and a different platform checker.")
)

func (g platformRuleGovernance) writeRevision(w http.ResponseWriter, r *http.Request, operation func(context.Context, pgx.Tx, string, platformRuleRevision) (platformRuleRevision, error)) {
	id, ok := platformPathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := g.access.repo.db.Begin(ctx)
	if err != nil {
		unavailable(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "payguard-platform-revision:"+id); err != nil {
		unavailable(w, err)
		return
	}
	latest, err := readPlatformRevision(ctx, tx, "WHERE rule_id=$1 ORDER BY version DESC LIMIT 1", id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		unavailable(w, err)
		return
	}
	result, err := operation(ctx, tx, id, latest)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "The draft or saved shared simulation run was not found.")
	case errors.Is(err, errPlatformDraftStale), errors.Is(err, errPlatformNoPending), errors.Is(err, errPlatformNotCurrent):
		problem(w, http.StatusConflict, err.Error())
	case errors.Is(err, errPlatformNoEvidence), errors.Is(err, errPlatformBadTarget):
		problem(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, errPlatformSelfCheck):
		problem(w, http.StatusForbidden, err.Error())
	case err != nil:
		unavailable(w, err)
	default:
		if err := tx.Commit(ctx); err != nil {
			unavailable(w, err)
			return
		}
		status := http.StatusCreated
		if result.Version == latest.Version {
			status = http.StatusOK
		}
		respond(w, status, result)
	}
}

func (g platformRuleGovernance) submit(w http.ResponseWriter, r *http.Request) {
	if _, ok := g.access.authorize(w, r, true); !ok {
		return
	}
	var input struct {
		DraftVersion int64  `json:"draftVersion"`
		RunID        string `json:"runId"`
		Reason       string `json:"reason"`
	}
	if !decodeRuleRequest(w, r, &input) {
		return
	}
	reason, validReason := platformChangeReason(input.Reason)
	if !validReason || input.DraftVersion < 1 || !validID.MatchString(input.RunID) {
		problem(w, http.StatusUnprocessableEntity, "Choose a draft version, a saved simulation run, and a reason of 1 to 250 characters without personal data.")
		return
	}
	accountID := platformAccountID(r)
	g.writeRevision(w, r, func(ctx context.Context, tx pgx.Tx, id string, latest platformRuleRevision) (platformRuleRevision, error) {
		draft, err := readDraft(tx.QueryRow(ctx, `SELECT rule_id,input,version,updated_at FROM system_rule_drafts
			WHERE tenant_id=$1 AND rule_id=$2 FOR SHARE`, merchantID, id))
		if err != nil {
			return platformRuleRevision{}, err
		}
		if draft.Version != input.DraftVersion {
			return platformRuleRevision{}, errPlatformDraftStale
		}
		if latest.Status == "PENDING" && latest.DraftVersion == draft.Version && latest.Preview.RunID == input.RunID &&
			latest.Reason == reason && samePlatformAccount(latest.ActorID, accountID) {
			return latest, nil
		}
		// Shared simulation runs are bounded and immutable. The stored preview
		// provides the exact evidence seen before checker approval.
		var saved run
		if err := tx.QueryRow(ctx, `SELECT result FROM simulation_runs WHERE merchant_id=$1 AND run_id=$2`,
			merchantID, input.RunID).Scan(&saved); err != nil {
			return platformRuleRevision{}, err
		}
		preview := previewRule(saved, draft.ruleInput)
		if preview.Eligible == 0 {
			return platformRuleRevision{}, errPlatformNoEvidence
		}
		return insertPlatformRevision(ctx, tx, platformRuleRevision{
			ID: id, Version: latest.Version + 1, DraftVersion: draft.Version, Action: "SUBMITTED",
			Rule: draft.ruleInput, Preview: preview, Reason: reason, Actor: platformMaker, ActorID: accountID,
		})
	})
}

func (g platformRuleGovernance) approve(w http.ResponseWriter, r *http.Request) {
	if !g.requireChecker(w, r) {
		return
	}
	var input struct {
		Version int64  `json:"version"`
		Reason  string `json:"reason"`
	}
	if !decodeRuleRequest(w, r, &input) {
		return
	}
	reason, validReason := platformChangeReason(input.Reason)
	if input.Version < 1 || !validReason {
		problem(w, http.StatusUnprocessableEntity, "Choose a pending revision and a reason of 1 to 250 characters without personal data.")
		return
	}
	accountID := platformAccountID(r)
	g.writeRevision(w, r, func(ctx context.Context, tx pgx.Tx, _ string, latest platformRuleRevision) (platformRuleRevision, error) {
		if latest.Action == "APPROVED" && latest.SourceVersion != nil && *latest.SourceVersion == input.Version &&
			latest.Reason == reason && samePlatformAccount(latest.ActorID, accountID) {
			return latest, nil
		}
		if latest.Action != "SUBMITTED" || latest.Version != input.Version || latest.Actor != platformMaker {
			return platformRuleRevision{}, errPlatformNoPending
		}
		if (latest.ActorID == nil) != (accountID == nil) || (accountID != nil && samePlatformAccount(latest.ActorID, accountID)) {
			return platformRuleRevision{}, errPlatformSelfCheck
		}
		next := latest
		next.Action, next.Actor, next.Reason = "APPROVED", platformChecker, reason
		next.ActorID = accountID
		next.Version, next.SourceVersion = latest.Version+1, &latest.Version
		return insertPlatformRevision(ctx, tx, next)
	})
}

const latestPlatformActiveRevision = `WHERE rule_id=$1 AND action IN ('APPROVED','ROLLED_BACK','DISABLED')
	ORDER BY version DESC LIMIT 1`

func (g platformRuleGovernance) rollback(w http.ResponseWriter, r *http.Request) {
	if !g.requireChecker(w, r) {
		return
	}
	var input struct {
		Version               *int64 `json:"version"`
		ExpectedActiveVersion int64  `json:"expectedActiveVersion"`
		Reason                string `json:"reason"`
	}
	if !decodeRuleRequest(w, r, &input) {
		return
	}
	reason, validReason := platformChangeReason(input.Reason)
	if input.Version == nil || *input.Version < 0 || input.ExpectedActiveVersion < 1 || !validReason {
		problem(w, http.StatusUnprocessableEntity, "Choose the current and target revisions and a reason of 1 to 250 characters without personal data.")
		return
	}
	g.writeRevision(w, r, func(ctx context.Context, tx pgx.Tx, id string, latest platformRuleRevision) (platformRuleRevision, error) {
		current, err := readPlatformRevision(ctx, tx, latestPlatformActiveRevision, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (current.Status != "ACTIVE" || current.Version != input.ExpectedActiveVersion)) {
			return platformRuleRevision{}, errPlatformNotCurrent
		}
		if err != nil {
			return platformRuleRevision{}, err
		}
		if *input.Version >= current.Version {
			return platformRuleRevision{}, errPlatformBadTarget
		}
		next := current
		next.Action = "DISABLED"
		if *input.Version > 0 {
			next, err = readPlatformRevision(ctx, tx, "WHERE rule_id=$1 AND version=$2", id, *input.Version)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && next.Status != "ACTIVE") {
				return platformRuleRevision{}, errPlatformBadTarget
			}
			if err != nil {
				return platformRuleRevision{}, err
			}
			next.Action = "ROLLED_BACK"
		}
		sourceVersion := next.Version
		next.Version, next.SourceVersion = latest.Version+1, &sourceVersion
		next.Actor, next.Reason = platformChecker, reason
		next.ActorID = platformAccountID(r)
		return insertPlatformRevision(ctx, tx, next)
	})
}

func (g platformRuleGovernance) versions(w http.ResponseWriter, r *http.Request) {
	account, production := productionActorFromRequest(r)
	if production {
		if account.Role != productionRoleAdmin && account.Role != productionRoleChecker {
			problem(w, http.StatusForbidden, "Platform rule history requires a platform account.")
			return
		}
	} else if !g.checkerEnabled || !g.credentialMatches(r, g.checkerHash) {
		if _, ok := g.access.authorize(w, r, true); !ok {
			return
		}
	}
	id, ok := platformPathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	tx, err := g.access.repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		unavailable(w, err)
		return
	}
	defer tx.Rollback(ctx)
	current, err := readPlatformRevision(ctx, tx, latestPlatformActiveRevision, id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		unavailable(w, err)
		return
	}
	var activeVersion int64
	if current.Status == "ACTIVE" {
		activeVersion = current.Version
	}
	rows, err := tx.Query(ctx, "SELECT "+platformRevisionColumns+` FROM platform_rule_revisions
		WHERE rule_id=$1 ORDER BY version DESC LIMIT 100`, id)
	if err != nil {
		unavailable(w, err)
		return
	}
	defer rows.Close()
	revisions := make([]platformRuleRevision, 0)
	for rows.Next() {
		var item platformRuleRevision
		if err := item.scan(rows); err != nil {
			unavailable(w, err)
			return
		}
		revisions = append(revisions, item)
	}
	if err := rows.Err(); err != nil {
		unavailable(w, err)
		return
	}
	respond(w, http.StatusOK, struct {
		ActiveVersion int64                  `json:"activeVersion"`
		Versions      []platformRuleRevision `json:"versions"`
	}{activeVersion, revisions})
}

type activePlatformRule struct {
	ID      string
	Version int64
	Rule    ruleInput
}

func (r repository) activePlatformRules(ctx context.Context) ([]activePlatformRule, error) {
	return readActivePlatformRules(ctx, r.db)
}

func readActivePlatformRules(ctx context.Context, db mockMerchantQueryer) ([]activePlatformRule, error) {
	rows, err := db.Query(ctx, `SELECT rule_id,version,rule_input FROM (
		SELECT DISTINCT ON (rule_id) rule_id,version,action,rule_input FROM platform_rule_revisions
		WHERE action IN ('APPROVED','ROLLED_BACK','DISABLED') ORDER BY rule_id,version DESC
	) latest WHERE action <> 'DISABLED' ORDER BY rule_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]activePlatformRule, 0)
	for rows.Next() {
		var active activePlatformRule
		if err := rows.Scan(&active.ID, &active.Version, &active.Rule); err != nil {
			return nil, err
		}
		rules = append(rules, active)
	}
	return rules, rows.Err()
}

func platformRulesetVersion(rules []activePlatformRule) string {
	if len(rules) == 0 {
		return ruleVersion
	}
	hash := sha256.New()
	for _, active := range rules {
		_, _ = fmt.Fprintf(hash, "%s:%d\n", active.ID, active.Version)
	}
	return ruleVersion + "+governed-" + hex.EncodeToString(hash.Sum(nil)[:16])
}

func platformActionSeverity(action string) int {
	switch action {
	case "BLOCK":
		return 3
	case "REVIEW":
		return 2
	case "CHALLENGE":
		return 1
	default:
		return 0
	}
}

func applyGovernedPlatformRules(d *decision, rules []activePlatformRule) {
	for _, active := range rules {
		if (active.Rule.Action != "REVIEW" && active.Rule.Action != "CHALLENGE") || !active.Rule.includesStage(d.Event.Stage) {
			continue
		}
		if matched, _ := active.Rule.matchSignals(d.Event, d.Features); matched {
			d.RuleHits = append(d.RuleHits, active.ID+"@"+strconv.FormatInt(active.Version, 10))
			if platformActionSeverity(active.Rule.Action) > platformActionSeverity(d.Platform) {
				d.Platform = active.Rule.Action
				if d.Source == "PLATFORM" {
					d.Action = d.Platform
				}
			}
		}
	}
}
