package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	bookingTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)
	bookingKeyPattern   = regexp.MustCompile(`^[!-~]{1,128}$`)
	errBookingConflict  = errors.New("booking event ID or idempotency key is already in use")
)

type bookingAuth struct {
	tokens map[string][sha256.Size]byte
}

func newBookingAuth(tokenA, tokenB string) (bookingAuth, error) {
	if tokenA != "" && tokenA == tokenB {
		return bookingAuth{}, errors.New("demo merchant bearer tokens must be distinct")
	}
	auth := bookingAuth{tokens: make(map[string][sha256.Size]byte)}
	for tenant, token := range map[string]string{"demo-merchant-a": tokenA, "demo-merchant-b": tokenB} {
		if token == "" {
			continue
		}
		if !bookingTokenPattern.MatchString(token) {
			return bookingAuth{}, errors.New("demo merchant token contains invalid bearer token characters")
		}
		auth.tokens[tenant] = sha256.Sum256([]byte(token))
	}
	return auth, nil
}

func (a bookingAuth) merchant(headers []string) string {
	if len(headers) != 1 {
		return ""
	}
	scheme, token, ok := strings.Cut(headers[0], " ")
	token = strings.TrimLeft(token, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || !bookingTokenPattern.MatchString(token) {
		return ""
	}
	presented := sha256.Sum256([]byte(token))
	var merchant string
	for tenant, expected := range a.tokens {
		if subtle.ConstantTimeCompare(presented[:], expected[:]) == 1 {
			merchant = tenant
		}
	}
	return merchant
}

func (a bookingAuth) requireMerchant(w http.ResponseWriter, r *http.Request) string {
	merchant := a.merchant(r.Header.Values("Authorization"))
	if merchant == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		problem(w, http.StatusUnauthorized, "A valid demo merchant bearer token is required.")
	}
	return merchant
}

type bookingInput struct {
	EventID       string    `json:"eventId"`
	SchemaVersion int       `json:"schemaVersion"`
	Stage         string    `json:"stage"`
	OccurredAt    time.Time `json:"occurredAt"`
	AmountMinor   *int64    `json:"amountMinor,omitempty"`
	Product       string    `json:"product,omitempty"`
	SDKStatus     string    `json:"sdkStatus,omitempty"`
}

func validBookingProduct(product string) bool {
	switch product {
	case "", "flight_ticket", "hotel_room", "physical", "digital":
		return true
	default:
		return false
	}
}

func validBookingTime(value time.Time) bool {
	return !value.IsZero() && value.UTC().Year() >= 1 && value.UTC().Year() <= 9999 &&
		value.Nanosecond()%1000 == 0
}

type bookingEvent struct {
	bookingInput
	MerchantID string    `json:"merchantId"`
	ReceivedAt time.Time `json:"receivedAt"`
}

type bookingStore interface {
	saveBooking(context.Context, string, string, bookingInput) (bookingEvent, bool, error)
}

type bookingHandler struct {
	store bookingStore
	auth  bookingAuth
}

func (h bookingHandler) create(w http.ResponseWriter, r *http.Request) {
	merchant := h.auth.requireMerchant(w, r)
	if merchant == "" {
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !bookingKeyPattern.MatchString(keys[0]) {
		problem(w, http.StatusBadRequest, "Expected one Idempotency-Key of 1 to 128 printable ASCII characters without spaces.")
		return
	}
	var input bookingInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		problem(w, http.StatusBadRequest, "Expected a valid booking event JSON object.")
		return
	}
	if !validID.MatchString(input.EventID) || input.SchemaVersion != 1 || input.Stage != bookingStage ||
		!validBookingTime(input.OccurredAt) || (input.AmountMinor != nil && *input.AmountMinor < 0) {
		problem(w, http.StatusBadRequest, "Expected a lowercase event UUID, schemaVersion 1, booking stage, an RFC3339 timestamp with microsecond precision, and an optional nonnegative integer amountMinor.")
		return
	}
	if !validBookingProduct(input.Product) || (input.SDKStatus != "" && input.SDKStatus != "observed" && input.SDKStatus != "unavailable") {
		problem(w, http.StatusBadRequest, "Expected a supported product and SDK status observed or unavailable when supplied.")
		return
	}
	saved, created, err := h.store.saveBooking(r.Context(), merchant, keys[0], input)
	switch {
	case errors.Is(err, errBookingConflict):
		problem(w, http.StatusConflict, "Booking event ID or idempotency key is already used by a different request.")
	case err != nil:
		log.Printf("booking storage error: %v", err)
		problem(w, http.StatusServiceUnavailable, "Booking storage is temporarily unavailable. Please retry.")
	case created:
		respond(w, http.StatusCreated, saved)
	default:
		respond(w, http.StatusOK, saved)
	}
}

func (r repository) saveBooking(ctx context.Context, merchant, key string, input bookingInput) (bookingEvent, bool, error) {
	saved := bookingEvent{MerchantID: merchant, bookingInput: input}
	saved.OccurredAt = input.OccurredAt.UTC()
	err := r.db.QueryRow(ctx, `
		INSERT INTO booking_events (merchant_id, event_id, idempotency_key, schema_version, stage, occurred_at, amount_minor, product, sdk_status)
		VALUES ($1, $2::text::uuid, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING
		RETURNING received_at`,
		merchant, input.EventID, key, input.SchemaVersion, input.Stage, input.OccurredAt, input.AmountMinor, input.Product, input.SDKStatus).Scan(&saved.ReceivedAt)
	if err == nil {
		saved.ReceivedAt = saved.ReceivedAt.UTC()
		return saved, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return bookingEvent{}, false, err
	}

	// Use a new statement so a competing insert that won the unique constraint
	// is visible. Matching both identifiers rules out changing the key on replay.
	err = r.db.QueryRow(ctx, `
		SELECT schema_version, stage, occurred_at, amount_minor, received_at, product, sdk_status
		FROM booking_events
		WHERE merchant_id = $1 AND event_id = $2::text::uuid AND idempotency_key = $3`,
		merchant, input.EventID, key).Scan(&saved.SchemaVersion, &saved.Stage, &saved.OccurredAt, &saved.AmountMinor, &saved.ReceivedAt, &saved.Product, &saved.SDKStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingEvent{}, false, errBookingConflict
	}
	if err != nil {
		return bookingEvent{}, false, err
	}
	if saved.SchemaVersion != input.SchemaVersion || saved.Stage != input.Stage ||
		!saved.OccurredAt.Equal(input.OccurredAt) || !sameBookingAmount(saved.AmountMinor, input.AmountMinor) ||
		saved.Product != input.Product || saved.SDKStatus != input.SDKStatus {
		return bookingEvent{}, false, errBookingConflict
	}
	saved.OccurredAt = saved.OccurredAt.UTC()
	saved.ReceivedAt = saved.ReceivedAt.UTC()
	return saved, false, nil
}

func sameBookingAmount(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
