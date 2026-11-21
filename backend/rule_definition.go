package main

import "net/http"

// A definition is a read model for the composer. Each engine still validates and saves its own proposal.
type ruleDefinition struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Scope         string              `json:"scope"`
	Name          string              `json:"name"`
	Description   string              `json:"description,omitempty"`
	ProductTypes  []string            `json:"productTypes,omitempty"`
	Stages        []string            `json:"stages"`
	Action        string              `json:"action"`
	Conditions    ruleDefinitionGroup `json:"conditions"`
	Duration      *ruleDuration       `json:"duration,omitempty"`
}

type ruleDefinitionGroup struct {
	Logic string                    `json:"logic"`
	Rules []ruleDefinitionCondition `json:"rules"`
}

type ruleDefinitionCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type ruleDuration struct {
	Value int    `json:"value"`
	Unit  string `json:"unit"`
}

func respondRuleInterpretation(w http.ResponseWriter, output composerInterpretation) {
	if output.Status == "READY" && output.Validation.Valid {
		output.Definition = definitionFromProposal(output.Proposal)
	}
	respond(w, http.StatusOK, output)
}

func definitionFromProposal(proposal any) *ruleDefinition {
	definition := ruleDefinition{SchemaVersion: 1}
	switch proposal := proposal.(type) {
	case platformComposerProposal:
		if proposal.Rule == nil {
			return nil
		}
		input := proposal.Rule
		definition.Scope = composerPlatform
		definition.Name, definition.Description = input.Name, input.Description
		definition.Stages, definition.Action = []string{input.Stage}, input.Action
		definition.Conditions.Logic = "AND"
		for _, condition := range input.Conditions {
			definition.Conditions.Rules = append(definition.Conditions.Rules, ruleDefinitionCondition{
				Field: condition.Signal, Operator: condition.Operator, Value: condition.Value,
			})
		}
	case merchantPolicyProposal:
		definition.Scope = composerMerchant
		definition.Name, definition.Action = proposal.Name, proposal.Action
		definition.Stages, definition.ProductTypes = proposal.Stages, proposal.ProductTypes
		definition.Conditions.Logic = proposal.Conditions.Logic
		for _, condition := range proposal.Conditions.Rules {
			item := ruleDefinitionCondition{Field: condition.Field, Operator: condition.Operator}
			if condition.Field != policyMailbox {
				item.Value = condition.Value
			}
			definition.Conditions.Rules = append(definition.Conditions.Rules, item)
		}
		if proposal.DurationMinutes > 0 {
			definition.Duration = &ruleDuration{Value: proposal.DurationMinutes, Unit: "minute"}
		} else if proposal.DurationHours > 0 {
			definition.Duration = &ruleDuration{Value: proposal.DurationHours, Unit: "hour"}
		}
	default:
		return nil
	}
	return &definition
}
