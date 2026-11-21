# Simulation Lab runbook

Follow the [local startup steps](../README.md). PostgreSQL initializes its demo tables and QA schema when Go starts.

The app opens Dashboard at `/dashboard`. Dashboard totals and recent transactions come from saved merchant records. Use `/simulation-lab` for Simulation Lab and `/settings/merchants` for the merchant directory.

## Local keys and Omise sandbox preparation

Docker Compose reads the repository-root `.env` for demo merchant tokens and optional enrichment settings. Create it from the [example template](../.env.example) if it does not already exist.

```sh
test -e .env || (umask 077; cp .env.example .env)
```

Run `openssl rand -hex 32` separately for each demo merchant you want to enable, then assign each generated value to its matching `PAYGUARD_DEMO_MERCHANT_A_TOKEN` or `PAYGUARD_DEMO_MERCHANT_B_TOKEN` in `.env`. The template also has optional `PAYGUARD_IPAPI_IS_KEY` and `PAYGUARD_PROXYCHECK_KEY` fields. Set `PAYGUARD_ENRICHMENT_ENABLED=true` to enable outbound provider lookups. Local masters work with the default setting.

For sandbox checkout, get the keys from **Omise Dashboard → Test mode → Keys** and put the pair in the ignored `backend/.env`. Set `OMISE_PUBLIC_KEY` to the `pkey_test_` key and `OMISE_SECRET_KEY` to the `skey_test_` key. Keep the secret key in the backend environment. Compose loads `backend/.env` for the Go service. After changing the file for a running stack, apply it with `docker compose up -d --wait backend`. [Finding Omise API keys](https://docs.omise.co/how-to-access-omise-api-keys)

When running Go directly on the host, export values from your trusted shell-compatible key file. From the repository root, run the following after starting PostgreSQL and Redis. If you also use the root `.env` for demo tokens or enrichment, export those variables before starting Go.

```sh
(
  cd backend || exit
  set -a
  . ./.env || exit
  set +a
  go run .
)
```

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

PG-03 through PG-06 use the same configured demo merchant tokens as [booking intake](#booking-event-intake). For this standalone profile walkthrough, leave `PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN` unset and save the business model and booking signals at `/settings/merchant-setup`. Open `/settings/booking-assessment` on the frontend, enter your local demo token, generate a proposal, and approve it. Then submit a synthetic booking with the normal mock Jev response or simulated timeout. [Requirements](requirements/booking-assessment.md) · [UI](ui/booking-assessment.md)

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

## Merchant scenarios and Jev

Set `PAYGUARD_DEMO_ADMIN_TOKEN` to a local demo value before starting or rebuilding Compose. Platform Admin can manage merchant settings and view transactions across active merchants. A merchant’s own credential opens its read-only transaction list and results.

For the host-run Next preview on loopback, set the Next server’s `PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN` to the same value as the API’s `PAYGUARD_DEMO_ADMIN_TOKEN`. Start Next and open `/settings/merchants` on the loopback preview. Merchants and the scenario pickers in Mock Checkout and Simulation Lab connect automatically as the demo administrator. The old `/settings/operations` route opens the merchant Transactions view. Set this variable only for the local preview.

For the standard token flow, leave the Next preview variable unset. Open `/settings/merchants` and enter the administrator credential as the **Demo access token**. To check merchant isolation, open the Merchant workspace and use the credential issued to that merchant.

If a valid merchant form returns **Expected one valid JSON request object.** after pulling changes, the host-run Next preview may be sending newer fields to an older backend image. From the current checkout, run `docker compose up -d --build --force-recreate --no-deps --wait backend` with the same project name, Compose files, and environment file options used to start the preview's backend. Reload **Merchants**, reopen the form, and retry the save. Verify that the saved details persist after reload and the merchant's **Transactions** tab opens.

### Check the default mock flow

1. Select **Create merchant** and enter a merchant name. Choose **OTA** or **E-commerce**, confirm the **Booking** stage and required booking timestamp, and select booking amount if the merchant provides it. Save the merchant and copy its scoped access token from the one-time confirmation if you plan to check merchant access. The automatic preview opens the new merchant workspace directly. An older merchant without setup details appears as **Setup incomplete** until the details are saved.
2. Select **Add scenario**. Create **Everyday bookings** with a review threshold of **100000** minor units, select **Generate rule preview**, and select **Save scenario and rules**. Repeat for **High-value bookings** with a threshold of **50000**. Editing a scenario and saving a new threshold creates a new revision.
3. In **Mock Checkout**, select the merchant and **Everyday bookings**, set the booking amount to **50000** minor units, and submit. Expect **Low risk**. Repeat with **High-value bookings** and expect **Review**. Confirm that the result identifies the scenario, rule revision, and checkout source. In the standard token flow, use **Connect merchant directory** before selecting a merchant.
4. In **Simulation Lab** at `/simulation-lab`, use **Merchant scenario simulation** and repeat the same selections and amount. Confirm the same outcomes and versions, with Simulation Lab recorded as the source. The existing batch simulation remains available on the same page.
5. Run either tool with a simulated Jev timeout for **High-value bookings**. Expect **Unresolved** and **Needs decision**. As Platform Admin, open **Transactions**, select **Needs decision**, and open the transaction. Clear or reject it to verify that the manual disposition appears separately from the saved mock decision.
6. Return to **Dashboard** and follow a recent transaction link to its saved detail. Open the detail again after changing its scenario’s rule and verify that it still shows the original version.
7. Create a second merchant and copy its token. In a separate browser session, choose **Merchant** in the workspace selector and enter that token. Verify that **Transactions** shows the second merchant's read-only ledger and **Merchant AI Rules** uses its own policy scope. Try a direct link to the first merchant’s transaction and an administrator page. The same merchant credential works in the standard token flow and the automatic preview.

### Enable live Jev for synthetic tests

Set `OPENROUTER_API_KEY` in the ignored root `.env` for Compose. In **Decision Engine → AI Configuration**, save the OpenRouter provider. Before a configuration version has been saved, `PAYGUARD_AI_MODE=live` enables the deployment default. Optionally set `OPENROUTER_MODEL_PRIMARY` to a versioned TypeSafe Jev model. The deployment default is `typesafe/jev-1.14`. For the standard local Compose stack, rebuild and recreate the backend to reload the environment. A separately named preview stack requires its own Compose project arguments. [Team Blueprint](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=1604066438) · [OpenRouter Decisions API for Jev](https://openrouter.ai/typesafe/jev-1.14)

```sh
docker compose up -d --build --force-recreate --wait backend
```

1. In **Mock Checkout**, choose a merchant and scenario, enter a synthetic booking amount, and submit with the timeout control off.
2. In **Simulation Lab**, use **Merchant scenario simulation** with the same merchant and scenario. Enter a synthetic amount and submit with the timeout control off. The batch simulator uses its own mock evaluator.
3. Open **Transactions** for the merchant and select each test. Look for **Jev answer**, **Jev model**, and **Jev decision ID**. A transaction evaluated against saved settings also records the Jev configuration version. Local threshold and merchant rule hits can raise the final assessment to **Review**. Compare **Automated result** with the displayed Jev answer.
4. Run a test with **Simulate timeout** on or omit the booking amount to exercise the **Unresolved** result. A real provider error also saves an unresolved transaction under **Needs decision**. An identical retry returns the saved result.

The Go server sends bounded booking evidence and keeps the credential server-side. Use synthetic bookings for this preview. To return to mock decisions, save Mock in **AI Configuration** when a saved version exists. With version `0`, set `PAYGUARD_AI_MODE=mock` and recreate the backend. The standalone Scoring Profile and legacy booking flows continue to use their own mock assessment.

### Rules for the selected merchant

As Platform Admin, open **Merchant Rules** for the selected merchant. Use the separate global **Platform Rules** workspace for platform drafts.

- In **Booking review rules**, create a rule using the booking amount, preview it against saved merchant transactions, and publish it. A new Mock Checkout or single-booking merchant scenario simulation uses the published rule and records the supplemental Jev ruleset version. Earlier results retain their recorded versions.
- The old merchant-specific platform draft links remain compatible. Those drafts require a saved Simulation Lab batch run for the same merchant. Use a saved shared run when submitting global Platform Rules for independent approval.

The standalone rule and Risk Operations routes remain available with the automatic preview disabled.

### Update or delete a merchant

Select the merchant in **Merchants** to edit its name or setup. The saved setup describes the merchant and its declared signals. Scenario rules remain separately configured under **Scenarios and rules**. The browser-local setup route at `/settings/merchant-setup` redirects to **Merchants** in the automatic preview. Outside that preview, the standalone setup and booking profile flows are still available.

To remove an unused demo merchant, open its details, select **Delete merchant**, and confirm. The merchant disappears from **Merchants** and from both test pickers. Its issued token can no longer open the workspace. Stored scenarios and transactions remain in the database and are unavailable through the ordinary workspace.

This path uses synthetic transactions and one structured threshold rule per scenario. The local review records a disposition without taking a payment action.

## Merchant Management

Sign in as Platform Admin and open `/settings/merchants`. Search by the merchant's name or ID, or sort by its creation date or last activity. Create a synthetic merchant, choose its supplied catalogue products, and select **Copy token** on the one-time issuance message. Open the merchant's detail URL and confirm the actual stored totals, effective policies, and audit entries.

To sign in to an existing merchant for a Sandbox walkthrough, open its **Overview** as Platform Admin and select **Get temporary token**. Select **Copy token**, or use **Show token** to view it. In a separate browser tab, choose the **Merchant** workspace, paste the token in the access form, and select **Connect**. The token expires an hour after issuance. The merchant’s original token remains valid.

Use `/settings/merchants/{merchantId}?tab=transactions` to open the ledger for a known merchant. The filters use transaction ID prefix, saved outcome, review state, and UTC dates. Open a saved transaction and check the separate platform-side result, Merchant Policy result, final decision, and review disposition. Missing evidence depends on the product, stage, and sources and displays only where recorded.

Verify isolation with two synthetic merchants. Sign in with one merchant credential and check that the other merchant's workspace or rule history is unavailable. With the administrator credential, inspect both merchants' rule history through their detail workspace. Use the selected merchant's scenario in Mock Checkout or Simulation Lab, then verify it feeds that merchant's existing ledger. Review the merchant audit after a metadata change, policy revision, or transaction review. [Read API and permissions](requirements/merchant-management.md)

Point `PAYGUARD_POLICY_TEST_DATABASE_URL` at a disposable PostgreSQL database named `payguard_policy_test_*` before running the management and temporary token API integration tests.

```sh
(cd backend && go test -run '^(TestMerchantManagementPostgres|TestMerchantTemporaryAccessTokenPostgres)$' -count=1)
```

## Local evaluation

For the standalone operations walkthrough, leave `PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN` unset. Start the stack with distinct demo merchant and operations bearer tokens. Configure `PAYGUARD_DEMO_ANALYST_A_TOKEN`, `PAYGUARD_DEMO_ANALYST_A2_TOKEN`, `PAYGUARD_DEMO_VIEWER_A_TOKEN`, `PAYGUARD_DEMO_ANALYST_B_TOKEN`, and `PAYGUARD_DEMO_VIEWER_B_TOKEN` as needed. Analyst and viewer credentials identify a fixed demo actor within their own merchant. The browser keeps credentials in memory for the current page. Use the [booking smoke script](#mock-checkout-and-booking-lineage) to create repeatable labeled synthetic cases for the shadow report.

1. Open `/settings/mock-checkout` with a demo merchant token. Submit a normal synthetic booking, then a second with **Simulate timeout** enabled. The latter creates a manual-review case.
2. Open `/settings/operations` and select **Load transactions** with the same merchant’s viewer token. Filter by product, outcome, or HOLD state. Open the timeout booking and inspect its masked contact, saved assessment, and HOLD deadline. Queue KPIs summarize the tenant’s stored transactions, and freshness is the age of the newest receipt. The latency metric uses bookings with an assessment timestamp.
3. Reconnect with an analyst token. Reveal the synthetic contact with a reason to see an audit reference. Claim the HOLD and record a review disposition. An overdue HOLD requires active escalation before it can be reassigned and resolved. Connect with the second analyst to confirm another actor cannot resolve a case assigned to the first analyst.
4. In an assessed transaction, select **Generate dossier** and wait for the cited evidence. Use **Export masked JSON** to save the bounded set of assessment fields and recorded versions. Use **Dashboard settings** to choose which KPIs appear and save the active filters for this demo actor.
5. Reconnect with the merchant token and open **Rule workspace**. Set a holdout interval containing labeled fixture bookings, select a data cutoff after the applicable outcomes were recorded, and run a shadow report. Inspect sample exclusions, confusion matrices, recall, false-positive rate, and pinned version tuples. A missing denominator displays as undefined.
6. In **Change approvals**, propose a velocity threshold from a report with at least one scored booking, or make a profile, rubric, or product-list proposal. Use a same-tenant analyst credential to approve or reject it. Check the audit actors, status, active revision, and expiry. A rollback copies an earlier approved candidate into a fresh proposal.
7. In Simulation Lab, enter the optional demo merchant token and run an OTA, e-commerce, or gateway fixture. Inspect linked entities and the platform, merchant-policy, and final action fields. Repeat a normal fixture with legitimate shared Wi-Fi to verify that shared IP alone does not trigger a block.

### Operations and governance API

These routes use `Authorization: Bearer <token>`. The operations prefix is `/api/v1/booking/ops/transactions`.

| Method and path | Behavior |
| --- | --- |
| `GET /api/v1/booking/ops/transactions` | Returns tenant queue rows, KPIs, and the authenticated actor. Filters are `product`, `outcome`, `hold`, and `limit` up to `100`. |
| `GET /api/v1/booking/ops/transactions/{id}` | Returns the booking, saved assessment, masked synthetic contact when available, and review state. |
| `POST /api/v1/booking/ops/transactions/{id}/reveal` | An analyst sends a `reason` of `manual_review` or `support`. Returns the synthetic address and an audit ID after recording access. |
| `POST /api/v1/booking/ops/transactions/{id}/hold/{action}` | An analyst sends `claim`, `escalate`, or `resolve`. Resolution takes `{"resolution":"cleared"}` or `{"resolution":"rejected"}`. State changes serialize on the merchant and booking case and record review history. |
| `POST` or `GET /api/v1/booking/ops/transactions/{id}/dossier` | Requests or reads the asynchronous dossier for an assessed booking. |
| `POST /api/v1/booking/shadow` | Sends `holdoutStart`, `holdoutEnd`, `asOf`, and a positive integer `reviewThreshold`. The holdout is start-inclusive and end-exclusive on booking receipt time. This read-only comparison uses synthetic verified outcomes available at the cutoff and excludes baseline overlap. |
| `GET` or `POST /api/v1/booking/changes` | Lists merchant changes or accepts a proposal from the merchant actor. Proposals include a lowercase UUID `id`, `kind`, `expiresAt`, and `candidate`. Use `rollbackOf` instead of `candidate` to copy an approved revision of the same kind. Rule proposals also include a matching `shadowWindow` with scorable evidence. |
| `POST /api/v1/booking/changes/{id}/decision` | A different analyst submits `{"decision":"approve"}` or `{"decision":"reject"}` before expiration. An approved rollback becomes a new revision. |
| `POST /api/v1/booking/network/runs` | Creates a saved synthetic Simulation Lab run for the authenticated merchant. Read its evidence at `GET /api/v1/booking/network/runs/{id}` and its QA aggregates at `GET /api/v1/booking/network/runs/{id}/report`. |

The local HOLD deadline is 15 minutes. Escalation and dispositions are demo workflow records. Rule replays and shadow reports take no payment action. Production expiry policy, real-label validation, and security and legal review require separate team decisions.

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

1. Select **Travel → Bangkok to Singapore**. In **Simulation settings**, select **Normal traffic**, set **Accounts** to `1`, and leave **Omit payment token** and **Mock Jev timeout** off.
2. Enter a synthetic contact such as **Test Buyer**, `buyer@restricted.test`, and `+1 202-555-0142`. Enable **Block flights for restricted.test**, then run the scenario. In the first order's **PRE PAYMENT** evidence, check that the contact is visible, **Event email domain** reads `restricted.test`, the merchant policy and final action are `BLOCK`, and the final action source is `MERCHANT_POLICY`. The platform decision is `ALLOW` for this fixture.
3. Change the email to `buyer@traveler.test` and run again. Check that the contact reflects the new submission, **Event email domain** reads `traveler.test`, the merchant policy is `PASS`, and the final action is `ALLOW`. A subdomain such as `sub.restricted.test` also passes because the rule matches the exact hostname.
4. Clear the contact and run again to use the original seeded domain. To verify product and stage boundaries, inspect **BOOKING**, disable the policy before rerunning, or repeat with a hotel or e-commerce product. Merchant policy is `NOT_APPLICABLE` in those cases.

## Omise sandbox checkout

The Omise test payment flow applies to the shared synthetic demo in **Travel** and **Online store**. Merchant-scoped scenarios and **Gateway** use simulated payments.

1. Configure the [test-mode keys](#local-keys-and-omise-sandbox-preparation) and start the app. Confirm `GET /api/v1/simulation/checkout/config` reports checkout available before opening the lab.
2. Choose a product in **Travel** or **Online store** and set the quantity. For a simple eligible run, open **Simulation settings**, select **Normal traffic**, and set **Accounts** to `1`. Leave merchant blocking, **Omit payment token**, and **Mock Jev timeout** off. Select **Run simulation** in the inline **Trip summary** or **Your cart**.
3. Choose **Pay in test mode** in the same panel. The first order must have final action `ALLOW` at `PRE_PAYMENT` for Travel or `PAYMENT` for Online store. If the checkout settings changed, run the simulation again before paying. Fill the Omise-hosted form with an official test card. [Omise.js hosted form](https://docs.omise.co/omise-js), [Official test cards](https://docs.omise.co/api-testing)
4. Review the payment status in the inline summary. If the status is pending or unknown, check the Test dashboard for the provider's outcome. The existing attempt remains associated with the saved run.

| Method and path | Behavior |
| --- | --- |
| `GET /api/v1/simulation/checkout/config` | Returns `enabled`, `currency`, and the per-product `prices` in satang, plus `publicKey` when sandbox checkout is configured. |
| `POST /api/v1/simulation/checkout/charges` | Accepts `{runId, token}`. Go verifies the saved run, calculates the THB total from its product and quantity, and records one charge attempt per run in PostgreSQL. A recorded attempt prevents another sandbox charge for the same run. |

Omise.js tokenizes the test card in the hosted form. PayGuard receives the token, and Go authenticates the sandbox charge with the test secret key. Full card numbers and security codes stay in the hosted form. [Omise authentication](https://docs.omise.co/api-authentication)

## System Rules

In the automatic loopback preview, open **Platform Rules** from the selected merchant. For the standalone platform rule dashboard, leave `PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN` unset and open [System Rules](http://127.0.0.1:3000/settings/system-rules) in the Platform Admin view. Save a synthetic run in Simulation Lab, then create a draft with a payment-stage condition such as `device_orders >= 2` and action `REVIEW`. Select the saved run to preview the draft. Use the returned event IDs to compare matches against the run's evidence. Check that the active built-ins and recorded decisions remain the same after saving and previewing. See the [scope and acceptance criteria](requirements/system-rules.md).

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

## Jev Rules

Use **Merchant Details → Merchant Rules → Booking review rules** for the administrator's merchant scenario flow. Open `/settings/merchant-rules?tab=review` or use the following API calls for the legacy [booking assessment](#booking-assessment) flow. Use the same demo booking credential, load the active rules, draft an example prompt or paste [rule JSON](requirements/jev-rules.md#rule-contract), and preview it against saved assessments before publishing.

A chat client with authorized access to the local API can send the same JSON directly. Save the draft as `rule.json` and set `PAYGUARD_DEMO_TOKEN` to the configured merchant token in your shell. Review the JSON and run a preview before calling the publish endpoint.

```sh
curl -fsS http://127.0.0.1:8081/api/v1/booking/jev/rules/preview \
  -H "Authorization: Bearer ${PAYGUARD_DEMO_TOKEN}" \
  -H "Content-Type: application/json" \
  --data-binary @rule.json

curl -fsS http://127.0.0.1:8081/api/v1/booking/jev/rules \
  -H "Authorization: Bearer ${PAYGUARD_DEMO_TOKEN}" \
  -H "Content-Type: application/json" \
  --data-binary @rule.json
```

For a rule that uses `amountMinor`, first approve a Scoring Profile that includes it. Assess a fresh booking at the threshold and inspect its `ruleHits`, outcome, and the Jev ruleset version in `jevRuleVersion` when present or `ruleVersion` otherwise. To isolate an amount rule during this check, use a fresh fixture window with fewer than five bookings. Replay an assessment made before the push to confirm it still returns the original result.

## Jev AI Configuration

Open **Decision Engine → AI Configuration** as Platform Admin. Select Mock or OpenRouter, the primary and optional fallback Jev models, an advisory rubric, and the total timeout. Save before testing changed settings. A saved version takes precedence over the deployment defaults for new merchant scenario transactions. The provider test is independent of Merchant Rules and Scoring Profiles. [Architecture and supported settings](requirements/decision-engine.md#jev-configuration)

For OpenRouter testing in Compose, set `OPENROUTER_API_KEY` in the ignored root `.env` and recreate the backend. To run Go on the host, export the key into its process environment. The diagnostic sends a server-defined synthetic fixture. The result reports `MOCKED`, `LIVE`, `TIMEOUT`, or `UNAVAILABLE`. A successful `LIVE` diagnostic verifies the response for that fixture. Check a new merchant scenario transaction separately to verify its recorded outcome and pinned configuration.

| Method and path | Behavior |
| --- | --- |
| `GET /api/v1/booking/ai-configuration/jev` | Get current settings, version, provider configuration state, and optional updated timestamp. |
| `PUT /api/v1/booking/ai-configuration/jev` | Send `config` and `expectedVersion`. Start with version `0` when no changed settings have been saved. A stale version returns 409. |
| `POST /api/v1/booking/ai-configuration/jev/test` | Send `{}` to test the saved settings with the fixed synthetic fixture. |

```sh
api=http://127.0.0.1:8081
curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer ${admin_token}" \
  "$api/api/v1/booking/ai-configuration/jev"

curl --fail-with-body --silent --show-error --request PUT \
  -H "Authorization: Bearer ${admin_token}" \
  -H 'Content-Type: application/json' \
  --data '{"expectedVersion":0,"config":{"provider":"mock","model":"typesafe/jev-1.14","fallbackModel":"","rubric":"conservative-v1","timeoutMs":2500}}' \
  "$api/api/v1/booking/ai-configuration/jev"

curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer ${admin_token}" \
  -H 'Content-Type: application/json' --data '{}' \
  "$api/api/v1/booking/ai-configuration/jev/test"
```

Use the version returned by GET as `expectedVersion` for the next save.

## AI Rule Composer

Set these values in the repository-root `.env` for Docker Compose. Use distinct administrator and merchant credentials. Set a bare OpenAI model ID that supports structured JSON output and is accessible to the API project.

```dotenv
PAYGUARD_DEMO_ADMIN_TOKEN=
PAYGUARD_PLATFORM_RULE_CHECKER_TOKEN=
PAYGUARD_RULE_COMPOSER_ENABLED=true
PAYGUARD_RULE_COMPOSER_PROVIDER=openai
PAYGUARD_RULE_COMPOSER_MODEL=gpt-4.1-mini
```

Set `OPENAI_API_KEY` in the ignored `backend/.env`. The optional OpenRouter key belongs to the [Jev assessment and diagnostic](#jev-ai-configuration). Use a checker token distinct from the administrator token to enable independent Platform Rule approval and rollback. For a host-run backend, export the same variables into the Go process before starting it. For Compose, restart the backend with `docker compose up -d --wait backend` after an environment change. Build the application after changing code. With general-purpose AI composition disabled or required configuration absent, AI interpretation requests return 503. Existing rules and the temporary exact-email command run locally. [Rule fields, actions, and lifecycle](requirements/ai-rule-management.md)

1. Create a synthetic merchant in **Merchants** as Platform Admin. In the merchant registration or edit form, record its catalogue products and declare email domain support only if the integration supplies it. Copy the one-time merchant credential for the test. Existing merchant setup and observed evidence can supply available signals.
2. Sign in as the merchant and open `/settings/merchant-rules`. Describe a rule in Thai or English, then select **Generate preview**. For a merchant that supplies flight tickets and email domains, try “Block flight bookings when the customer email domain is exactly skycannner.com.” Check the product, stage, signal, and action before selecting **Save Rule**.
3. Check the saved status and preview counts. Use **Test rule** to replay against recent stored evidence. An active policy applies to future merchant scenario transactions. Submit a matching synthetic transaction in Mock Checkout or the merchant scenario panel of Simulation Lab, then inspect the Platform, Merchant Policy, and Final decisions. An older transaction with the same input and ID returns its saved decision.
4. As Platform Admin, open `/settings/system-rules`. Compose a global rule, select a shared Simulation Lab run for optional replay, and save it as a draft. To request activation in the local test stack, open **Platform rule approval** and submit the current draft with a shared run that has eligible events and a reason.
5. Use the independent checker credential to review the pending revision and its recorded replay. Approve with a reason to apply the revision on new Simulation Lab runs. The checker can also restore an earlier active revision or disable the rule. Compare the ruleset version and rule hits on a new run with the original stored run.

### Temporary exact-email block

1. Open **Merchant AI Rules** with the merchant's credential. The merchant needs at least one supported product in its profile.
2. Enter `Block email buyer@example.test 12 h.` or `บล็อกอีเมล buyer@example.test เป็นเวลา 12 ชั่วโมง`, then select **Generate preview**. Use one synthetic address and a duration from 1 to 8,760 whole hours.
3. Verify the address, duration, and products in the preview, then select **Save Rule**. The rule covers all supported products shown in the preview. After activation, **Saved policies** shows when the block expires.
4. Run the merchant scenario in **Mock Checkout** and **Simulation Lab** with `buyer@example.test`. Check that the final decision is **Block** and the merchant policy is the source. Run again with `other@example.test` to confirm this rule only matches the first mailbox.
5. Once the rule expires, submit a new transaction with `buyer@example.test` and verify the expired rule does not match. Replaying the original transaction with its original ID and input returns its saved decision.

The full address stays in the browser. PayGuard receives a digest, derives a reference with the merchant's key, and uses that reference for matching inside PayGuard. The command does not require an OpenAI key.

### Composer and policy APIs

Send the bearer credential for the scope you request. The composer uses an issued merchant token for `MERCHANT` or the configured demo administrator token for `PLATFORM`. Use shell variables for the credentials in these examples and set `api` to your local backend base URL.

```sh
api=http://127.0.0.1:8081
curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer ${merchant_token}" \
  -H 'Content-Type: application/json' \
  --data '{"scope":"MERCHANT","instruction":"Block flight bookings when the email domain equals skycannner.com."}' \
  "$api/api/v1/booking/rule-composer/interpret"

curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer ${admin_token}" \
  -H 'Content-Type: application/json' \
  --data '{"scope":"PLATFORM","instruction":"At payment, review when there were more than five orders on the same device in 60 seconds."}' \
  "$api/api/v1/booking/rule-composer/interpret"
```

An interpretation returns `READY` with a validated proposal or `CLARIFICATION` with a message or validation errors. Pass the returned proposal as `{"proposal":{...}}` when saving. For a Platform draft with replay, add `"runId"` to the save body. The [browser flow](#ai-rule-composer) builds these requests from the preview.

| Method and path | Purpose |
| --- | --- |
| `POST /api/v1/booking/rule-composer/interpret` | Interpret an instruction using the authorized scope. |
| `POST /api/v1/booking/rule-composer/platform/save` | Save an administrator's validated global draft, with optional replay evidence in the response. |
| `POST /api/v1/booking/rule-composer/platform/{id}/submit` | Administrator submits `draftVersion`, a shared `runId` with eligible events, and `reason`. |
| `POST /api/v1/booking/rule-composer/platform/{id}/approve` | Independent checker approves the pending `version` with a `reason`. |
| `POST /api/v1/booking/rule-composer/platform/{id}/rollback` | Checker sends `expectedActiveVersion`, earlier active `version`, and `reason`. Send `version: 0` to disable the rule. |
| `GET /api/v1/booking/rule-composer/platform/{id}/versions` | Administrator or checker reads the recorded revisions and current active version. |
| `GET /api/v1/booking/demo/merchant-policy/profile` | Read the authorized merchant's products, stages, and available signals. |
| `GET /api/v1/booking/demo/merchant-policy/rules` | List the authorized merchant's latest rules and effective versions. |
| `POST /api/v1/booking/demo/merchant-policy/rules` | Save a validated proposal for the authorized merchant. |
| `GET /api/v1/booking/demo/merchant-policy/rules/{id}` | Read the merchant's most recent rule version and history. |
| `PUT /api/v1/booking/demo/merchant-policy/rules/{id}` | Save a new version with `proposal` and `expectedVersion`. |
| `POST /api/v1/booking/demo/merchant-policy/rules/{id}/simulate` | Preview the latest policy over up to 200 of the merchant's recent stored transactions. |
| `POST /api/v1/booking/demo/merchant-policy/rules/{id}/activate` | Activate a pending policy version after validation and conflict checks. Send `expectedVersion`. |
| `POST /api/v1/booking/demo/merchant-policy/rules/{id}/disable` | Disable the rule by appending a version. Send `expectedVersion`. |
| `POST /api/v1/booking/demo/merchant-policy/rules/{id}/rollback` | Restore an earlier proposal as a new version. Send its `version` and the latest `expectedVersion`. |

Merchant transaction submission accepts an optional normalized `customerEmailDomain` and a `customerMailboxDigest` from the browser's exact-email flow. The API persists the three decision parts in `policyDecision`. Old transaction records can omit that object. Save and evaluation use the registered scope and current evidence. If activation returns 409, resolve the conflicting policy or refresh the latest version before retrying. If the general-purpose composition API returns 422, check the instruction length and remove personal data. For 502, check provider reachability and model support for structured output. For 503, check the backend's enable flag, API key, and text model configuration.

### Migration and recovery

The Go backend applies additive PostgreSQL migrations when it initializes. The rule migrations add `policy_integration` and `mailbox_ref_key` to the merchant, `customer_email_domain`, `customer_mailbox_ref`, and `policy_decision` to the transaction, the tenant-scoped `merchant_policy_rule_versions` table, and the `platform_rule_revisions` table. The old tables and existing decision snapshots remain readable. Take a protected PostgreSQL backup before applying the build to an existing environment. Test against a disposable database or a restored copy first.

To pause composition, set `PAYGUARD_RULE_COMPOSER_ENABLED=false` and restart the backend. To change an effective rule, use the Merchant Policy or Platform checker workflow to append a disable or rollback revision. Keep the additive schema and saved decisions in place during a backend rollback. A previous backend build resumes its prior evaluation behavior while the new policy data stays stored. Use a pre-migration backup only for full database recovery, and account for transactions created after that backup.

### Verification commands

```sh
(cd backend && go test ./... && go test -race ./... && go vet ./...)
(cd frontend && npm run typecheck && npm run build)
docker compose config --quiet
git diff --check
```

Run integration tests on an isolated PostgreSQL and Redis stack. Set `PAYGUARD_POLICY_TEST_DATABASE_URL` to a disposable database named `payguard_policy_test_*` to include the persistence and timing tests. Mock provider responses when verifying parsing, clarification, timeout, and malformed output handling. A live provider smoke test requires a direct OpenAI key, an accessible structured-output model, and synthetic prompts. Record its result separately. [Current verification](current-work.md#ai-rule-management)

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
