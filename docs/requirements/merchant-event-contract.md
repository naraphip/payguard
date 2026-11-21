# Merchant event intake contract

Status **Production contract proposal.** The booking intake in this repository supports Booking Event v1. The pasted merchant data synthesis supplied in chat describes a broader event contract that requires product and stage decisions before publication. [Booking intake runbook](../simulation-runbook.md#booking-event-intake) · [Team Blueprint core fields](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260105)

## Booking Event v1

`POST /api/v1/booking/events` requires a configured demo merchant bearer token in the `Authorization` header and one `Idempotency-Key` header. The server resolves the merchant from that token.

| Field | Current requirement |
| --- | --- |
| `eventId` | Required lowercase UUID |
| `schemaVersion` | Required integer `1` |
| `stage` | Required value `booking` |
| `occurredAt` | Required RFC 3339 timestamp with at most microsecond precision |
| `amountMinor` | Optional nonnegative integer. Omitted and zero are distinct. |
| `product` | Optional. Accepted values are `flight_ticket`, `hotel_room`, `physical`, and `digital`. |
| `sdkStatus` | Optional. Accepted values are `observed` and `unavailable`. |

The decoder rejects unknown fields. Use the v1 fields above for this route. The proposed snake-case envelope and nested customer, device, order, and payment fields belong to a future contract. [Booking handler](../../backend/booking.go)

## Proposed production tiers

| Tier | Proposed data |
| --- | --- |
| Core envelope | Require `schema_version` (string), `event_id`, `idempotency_key`, `occurred_at` (RFC 3339), `event_type`, `stage`, and `vertical`. Derive `merchant_id` from verified authentication. Define the wire location of the target idempotency key in the OpenAPI contract. |
| Conditional core | Require `product_type` on product events. Require registered customer identity and account creation time when applicable, or a guest session reference for guest checkout. Use a session ID for web and app sessions, an order or intent reference when one exists, and `amount_minor` with `currency` for money events. Require `items[]` for itemized checkout and a payment attempt ID and method once payment starts. Obtain network IP from a trusted edge. |
| Optional fraud evidence | Accept available, authorized pseudonymous identity, verification results, customer history, device or network signals, bot and promotion signals, tokenized payment evidence, PSP checks, or trace IDs with documented provenance. Derive velocity and IP enrichment from trusted sources. Preserve missing, unknown, and observed values separately, including explicit zero and false. |
| Conditional product extensions | Validate each relevant item against a registered product module. A merchant can use multiple product types. Apply the product and stage matrix for the transaction. |

| Product | Proposed minimum when applicable | Optional or later evidence |
| --- | --- | --- |
| Flight | Origin and destination IATA codes, departure, itinerary type, passenger count | Carrier, passenger tokens, PNR, ticket status |
| Hotel | Property ID, check-in and check-out, room count | Room type, rate plan, refundable flag, guest token. Guest count is pending. |
| OTA | Channel ID and supplier ID | Partner ID, inventory source, merchant of record, payment collection model |
| Physical goods | SKU, category, quantity, unit price | Shipping method, delivery country, address token, recipient when shipping applies |
| Digital goods | Item and product type. Delivery method is pending. | Recipient token, activation count |
| Payment gateway | Payment intent, method, amount, currency | PSP result, 3DS, AVS, CVV check result |
| Marketplace | Seller ID and tenant mapping | Escrow, payout token, seller history |
| Subscription | Plan ID, contract ID, billing cycle | Trial dates and renewal number |

| Stage | Evidence to require when applicable |
| --- | --- |
| Pre-booking | Booking intent, customer or guest reference, and product details. Flight and hotel bookings that pay later can omit card and payment-token evidence. |
| Pre-payment | Order or intent, payment attempt and method, amount, and currency. Authorization evidence becomes available after this stage. |
| Post-auth | Payment reference, PSP transaction ID, and authorization status. |
| Refund | Original payment reference, refund amount, and reason. Reuse the saved order evidence. |
| Dispute | Original transaction reference and dispute or chargeback evidence. Reuse the saved checkout evidence. |

For an email-domain policy, derive the normalized domain from available authorized evidence. Report **Signal unavailable** when that evidence is missing. Use only normalized, minimized features and rule results in Jev context. Keep raw email, phone, card details, raw IP addresses, and QA ground truth out of Jev, and treat provider responses as evidence for the server’s decision. [Team Blueprint product modules](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260106) · [Required matrix](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260108)

## Decisions before publication

- Resolve `hotel.guest_count`. Product Modules places it in P0 for hotel rooms. The Required Matrix lists it under conditional or optional evidence for hotel pre-booking.
- Resolve `digital.delivery_method`. Product Modules assigns P1. The Required Matrix puts delivery method in the minimum required digital checkout payload.
- Specify how a mixed-product order validates each item and its product extension, including item and order amount consistency and separate delivery groups.
- Publish one versioned OpenAPI contract after reconciling the core envelope, product and stage matrix, provenance, missing-value semantics, and validation responses. Its examples should exercise guest checkout, a pay-later booking, pre-payment with field availability appropriate to that stage, observed zero versus missing evidence, and a mixed-product order. Check that each example passes the applicable schema and each missing required field is rejected. Keep Booking Event v1 covered by its existing contract tests.
