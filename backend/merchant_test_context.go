package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type merchantTestPolicyCondition struct {
	Field    string          `json:"field"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value,omitempty"`
}

type merchantTestPolicyConditions struct {
	Logic string                        `json:"logic"`
	Rules []merchantTestPolicyCondition `json:"rules"`
}

type merchantTestPolicy struct {
	ID         string                       `json:"id"`
	Name       string                       `json:"name"`
	Action     string                       `json:"action"`
	Version    int                          `json:"version"`
	Products   []string                     `json:"products"`
	Conditions merchantTestPolicyConditions `json:"conditions"`
	ExpiresAt  *time.Time                   `json:"expiresAt,omitempty"`
}

func (s mockMerchantService) merchantTestContext(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(w, r, true); !ok {
		return
	}
	id, ok := demoPathID(w, r, "merchantId")
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.repo.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		demoWriteError(w, err)
		return
	}
	defer tx.Rollback(ctx)

	profile, _, err := readMerchantPolicyContext(ctx, tx, id)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	platformRules, err := readActivePlatformRules(ctx, tx)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	rules, err := readMerchantPolicies(ctx, tx, id, true)
	if err != nil {
		demoWriteError(w, err)
		return
	}
	jevRules, err := readJevRules(ctx, tx, id)
	if err != nil {
		demoWriteError(w, err)
		return
	}

	policies := make([]merchantTestPolicy, 0, len(rules))
	for _, rule := range rules {
		proposal := rule.Proposal
		name := proposal.Name
		conditions := make([]merchantTestPolicyCondition, 0, len(proposal.Conditions.Rules))
		for _, condition := range proposal.Conditions.Rules {
			value := condition.Value
			if condition.Field == policyMailbox {
				value = nil
				// The composer-generated name contains part of the mailbox reference.
				name = "Temporary email block"
			}
			conditions = append(conditions, merchantTestPolicyCondition{Field: condition.Field, Operator: condition.Operator, Value: value})
		}
		policies = append(policies, merchantTestPolicy{
			ID: rule.ID, Name: name, Action: proposal.Action, Version: rule.Version,
			Products: proposal.ProductTypes, Conditions: merchantTestPolicyConditions{Logic: proposal.Conditions.Logic, Rules: conditions}, ExpiresAt: rule.ExpiresAt,
		})
	}
	respond(w, http.StatusOK, struct {
		Products                   []string             `json:"products"`
		FieldRegistry              []merchantCapability `json:"fieldRegistry"`
		Policies                   []merchantTestPolicy `json:"policies"`
		JevRules                   jevRulesSnapshot     `json:"jevRules"`
		PlatformRuleVersion        string               `json:"platformRuleVersion"`
		UnavailablePlatformSignals []string             `json:"unavailablePlatformSignals"`
	}{
		Products: profile.Products, FieldRegistry: profile.FieldRegistry, Policies: policies, JevRules: jevRules,
		PlatformRuleVersion: platformRulesetVersion(platformRules), UnavailablePlatformSignals: merchantUnavailablePlatformSignals(),
	})
}
