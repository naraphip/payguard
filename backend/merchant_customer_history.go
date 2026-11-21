package main

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
)

var merchantCustomerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func merchantCustomerIDRef(ctx context.Context, db mockMerchantQueryer, merchant, customerID string) (string, error) {
	if customerID == "" {
		return "", nil
	}
	// Separate Customer IDs from the existing bare-hex mailbox namespace.
	return merchantScopedRef(ctx, db, merchant, "customer-id:"+customerID)
}

// Call under the merchant lock before inserting a transaction. The immediately
// previous email evidence makes this a transition signal for the first change.
func customerChangedDisposableEmail(ctx context.Context, db mockMerchantQueryer, merchant, customerRef, domain, mailboxRef string) (bool, error) {
	if customerRef == "" || (domain == "" && mailboxRef == "") {
		return false, nil
	}
	var previousDomain, previousMailboxRef string
	var disposable bool
	err := db.QueryRow(ctx, `
		SELECT customer_email_domain, customer_mailbox_ref,
			COALESCE(policy_decision @> '{"riskScore":{"factors":[{"code":"DISPOSABLE_EMAIL_DOMAIN"}]}}', false)
		FROM merchant_demo_transactions
		WHERE merchant_id=$1::text::uuid AND customer_id_ref=$2 AND customer_id_ref <> ''
			AND (customer_email_domain <> '' OR customer_mailbox_ref <> '')
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, merchant, customerRef).Scan(&previousDomain, &previousMailboxRef, &disposable)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil || !disposable {
		return false, err
	}
	if previousMailboxRef != "" && mailboxRef != "" {
		return previousMailboxRef != mailboxRef, nil
	}
	return previousDomain != "" && domain != "" && previousDomain != domain, nil
}
