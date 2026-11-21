package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	maximumMailboxBlockHours    = 365 * 24
	maximumMailboxBlockMinutes  = maximumMailboxBlockHours * 60
	mailboxBlockDurationMessage = "Use one duration from 1 minute to 365 days, in whole minutes or hours."
)

func validMailboxBlockDuration(hours, minutes int) bool {
	return (hours == 0 && minutes >= 1 && minutes <= maximumMailboxBlockMinutes) ||
		(minutes == 0 && hours >= 1 && hours <= maximumMailboxBlockHours)
}

func validMailboxDigest(value string) bool {
	return len(value) == sha256.Size*2 && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f')
	}) == -1
}

// The browser submits a digest of the normalized mailbox. Use a different
// reference for each merchant and keep the browser digest out of storage.
func merchantMailboxRef(ctx context.Context, db mockMerchantQueryer, merchant, digest string) (string, error) {
	if digest == "" {
		return "", nil
	}
	var key string
	if err := db.QueryRow(ctx, `SELECT mailbox_ref_key::text FROM merchant_demo_merchants
		WHERE id=$1::text::uuid AND deleted_at IS NULL`, merchant).Scan(&key); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(digest))
	return hex.EncodeToString(mac.Sum(nil)), nil
}
