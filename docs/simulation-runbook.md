# Simulation Lab runbook

Follow the [local startup steps](../README.md). PostgreSQL initializes its demo tables and QA schema when Go starts.

## Local keys and Omise sandbox preparation

Docker Compose reads the repository-root `.env`. Create it from the [example template](../.env.example) if it does not already exist.

```sh
test -e .env || (umask 077; cp .env.example .env)
```

Run `openssl rand -hex 32` separately for each demo merchant you want to enable, then assign each generated value to its matching `PAYGUARD_DEMO_MERCHANT_A_TOKEN` or `PAYGUARD_DEMO_MERCHANT_B_TOKEN` in `.env`. The template also has optional `PAYGUARD_IPAPI_IS_KEY` and `PAYGUARD_PROXYCHECK_KEY` fields. Set `PAYGUARD_ENRICHMENT_ENABLED=true` to enable outbound provider lookups. Local masters work with the default setting.

After changing the keys for a running Compose stack, apply the new environment with `docker compose up -d --wait backend` from the repository root.

Go reads exported variables when running directly on the host. From the repository root, source your trusted shell-compatible `.env` if you use one. You can keep using an existing trusted `backend/.env` as your source.

```sh
(
  set -a
  . ./.env || exit
  set +a
  cd backend && go run .
)
```

The commented Omise variables are proposed for the next sandbox integration and the current application does not read them. Get the test public and secret keys from **Omise Dashboard → Test mode → Keys**. The future host frontend would read `NEXT_PUBLIC_OMISE_TEST_PUBLIC_KEY` from `frontend/.env.local`. Keep `PAYGUARD_OMISE_TEST_SECRET_KEY` in the backend's private environment. [Finding Omise API keys](https://docs.omise.co/how-to-access-omise-api-keys)

The planned sandbox flow uses Omise.js in the browser to tokenize an official test card. Go receives the token and uses the test secret key to create a sandbox charge. [Omise authentication](https://docs.omise.co/api-authentication), [Omise testing](https://docs.omise.co/api-testing)

## Booking event intake

PG-01 adds `POST /api/v1/booking/events` on the Go API at `http://127.0.0.1:8081`. Set distinct local tokens in `PAYGUARD_DEMO_MERCHANT_A_TOKEN` and `PAYGUARD_DEMO_MERCHANT_B_TOKEN` before starting Compose. An unset token disables access for its demo merchant. Send the token as `Authorization: Bearer <token>` and supply one `Idempotency-Key` header with 1–128 printable ASCII characters and no spaces.

Send a JSON object with a lowercase UUID `eventId`, `schemaVersion: 1`, `stage: "booking"`, and an RFC 3339 `occurredAt` at microsecond precision or coarser. Optionally include a nonnegative integer `amountMinor`. The response includes the authenticated `merchantId` and persisted `receivedAt`. A missing amount is omitted and an explicit zero is retained.

The API returns 201 on creation, 200 for an identical replay, 409 when either identifier is reused with conflicting request details, 400 for invalid inputs, and 401 for invalid or missing credentials. Both identifiers are scoped to the authenticated merchant.

| Optional variable | Default |
| --- | --- |
| `DATABASE_URL` | `postgres://payguard:payguard@127.0.0.1:55432/payguard?sslmode=disable` |
| `REDIS_URL` | `redis://127.0.0.1:56379/0` |
| `PAYGUARD_ADDR` | `127.0.0.1:8081` |
| `ENTITY_KEY_SECRET` | Built-in local fixture key. Keep the same key for deterministic generation. |
| `PAYGUARD_API_URL` (Next.js) | `http://127.0.0.1:8081` |

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

## Provider enrichment

`POST /api/v1/enrichment` is available directly on the Go API at port 8081. Authenticate with an existing `PAYGUARD_DEMO_MERCHANT_A_TOKEN` or `PAYGUARD_DEMO_MERCHANT_B_TOKEN` as a bearer token. Missing or invalid authentication returns 401. Local BIN and email-domain lookups work with the default environment. Set `PAYGUARD_ENRICHMENT_ENABLED=true` before starting Go or Compose to allow optional external IP and BIN lookups.

| Backend variable | Purpose |
| --- | --- |
| `PAYGUARD_ENRICHMENT_ENABLED` | Defaults to `false`. Controls external provider calls only. |
| `PAYGUARD_IPAPI_IS_KEY` | Optional ipapi.is key. |
| `PAYGUARD_PROXYCHECK_KEY` | Optional proxycheck.io key. |
| `PAYGUARD_ENRICHMENT_CACHE_SECRET` | Optional HMAC secret for stable cache keys across backend restarts. Go generates a random secret for each process if unset. |
| `PAYGUARD_EMAIL_MASTER_LOCAL` | Optional path to a local domain overlay. With the variable unset, Go loads `masterdata/local/domains.txt.gz` relative to its working directory if present. |

### Prepare the local masters

With PostgreSQL running, import the pinned snapshot into `enrichment_bin_master` and `enrichment_bin_snapshot`. Importing the same snapshot again reuses the stored data. The command honors `DATABASE_URL` and defaults to the loopback Compose database.

```sh
(cd backend && go run ./cmd/import-bin-master)
```

If using Docker for the backend, run the importer from the locally built image after PostgreSQL is up.

```sh
docker compose run --rm --entrypoint /usr/local/bin/payguard-import-bin-master backend
```

The backend loads the bundled email-domain master on the first email-domain lookup. To regenerate the snapshots and add the optional local-only feeds, run the refresh command and restart the backend. Compose mounts `backend/masterdata/local` read-only at `/masterdata/local`. For a custom path in Compose, use the path as seen by the container.

```sh
(cd backend && go run ./cmd/refresh-email-master -local)
```

### Request and response

Send a JSON object with one or more string fields: `ip`, `bin`, or `emailDomain`. Use a literal IPv4 or IPv6 address, 6–8 ASCII digits for the BIN, and a bare domain for email. The API rejects a full card number, a full email address, and unknown fields such as `rule` or `stage`. The response contains lookup evidence only.

Each requested kind returns an object with `status` (`OK` or `UNKNOWN`), `lookedUpAt`, and `cached`. Optional fields are `reason`, `provider`, `dataTimestamp`, and normalized `data`. Omitted flags are unknown, and explicit `false` flags stay visible. Backend logs record the lookup kind, status, provider, timestamps, and cache status.

| Response object | Evidence |
| --- | --- |
| `ip.data` | Available normalized flags, risk score, and country code. |
| `bin.data` | Canonical exact-prefix evidence with `scheme`, `funding`, `issuer`, and `countryCode` when known. `bin.fieldSources` identifies the source of each populated field. |
| `bin.master` | `FOUND`, `NOT_FOUND`, or `UNKNOWN`, with `EXACT` or `PREFIX`, `matchedPrefix`, dataset `source` and `revision`, and `recordSource` and `seen` when available. A shorter match uses `reference` with the same BIN field names. |
| `email.data` | Normalized `domain`, a `listed` boolean, and optional `matchedDomain` and `sources` for the most specific listed domain or parent. |

An exact PostgreSQL record supplies its known fields first. When enabled, external evidence for the requested BIN may fill gaps. For a 7–8 digit request, the provider must echo the exact digits. A shorter master match stays in `bin.master.reference`. All four known fields yield `bin.status: "OK"`. An incomplete lookup returns `UNKNOWN` and can still include the known exact fields.

The following request consumes no provider quota. The TEST-NET IP returns `UNKNOWN` with `reason: "NON_PUBLIC_IP"`. The bundled snapshot returns `email.status: "OK"` and lists the parent `mailinator.com` in `email.data`.

```sh
curl -fsS http://127.0.0.1:8081/api/v1/enrichment \
  -H "Authorization: Bearer ${PAYGUARD_DEMO_MERCHANT_A_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"ip":"192.0.2.10","emailDomain":"trial.mailinator.com"}'
```

After importing the master, use `{"bin":"424242"}` for a complete exact local lookup. With external providers disabled, use `{"bin":"42424299"}` to see `bin.status: "UNKNOWN"` with the shorter `424242` record under `bin.master.reference`.

See the [lookup behavior, cache policy and provider terms](requirements/provider-enrichment.md).
