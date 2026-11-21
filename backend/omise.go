package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	omiseChargesURL    = "https://api.omise.co/charges"
	omiseAPIVersion    = "2019-05-29"
	omiseCurrency      = "THB"
	omiseCallTimeout   = 8 * time.Second
	omiseWriteTimeout  = 3 * time.Second
	omiseAttemptWindow = omiseCallTimeout + omiseWriteTimeout + time.Second
	maxOmiseResponse   = 64 << 10
)

var (
	omisePublicKey = regexp.MustCompile(`^pkey_test_[A-Za-z0-9]{1,128}$`)
	omiseSecretKey = regexp.MustCompile(`^skey_test_[A-Za-z0-9]{1,128}$`)
	omiseCardToken = regexp.MustCompile(`^tokn_test_[a-z0-9]{1,128}$`)
	omiseChargeID  = regexp.MustCompile(`^chrg_test_[a-z0-9]{1,128}$`)
	omisePrices    = map[string]int64{
		"flight_ticket": 459000,
		"hotel_room":    325000,
		"physical":      249000,
		"digital":       75000,
	}
)

type omiseChargeResult struct {
	Status      string `json:"status"`
	ChargeID    string `json:"chargeId,omitempty"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	FailureCode string `json:"failureCode,omitempty"`
}

func unknownOmiseCharge(amount int64) omiseChargeResult {
	return omiseChargeResult{Status: "unknown", Amount: amount, Currency: omiseCurrency}
}

type omiseClient struct {
	secretKey string
	http      *http.Client
}

func (client omiseClient) charge(ctx context.Context, runID, token string, amount int64) omiseChargeResult {
	result := unknownOmiseCharge(amount)
	if client.http == nil || !omiseSecretKey.MatchString(client.secretKey) || !omiseCardToken.MatchString(token) ||
		!validID.MatchString(runID) || amount <= 0 {
		return result
	}
	form := url.Values{
		"amount":   {strconv.FormatInt(amount, 10)},
		"currency": {omiseCurrency},
		"card":     {token},
		"ref_id":   {runID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, omiseChargesURL, strings.NewReader(form.Encode()))
	if err != nil {
		return result
	}
	req.GetBody = nil
	req.SetBasicAuth(client.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Omise-Version", omiseAPIVersion)

	response, err := client.http.Do(req)
	if err != nil {
		return result
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 500 ||
		response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests {
		return result
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxOmiseResponse+1))
	if err != nil || len(body) > maxOmiseResponse {
		return result
	}
	var charge struct {
		Object      string `json:"object"`
		ID          string `json:"id"`
		LiveMode    *bool  `json:"livemode"`
		Amount      int64  `json:"amount"`
		Currency    string `json:"currency"`
		Status      string `json:"status"`
		Paid        *bool  `json:"paid"`
		RefID       string `json:"ref_id"`
		FailureCode string `json:"failure_code"`
		ErrorCode   string `json:"code"`
	}
	if err := json.Unmarshal(body, &charge); err != nil {
		return result
	}
	if response.StatusCode >= 400 {
		if charge.Object == "error" && knownOmiseRejection(charge.ErrorCode) {
			result.Status, result.FailureCode = "failed", charge.ErrorCode
		}
		return result
	}
	if response.StatusCode >= 300 || charge.Object != "charge" || charge.LiveMode == nil || *charge.LiveMode ||
		!omiseChargeID.MatchString(charge.ID) || charge.Amount != amount ||
		!strings.EqualFold(charge.Currency, omiseCurrency) || (charge.RefID != "" && charge.RefID != runID) {
		return result
	}
	switch charge.Status {
	case "successful", "pending", "failed":
		if charge.Paid != nil && *charge.Paid != (charge.Status == "successful") {
			return result
		}
		result.Status, result.ChargeID = charge.Status, charge.ID
	case "expired", "reversed":
		result.Status, result.ChargeID, result.FailureCode = "failed", charge.ID, charge.Status
	default:
		return result
	}
	if result.Status == "failed" && knownOmiseFailure(charge.FailureCode) {
		result.FailureCode = charge.FailureCode
	}
	return result
}

func knownOmiseRejection(code string) bool {
	return slices.Contains([]string{
		"authentication_failure", "key_expired_error", "locked_account_error",
		"not_authorized", "brand_not_supported", "invalid_amount", "invalid_card",
		"invalid_card_token", "missing_card", "missing_return_uri",
		"not_found", "used_token",
	}, code)
}

func knownOmiseFailure(code string) bool {
	return slices.Contains([]string{
		"confirmed_amount_mismatch", "failed_fraud_check", "failed_processing",
		"insufficient_balance", "insufficient_fund", "invalid_account_number", "invalid_account",
		"payment_cancelled", "payment_rejected", "expired_card", "stolen_or_lost_card",
		"timeout", "incorrect_security_code",
	}, code)
}
