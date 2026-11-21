package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const merchantBookingSimilarityQuestion = "booking_similarity"

type merchantComparisonBooking struct {
	Product     string  `json:"product"`
	AmountMinor *int64  `json:"amountMinor"`
	Disposition *string `json:"disposition,omitempty"`
}

type merchantCustomFieldComparison struct {
	Same    int `json:"same"`
	Changed int `json:"changed"`
}

// The provider receives only this projection. Local identifiers and custom
// signal contents never become part of the comparison state.
type merchantBookingComparisonEvidence struct {
	Prior                 merchantComparisonBooking      `json:"prior"`
	Current               merchantComparisonBooking      `json:"current"`
	DescriptionComparison string                         `json:"descriptionComparison"`
	CustomFieldComparison *merchantCustomFieldComparison `json:"customFieldComparison,omitempty"`
}

type merchantBookingComparison struct {
	merchantBookingComparisonEvidence
	Status     string   `json:"status"`
	Similarity string   `json:"similarity,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// Read the latest saved booking under the merchant lock before insertion.
func readMerchantBookingComparison(ctx context.Context, db mockMerchantQueryer, merchant, mailboxRef string, input mockTransactionInput, fields []merchantCapability) (*merchantBookingComparison, error) {
	if mailboxRef == "" {
		return nil, nil
	}
	var comparison merchantBookingComparison
	var priorSignals map[string]json.RawMessage
	err := db.QueryRow(ctx, `
		SELECT product, amount_minor, disposition, custom_signals
		FROM merchant_demo_transactions
		WHERE merchant_id=$1::text::uuid AND customer_mailbox_ref=$2 AND customer_mailbox_ref <> ''
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, merchant, mailboxRef).
		Scan(&comparison.Prior.Product, &comparison.Prior.AmountMinor, &comparison.Prior.Disposition, &priorSignals)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !validBookingProduct(comparison.Prior.Product) && comparison.Prior.Product != "payment_gateway" {
		comparison.Prior.Product = ""
	}
	comparison.Current = merchantComparisonBooking{Product: input.Product, AmountMinor: input.AmountMinor}
	comparison.DescriptionComparison = "UNKNOWN"
	if same, known := compareBookingField(policyDescription, "text", priorSignals, input.CustomSignals); known {
		comparison.DescriptionComparison = "CHANGED"
		if same {
			comparison.DescriptionComparison = "SAME"
		}
	}
	var custom merchantCustomFieldComparison
	for _, field := range fields {
		if !customSignalKey.MatchString(field.Name) {
			continue
		}
		same, known := compareBookingField(field.Name, field.DataType, priorSignals, input.CustomSignals)
		switch {
		case known && same:
			custom.Same++
		case known:
			custom.Changed++
		}
	}
	if custom.Same+custom.Changed > 0 {
		comparison.CustomFieldComparison = &custom
	}
	return &comparison, nil
}

func compareBookingField(key, kind string, prior, current map[string]json.RawMessage) (same bool, known bool) {
	if !validCustomValue(kind, prior[key]) || !validCustomValue(kind, current[key]) {
		return false, false
	}
	condition := merchantPolicyCondition{Field: key, Operator: "eq", Value: prior[key]}
	same, unavailable := evaluateCustomCondition(condition, current)
	return same, !unavailable
}

// A known change explains the similarity answer. A mock risk recommendation
// also needs a recorded rejection plus a changed product or amount.
func (comparison *merchantBookingComparison) mock() bool {
	productsKnown := comparison.Prior.Product != "" && comparison.Current.Product != ""
	amountsKnown := comparison.Prior.AmountMinor != nil && comparison.Current.AmountMinor != nil
	productChanged := productsKnown && comparison.Prior.Product != comparison.Current.Product
	amountChanged := amountsKnown && !sameBookingAmount(comparison.Prior.AmountMinor, comparison.Current.AmountMinor)
	previouslyRejected := comparison.Prior.Disposition != nil && *comparison.Prior.Disposition == "rejected"
	customChanged := comparison.CustomFieldComparison != nil && comparison.CustomFieldComparison.Changed > 0
	customSame := comparison.CustomFieldComparison != nil && comparison.CustomFieldComparison.Same > 0
	switch {
	case productChanged || amountChanged || comparison.DescriptionComparison == "CHANGED" || customChanged:
		comparison.Similarity = "DIFFERENT"
	case productsKnown && amountsKnown || comparison.DescriptionComparison == "SAME" || customSame:
		comparison.Similarity = "SIMILAR"
	default:
		comparison.Similarity = "INSUFFICIENT"
	}
	return previouslyRejected && (productChanged || amountChanged)
}
