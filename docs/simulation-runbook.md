# Simulation Lab runbook

Follow the [local startup steps](../README.md). PostgreSQL initializes its demo tables and QA schema when Go starts.

## Booking event intake

PG-01 adds `POST /api/v1/booking/events` on the Go API at `http://127.0.0.1:8081`. Set distinct local tokens in `PAYGUARD_DEMO_MERCHANT_A_TOKEN` and `PAYGUARD_DEMO_MERCHANT_B_TOKEN` before starting Compose. An unset token disables access for its demo merchant. Send the token as `Authorization: Bearer <token>` and supply one `Idempotency-Key` header with 1–128 printable ASCII characters and no spaces.

Send a JSON object with a lowercase UUID `eventId`, `schemaVersion: 1`, `stage: "booking"`, and an RFC 3339 `occurredAt` at microsecond precision or coarser. Optionally include a nonnegative integer `amountMinor`. The response includes the authenticated `merchantId` and persisted `receivedAt`. A missing amount is omitted and an explicit zero is retained.

The optional `product` accepts `flight_ticket`, `hotel_room`, `physical`, or `digital`. The optional `sdkStatus` accepts `observed` or `unavailable`. The lineage API reports `unknown` when the booking did not supply an SDK status. Both fields participate in booking replay validation.

The API returns 201 on creation, 200 for an identical replay, 409 when either identifier is reused with conflicting request details, 400 for invalid inputs, and 401 for invalid or missing credentials. Both identifiers are scoped to the authenticated merchant.

| Optional variable | Default |
| --- | --- |
| `DATABASE_URL` | `postgres://payguard:payguard@127.0.0.1:55432/payguard?sslmode=disable` |
| `REDIS_URL` | `redis://127.0.0.1:56379/0` |
| `PAYGUARD_ADDR` | `127.0.0.1:8081` |
| `ENTITY_KEY_SECRET` | Built-in local fixture key. Keep the same key for deterministic generation. |
| `PAYGUARD_API_URL` (Next.js) | `http://127.0.0.1:8081` |
| `PAYGUARD_BOOKING_API_URL` (Next.js) | Uses `PAYGUARD_API_URL` by default. Set this to route booking calls to a separate local Go backend. |
| `PAYGUARD_DEMO_BASELINE_MIN_SAMPLES` (Go and Compose) | Activation disabled when unset. Set a positive integer to enable baseline publication for a local demo. |

## Booking assessment

PG-03 through PG-06 use the same configured demo merchant tokens as [booking intake](#booking-event-intake). Save the business model and booking signals in Merchant Setup. Open `/settings/booking-assessment` on the frontend, enter your local demo token, generate a proposal, and approve it. Then submit a synthetic booking with the normal mock Jev response or simulated timeout. [Requirements](requirements/booking-assessment.md) · [UI](ui/booking-assessment.md)

The API uses the following request bodies. Send the same bearer credentials on each request, and include an idempotency key when creating the booking.

| Method and path | Body and response |
| --- | --- |
| `POST /api/v1/booking/profiles/proposal` | Send `{schemaVersion, businessModel, stage, availableSignals}`. The response contains `{setup, candidate}`. The deterministic candidate contains `{schemaVersion, stage, signals}`. |
| `POST /api/v1/booking/profiles/approve` | Send the returned `{setup, candidate}`. Approval activates the persisted profile version and returns `profileVersion`. |
| `POST /api/v1/booking/events` | Create a booking as described above and keep its `eventId`. Omit `amountMinor` for missing and use `0` for explicit zero. |
| `POST /api/v1/booking/events/{id}/assessment` | Send `{"jevMode":"mock"}` or `{"jevMode":"timeout"}` for a stored event belonging to the same merchant. The API returns 201 for the first assessment, 200 for a same-mode replay, or 409 if the event was already assessed with a different mode or no profile is active. The response carries the pinned lineage, feature evidence, baseline state, and mock Jev result. |

To check lineage, assess a booking and save the response, approve a different profile, then resend the original assessment request. Check that the replay still returns the original profile version. To check timeout, assess a fresh booking with `jevMode` set to `timeout` and verify the `UNRESOLVED` result and `MANUAL_REVIEW` handoff.

To check velocity, create five synthetic bookings for the same merchant with event timestamps less than 60 seconds apart. Assess the fifth booking and look for `bookingVelocity60s` equal to `5`, `BOOKING_VELOCITY_60S` in the rule hits, and Jev answer `ELEVATED` in mock mode. Use an unused event-time interval for a clean fixture history. Bookings exactly 60 seconds before the target, future bookings, and bookings received after the target are outside the count. An approved profile with `amountMinor` includes evidence with `presence` equal to `MISSING` and `value` equal to `null` for an omitted amount, or `OBSERVED` and `0` for an explicit zero.

## Mock checkout and booking lineage

Open `/settings/mock-checkout` on the local frontend. Pick an OTA or e-commerce product, enter the configured demo token, and approve the demo profile for that business model. Submit the checkout and read the assessment next to the booking receipt. Repeat with **Simulate timeout** and confirm the unresolved result and manual-review handoff. [Checkout UI](ui/mock-checkout.md) · [Requirements](requirements/checkout-and-lineage.md)

Send the same demo bearer credentials as booking intake for both lineage operations.

| Method and path | Body and response |
| --- | --- |
| `GET /api/v1/booking/events/{id}/lineage` | Returns `{booking, assessment, outcomes}` for the authenticated merchant. An unassessed booking has `assessment: null`. Outcomes appear in `occurredAt` order, with receipt time and outcome ID resolving ties. |
| `POST /api/v1/booking/events/{id}/outcomes` | Send a lowercase UUID `outcomeId`, a `source` of `verified_demo`, `analyst`, or `chargeback`, a stable `sourceReference`, a `label` of `normal`, `fraud`, or `inconclusive`, and RFC 3339 `occurredAt` at or after booking event time. Returns `{outcome, baseline}` with 201 on creation, 200 on identical replay, or 409 for a conflicting ID or source reference. |

The baseline response includes `state`, `version`, and `sampleCount`. The activation threshold for the local preview is `3`. A qualifying `verified_demo` normal outcome requires a completed assessment in `mock` mode and counts at most once per booking. Analyst and chargeback outcomes, inconclusive and fraud labels, and timeout assessments preserve the baseline. An outcome import does not change an existing assessment or activate rules. Subsequent bookings pin the current baseline when accepted.

For a repeatable local check, configure both distinct demo merchant tokens in your shell and backend, set the baseline threshold, and run the smoke script against an isolated demo stack. The script writes synthetic bookings and outcomes and can publish a demo baseline.

```sh
python3 scripts/booking-smoke.py --base http://127.0.0.1:8081 --threshold 3
```

## HTTP contract

| Method and path | Behavior |
| --- | --- |
| `GET /healthz` | Checks PostgreSQL and Redis |
| `POST /api/v1/simulation/runs` | Accepts `{id, config}`, evaluates a run, or returns a saved run for an unchanged request |
| `GET /api/v1/simulation/runs/{id}` | Returns saved evidence and version identifiers |
| `GET /api/v1/simulation/runs/{id}/report` | Compares saved payment decisions with synthetic QA labels and returns aggregates |

The run ID is a lowercase UUID. The POST returns 400 for a malformed body, 422 for invalid scenario settings, 409 for a conflicting run ID, and 503 for a failed dependency. A dependency failure does not create a successful run. The report returns null for an undefined ratio and includes timeout samples in `unscored`. These metrics are from seeded fixtures.

`config.contactEmailDomain` is optional and accepts a hostname under the synthetic `.test` domain. Go normalizes it before matching policy and checking replay. Only the first order uses this domain at both stages. Omit the field to keep the original seeded domains and support existing requests. The browser holds the full contact during the session and sends only the domain in the simulation request.

```sh
curl -fsS http://127.0.0.1:8081/healthz
curl -fsS http://127.0.0.1:8081/api/v1/simulation/runs \
  -H 'Content-Type: application/json' \
  -d '{"id":"498ab33a-c10d-45f9-8db9-7715f2e0f8b4","config":{"seed":42,"product":"flight_ticket","scenario":"mixed","accounts":12,"rate":30,"quantity":1,"date":"2026-12-04","rotateIp":true,"sharedIp":true,"sharedDevice":true,"sharedToken":true,"retry":true}}'
curl -fsS http://127.0.0.1:8081/api/v1/simulation/runs/498ab33a-c10d-45f9-8db9-7715f2e0f8b4/report
```

## Checks

```sh
(cd backend && go test -race ./... && go vet ./...)
(cd frontend && npm ci && npm run typecheck && npm run build)
```

The example produces 24 decisions, skips 12 duplicate PSP retries, and reports 12 payment samples. For shared Wi-Fi regression, set traffic to Normal and keep Shared legitimate Wi-Fi enabled. For missing-token and timeout regression, use the corresponding controls in Traffic signals and edge cases. The former displays `payment_token` in missing evidence at payment stages, and the latter adds unscored samples in Offline QA.

### Verify checkout contact and merchant policy

1. Select **OTA → Flight ticket** and **Normal** traffic. Set **Accounts** to `1`. Leave **Omit payment token** and **Mock Jev timeout** off.
2. Enter a synthetic contact such as **Test Buyer**, `buyer@restricted.test`, and `+1 202-555-0142`. Enable **Block flights for restricted.test**, then run the scenario. In the first order's **PRE PAYMENT** evidence, check that the contact is visible, **Event email domain** reads `restricted.test`, the merchant policy and final action are `BLOCK`, and the final action source is `MERCHANT_POLICY`. The platform decision is `ALLOW` for this fixture.
3. Change the email to `buyer@traveler.test` and run again. Check that the contact reflects the new submission, **Event email domain** reads `traveler.test`, the merchant policy is `PASS`, and the final action is `ALLOW`. A subdomain such as `sub.restricted.test` also passes because the rule matches the exact hostname.
4. Clear the contact and run again to use the original seeded domain. To verify product and stage boundaries, inspect **BOOKING**, disable the policy before rerunning, or repeat with a hotel or e-commerce product. Merchant policy is `NOT_APPLICABLE` in those cases.

## System Rules

Use [System Rules](http://127.0.0.1:3000/settings/system-rules) for the platform rule dashboard. Save a synthetic run in Simulation Lab, then create a draft with a payment-stage condition such as `device_orders >= 2` and action `REVIEW`. Select the saved run to preview the draft. Use the returned event IDs to compare matches against the run's evidence. Check that the active built-ins and recorded decisions remain the same after saving and previewing. See the [scope and acceptance criteria](requirements/system-rules.md).

| Method and path | Behavior |
| --- | --- |
| `GET /api/v1/system/rules` | Lists active built-ins and stored drafts, supported signals, and recent saved runs. |
| `POST /api/v1/system/rules` | Creates a draft with one to five AND conditions. |
| `PUT /api/v1/system/rules/{id}` | Updates a draft using the current version. Refresh the list after a version conflict. |
| `POST /api/v1/system/rules/preview` | Accepts `{runId, rule}` and returns the run ID, rule version, eligible, matched, and skipped counts, and matching event IDs. |

```sh
curl -fsS http://127.0.0.1:8081/api/v1/system/rules
curl -fsS http://127.0.0.1:8081/api/v1/system/rules/preview \
  -H 'Content-Type: application/json' \
  -d '{"runId":"498ab33a-c10d-45f9-8db9-7715f2e0f8b4","rule":{"name":"Device velocity","description":"Review repeat device orders at payment","stage":"PAYMENT_STAGES","action":"REVIEW","conditions":[{"signal":"device_orders","operator":"gte","value":2}]}}'
```

The example uses the saved run ID from the earlier API example. The preview counts stage-matching events with all required signals as eligible and stage-matching events missing any required signal as skipped. Matching IDs identify events for which every condition passed.
