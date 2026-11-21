package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type omiseChargeStore interface {
	get(context.Context, string, string) (run, error)
	reserveOmiseCharge(context.Context, string, string, int64) (bool, error)
	readOmiseCharge(context.Context, string, string) (omiseChargeRecord, error)
	finishOmiseCharge(context.Context, string, string, omiseChargeResult) error
}

type omiseCheckout struct {
	store     omiseChargeStore
	publicKey string
	provider  omiseClient
}

func newOmiseCheckout(store omiseChargeStore, publicKey, secretKey string) omiseCheckout {
	checkout := omiseCheckout{store: store}
	if omisePublicKey.MatchString(publicKey) && omiseSecretKey.MatchString(secretKey) {
		checkout.publicKey = publicKey
		checkout.provider = omiseClient{
			secretKey: secretKey,
			http: &http.Client{
				Timeout:       omiseCallTimeout,
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			},
		}
	}
	return checkout
}

func (checkout omiseCheckout) config(w http.ResponseWriter, _ *http.Request) {
	respond(w, 200, struct {
		Enabled   bool             `json:"enabled"`
		PublicKey string           `json:"publicKey,omitempty"`
		Currency  string           `json:"currency"`
		Prices    map[string]int64 `json:"prices"`
	}{checkout.publicKey != "", checkout.publicKey, omiseCurrency, omisePrices})
}

func checkoutAmount(saved run) (int64, bool) {
	price, knownProduct := omisePrices[saved.Config.Product]
	quantity := saved.Config.Quantity
	if !knownProduct || quantity < 1 || quantity > 20 || len(saved.Decisions) == 0 {
		return 0, false
	}
	orderID := saved.Decisions[0].Event.OrderID
	if orderID == "" {
		return 0, false
	}
	stage := "PAYMENT"
	if saved.Config.Product == "flight_ticket" || saved.Config.Product == "hotel_room" {
		stage = "PRE_PAYMENT"
	}
	for _, d := range saved.Decisions {
		if d.Event.OrderID == orderID && d.Event.Stage == stage {
			if d.Action == "ALLOW" && d.Event.Product == saved.Config.Product && d.Event.Quantity == quantity {
				return price * int64(quantity), true
			}
			break
		}
	}
	return 0, false
}

func (checkout omiseCheckout) create(w http.ResponseWriter, r *http.Request) {
	if checkout.publicKey == "" {
		problem(w, 503, "Omise test checkout is unavailable.")
		return
	}
	var request struct {
		RunID string `json:"runId"`
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF ||
		!validID.MatchString(request.RunID) || !omiseCardToken.MatchString(request.Token) {
		problem(w, 400, "Expected a saved run ID and an Omise test-mode token.")
		return
	}

	dbCtx, cancelDB := context.WithTimeout(r.Context(), omiseWriteTimeout)
	defer cancelDB()
	saved, err := checkout.store.get(dbCtx, merchantID, request.RunID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "Run not found.")
		return
	case err != nil:
		checkoutStorageUnavailable(w)
		return
	}
	amount, eligible := checkoutAmount(saved)
	if !eligible {
		problem(w, 409, "Checkout requires an ALLOW decision for the first order's payment stage.")
		return
	}
	owner, err := checkout.store.reserveOmiseCharge(dbCtx, merchantID, request.RunID, amount)
	if err != nil {
		checkoutStorageUnavailable(w)
		return
	}
	cancelDB()
	if !owner {
		checkout.replay(w, r, request.RunID)
		return
	}

	// After the durable reservation, finish the attempt even if the browser disconnects.
	callCtx, cancelCall := context.WithTimeout(context.WithoutCancel(r.Context()), omiseCallTimeout)
	result := checkout.provider.charge(callCtx, request.RunID, request.Token, amount)
	cancelCall()
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(r.Context()), omiseWriteTimeout)
	defer cancelSave()
	if err := checkout.store.finishOmiseCharge(saveCtx, merchantID, request.RunID, result); err != nil {
		log.Print("Unable to finalize an Omise test checkout reservation.")
		result = unknownOmiseCharge(amount)
	}
	respond(w, 200, result)
}

func checkoutStorageUnavailable(w http.ResponseWriter) {
	log.Print("Omise test checkout storage is unavailable.")
	problem(w, 503, "Omise test checkout storage is unavailable.")
}

func (checkout omiseCheckout) replay(w http.ResponseWriter, r *http.Request, runID string) {
	ctx, cancel := context.WithTimeout(r.Context(), omiseAttemptWindow)
	defer cancel()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	var record omiseChargeRecord
	for {
		current, err := checkout.store.readOmiseCharge(ctx, merchantID, runID)
		if err != nil {
			if record.Result.Amount > 0 && errors.Is(err, context.DeadlineExceeded) {
				respond(w, 200, record.Result)
				return
			}
			checkoutStorageUnavailable(w)
			return
		}
		record = current
		if record.FinishedAt != nil || time.Since(record.StartedAt) >= omiseAttemptWindow {
			respond(w, 200, record.Result)
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			respond(w, 200, record.Result)
			return
		}
	}
}
