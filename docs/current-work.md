# Current work

## Simulation Lab storefront and Omise Sandbox

Implemented and verified locally on `codex/omise-simulation-checkout`. The lab offers **Travel** and **Online store**, an inline THB summary, and the existing synthetic controls and contact. Eligible first orders can use the Omise-hosted test card form. [Storefront UI](ui/simulation-lab.md), [sandbox runbook](simulation-runbook.md#omise-sandbox-checkout)

- Go tests, race checks, and vet passed. Frontend type checking and the Next.js production build passed on Node.js 24.
- The Omise Thailand test-account check confirmed THB support. Sandbox API tests completed a flight for two travelers at 918,000 satang and two physical items at 498,000 satang. Duplicate and concurrent requests for the same run returned its original charge ID. Runs with final decisions of `BLOCK` and `UNSUCCESSFUL` were denied at checkout.
- Chrome completed the official Omise-hosted form on localhost and showed successful test payments for the Travel flight at 459,000 satang and the Online store digital gift card at 75,000 satang on mobile.
- Independent Chrome checks covered all four product images and selection, booking and delivery controls, quantity and THB totals, contact and decision evidence, and Offline QA. A changed cart or blocked first checkout disabled payment. A fresh `ALLOW` enabled it. The page fit 390 px and 320 px viewports without horizontal scrolling. Independent Go race and vet checks passed, and Rule Base and enrichment paths matched their branch base.

## Provider enrichment

Implemented and verified locally on `codex/provider-enrichment`. The authenticated Go endpoint returns IP, BIN, and email-domain evidence. Local BIN and domain masters operate with external services disabled. Rule Base files match the upstream base at `a97fcbe`. [Requirements](requirements/provider-enrichment.md), [runbook](simulation-runbook.md#provider-enrichment)

- Go race tests, vet, Docker Compose configuration validation, and the backend image build passed. The container importer reused the PostgreSQL snapshot, and the container user could read the local domain overlay. Provider fallback, normalization, and cache TTL checks used mocked responses.
- The PostgreSQL importer loaded 458,044 BINs, including 454,496 six-digit and 3,548 eight-digit records. Of these, 220,620 had all four canonical fields, including 217,083 six-digit and 3,537 eight-digit records.
- The local HTTP test returned complete exact matches for all six sampled BINs (`424242`, `540131`, `511332`, `371449`, `45717360`, and `43638410`). `010051` returned partial evidence under `UNKNOWN`. `00210299` returned its shorter match as reference evidence only. `00000000` returned no master match.
- Email checks covered 231,582 bundled domains and the 244,118-domain union with the local overlay. `mailinator.com` and a child domain returned all four source identifiers. `gmail.com` returned `listed: false`.

## PG-01 booking intake

Implemented the versioned booking intake on the local branch. The [runbook](simulation-runbook.md#booking-event-intake) documents the demo bearer token configuration, event contract, and replay responses.

- Go tests and vet passed in the Docker Go toolchain. Live checks passed for validation, authentication, concurrent idempotency, conflicting retries, tenant separation, and missing versus zero amounts.
- An identical request returned the saved response after a backend restart. Verification against the latest upstream application changes is pending. The team Sheet remains open until that integration is verified.

## System Rules dashboard · 21 November 2026

Implemented and verified locally on `codex/system-rules`. The dashboard shows active platform built-ins in evaluation order, saves editable drafts in PostgreSQL, and previews candidate conditions against one selected synthetic run. Active rule execution and Merchant Policy retain their existing behavior. See the [requirements](requirements/system-rules.md), [UI](ui/system-rules.md), and [runbook](simulation-runbook.md#system-rules).

- Go race tests and vet passed. Frontend type checking and the Next.js production build passed on Node.js 24.
- Live API checks with PostgreSQL and Redis verified the fresh rule catalogue, built-in protection, typed and stage-aware validation, draft edits, stale-version rejection, and persistence after a backend restart.
- Repeated previews returned the same results and skipped events with missing signals. Both the saved decision record and a fresh same-seed run were unchanged after draft and preview operations.
- Chrome checks on desktop and mobile covered navigation, search and filters, built-in details, draft creation and editing, retry and conflict states, and the one-to-five-condition limit. Changing stages cleared unavailable signals. A saved-run preview evaluated 12 events and matched 5, with matching event IDs shown in the editor.

## Simulation Lab checkout contact · 21 November 2026

Implemented and verified locally on `codex/simulation-contact`. The first order displays the submitted synthetic contact in both stages. The full contact stays in browser memory, and Go receives only the normalized `.test` email domain. See the [requirements](requirements/simulation-lab.md), [UI](ui/simulation-lab.md), and [runbook](simulation-runbook.md#verify-checkout-contact-and-merchant-policy).

- Go race tests and vet passed. Frontend type checking and the Next.js production build passed on Node.js 24.
- Live API tests with PostgreSQL and Redis verified domain validation, case-insensitive replay, a 409 for a different domain under the same run ID, first-order scope, exact policy matching, disabled and non-flight policy behavior, and timeout precedence.
- Chromium verified editable contact and the submitted snapshot, including invalid email and a simulated 503. Request and response checks and a PostgreSQL query confirmed that the submitted name, full email, and phone were absent from API traffic and persisted run data.

## Simulation Lab · 21 November 2026

Local implementation and verification complete for the [requirements](requirements/simulation-lab.md) and [UI](ui/simulation-lab.md). Use the [runbook](simulation-runbook.md) to start the demo and repeat the checks.

- Go race tests and vet passed. Frontend clean install, type check and Next.js production build passed on Node.js 24.
- A live Go API smoke test with PostgreSQL 16 and Redis 7 passed for all four products, concurrent idempotent replay, conflicting run IDs, deterministic results across run IDs, event ordering, duplicate PSP retries, shared Wi-Fi, rotating ring IPs, isolated labels, exact-domain policy, missing token and mock timeout. A Redis outage returned 503 and left no completed run.
- Chrome smoke checks exercised the setup, decisions and offline QA for normal e-commerce and mixed OTA, including the mobile layout. The default mixed run produced 24 decisions, deduplicated 12 PSP retries and scored 12 synthetic payment samples.
- The [Google Drive System Handbook](https://docs.google.com/document/d/1RtzQI1VIRXabDKUBPGcfQM8NxJZs1u_geFWFYC8Hq_I/edit) records the original local scope in section 12. Production rollout, production merchant authentication, live Jev, and production payment integration are future work.
