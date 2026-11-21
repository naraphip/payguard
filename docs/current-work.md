# Current work

## Merchant scenarios and mocked Jev

The local preview implements a runnable slice of the [merchant scenarios requirements](requirements/merchant-scenarios.md). Under **Merchants**, a demo administrator can create and rename merchants and save multiple named scenarios. Each scenario previews and versions one booking amount threshold rule. Created merchants receive a scoped access token.

Mock Checkout and the **Merchant scenario simulation** panel in Simulation Lab select a merchant and scenario, run the mock Jev decision, and save a transaction with its source, reasons, and versions. The merchant workspace lists transactions and records a review disposition separately from the saved assessment. [Evaluation steps](simulation-runbook.md#merchant-scenarios-and-mock-jev)

The [local preview](http://127.0.0.1:3001/settings/merchants) uses a structured threshold form and direct demo review. AI-assisted draft generation, a fully configurable rule set, the full HOLD lifecycle in this merchant workspace, and live Jev integration remain open.

The opt-in loopback preview opens the Merchants workspace and scenario pickers as a demo administrator. Set the Next server’s `PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN` to the API’s demo administrator token to enable it. The old Risk Operations route redirects to **Merchants → Transactions** in this preview. Standalone routes and the ordinary authenticated flow remain available when the opt-in is disabled.

The selected merchant has **Jev Rules** and **Platform Rules** tabs. Demo administrators manage that merchant’s rules, and authorized merchant users can view their own. Published amount-based Jev rules apply to Mock Checkout and the single-booking merchant scenario simulation, with the supplemental ruleset version pinned in the result. Platform built-ins remain read-only, and Platform drafts are for testing. A Platform draft preview requires a compatible saved Simulation Lab run for the selected merchant.

- Live API checks verified the administrator directory and merchant-scoped access. In both Mock Checkout and Simulation Lab, an amount of 50,000 minor units returned **Low risk** at a threshold of 100,000 and **Review** at a threshold of 50,000.
- Live API checks verified missing amount versus zero, timeout and review resolution, replay pinned to an earlier revision, and cross-merchant denial.
- Go tests, race tests, and vet passed. The isolated frontend production build, TypeScript checks, and existing booking smoke checks also passed.
- Independent browser checks verified merchant creation, one-time token display, and saving two scenario drafts. The browser showed a **Low risk** checkout, a **Review** simulation with a matching rule, and an **Unresolved** simulation after a mock timeout. The merchant’s ledger contained exactly three transactions with the expected sources and outcomes. A direct link opened the saved transaction, and sign-in with the issued token showed only the merchant’s own workspace with merchant creation and editing unavailable.
- A fresh Chrome session verified the automatic loopback path. It created a merchant and scenario, followed a deep link to checkout, and reopened its saved transaction after refreshing the ledger. The browser trace and storage contained no authorization header, administrator token, or issued merchant token.
- Independent Chrome checks verified that the three old operations and rule URLs open their merchant workspace views without login. Two merchants started with the same scenario threshold and both returned **Low risk**. After publishing a Jev rule for merchant A, A returned **Review** and B remained **Low risk**. A Platform draft stayed scoped to its merchant and persisted on reload. Previously saved transactions retained their results.
- Live API checks verified removal of the issued token from automatic-preview responses, rejection of unauthenticated direct API access and disallowed origins and hosts, and isolated create and edit operations for Platform drafts. New checkout and scenario simulation results pinned the selected merchant’s supplemental Jev revision, and replay returned the original result.

## PG-11 through PG-19 local evaluation

Local evaluation uses synthetic bookings and outcomes. The [evaluation runbook](simulation-runbook.md#local-evaluation) walks through the operations queue, fixed-window shadow replay, change approvals, and linked-entity simulation.

| Task | Local implementation |
| --- | --- |
| PG-11 | Demo merchant, viewer, and analyst credentials resolve a tenant and actor. Transaction details show masked synthetic contact information. Analyst reveals record actor, reason, and time in PostgreSQL. |
| PG-12 | Manual-review assessments appear as HOLDs. Analysts can claim and resolve their own cases or explicitly escalate an overdue case. The local deadline is 15 minutes and dispositions are review records. |
| PG-13 | Risk Operations shows a tenant-scoped transaction list, booking and assessment evidence, and KPIs computed from stored records and receipt and assessment timestamps. |
| PG-14 | A fixed holdout window and data cutoff compare recorded decisions with a candidate velocity rule. Reports include recall, false-positive rate, exclusion counts, and pinned versions. |
| PG-15 | Tenant change proposals support profiles, rules, rubrics, and product review lists. A different analyst records a decision. Expiry and rollback revisions preserve the history. |
| PG-16 | Simulation Lab uses separate travel, e-commerce, and gateway product modules. Merchant policy evaluates the exact configured flight domain separately and reports the action source. |
| PG-17 | An asynchronous dossier builds a typed, masked evidence export from a saved assessment and its versions. |
| PG-18 | A demo merchant token scopes saved Simulation Lab network runs and linked entity evidence to the merchant. |
| PG-19 | Risk Operations stores dashboard metrics and default filters per demo actor in the browser. The rule workspace provides a fixed-window preview and navigation to the visual rule editor. |

- Live PostgreSQL and Redis checks verified role gates and cross-tenant denial, masked and audited reveal, concurrent claim exclusivity, idempotent resolution, explicit overdue escalation and reassignment, and product, outcome, and HOLD filters. The queue returned measured assessment latency and freshness. The operations smoke passed on both an independent stack and the combined preview.
- A fixed shadow holdout replayed unchanged. The recorded decisions produced one true negative and one false negative. The candidate produced one true positive and one false positive. Assessment snapshots remained identical, and the tenant boundary held.
- Governance checks verified distinct maker and checker actors, decision replay and conflict, tenant denial, rejection, and rollback as a new revision. Product and policy checks covered the gateway stages and exact domain matching. The asynchronous dossier persisted typed citations and omitted raw contact and outcome data.
- Identical tenant-scoped network fixtures produced distinct entity keys across tenants. Reading another tenant’s run failed, and a legitimate shared IP alone did not trigger a block.
- Go tests and vet passed. The frontend production build and type check passed. Independent Chrome checks verified viewer masking, real queue filters and KPIs, dossier export, browser preferences by actor, shadow metrics, and rejection by a separate analyst. The 390 px layout fit without horizontal overflow.

PG-20 requires approved real outcome labels and shadow results. PG-21 requires Security and Legal review.

## Jev Rules

Implemented and verified the local demo for drafting a booking review rule from a supported prompt or AI-generated JSON, previewing it against saved assessments, and publishing it to the merchant’s Jev ruleset. [Requirements](requirements/jev-rules.md) · [UI](ui/jev-rules.md) · [Runbook](simulation-runbook.md#jev-rules)

- Independent API checks passed for unauthorized requests, strict validation, merchant isolation, read-only previews, amount threshold boundaries, missing versus zero and unselected amount, and AND conditions. New assessments pinned rule versions and hits, timeout kept its review handoff, and replay returned the prior result. Concurrent distinct pushes retained both rules. Identical and reordered-AND retries deduplicated.
- Chrome with the isolated Go API verified navigation from Simulation Lab, loading the default rules, drafting and reviewing a prompt, preview, and publication of version `booking-rules-v2`. The token stayed out of local and session storage, and the page fit a 390 px viewport. Local parser checks and the mocked UI test also passed.
- The isolated HTTP suite passed against a fresh database, and the published revision survived a backend rebuild and restart. `go test ./...`, `go test -race ./...`, and `go vet ./...` passed. The final frontend passed `npm run typecheck` and `npm run build`. `git diff --check` passed.

Implementation lives in the [Jev rules API and evaluator](../backend/booking_jev_rules.go), [booking assessment](../backend/booking_assessments.go), [Go regression tests](../backend/booking_jev_rules_test.go), and [Jev Rules page](../frontend/app/settings/jev-rules/page.tsx).

## PG-07 through PG-10 mock checkout, baseline and booking lineage

Implemented and verified the local synthetic checkout, outcome import, baseline activation, and tenant-scoped lineage. The demo runs at `/settings/mock-checkout`. [Requirements](requirements/checkout-and-lineage.md) · [Checkout UI](ui/mock-checkout.md) · [Runbook](simulation-runbook.md#mock-checkout-and-booking-lineage)

- PG-07 Chrome checks covered navigation and all four products, OTA checkout with amount zero, and e-commerce checkout with omitted amount, unavailable SDK data, and a Jev timeout. The visible assessment matched the saved response and lineage. A simulated lost response retried the same booking and got 200 from both APIs. The token stayed in memory and the page fit a 390 px viewport.
- PG-08 live checks verified that exactly three distinct eligible bookings moved the baseline from `COLD_START` to `ACTIVE`. Chrome displayed the active sample count. Earlier bookings retained their pinned baseline and rule version.
- PG-09 `python3 scripts/booking-smoke.py --base http://127.0.0.1:8082 --merchant b --threshold 3` passed. Both fixture passes produced `LOW_RISK` at velocity 1, `REVIEW` at velocity 5, and `UNRESOLVED` at velocity 1. A rerun on the same database preserved those signatures and advanced the active baseline by three samples. Duplicate replay and tenant scope passed, and assessment payloads contained no QA labels. The previous PG-01 and PG-03 through PG-06 API checks passed on merchant A.
- PG-10 live checks covered separately recorded analyst and chargeback provenance, out-of-order event and receipt timestamps, observed, unavailable and unknown SDK states, tenant isolation, and replay of the original assessment after outcomes were imported.
- Go race tests and vet, frontend type checking and production build, and `git diff --check` passed. DEC-04 and DEC-05 remain open. The team action plan is unchanged.

## PG-03 through PG-06 booking assessment

Implemented and verified all four tasks for the [local preview](http://127.0.0.1:3001/settings/booking-assessment). The demo covers deterministic Scoring Profile proposals and approval, booking evidence with pinned versions, trusted booking velocity and cold-start evidence, and typed mock Jev results with a timeout handoff. [Requirements and contract](requirements/booking-assessment.md) · [UI](ui/booking-assessment.md) · [Runbook](simulation-runbook.md#booking-assessment)

- PG-03 live API checks verified bearer authentication, tenant separation, the same proposal for reversed signal order, and rejection of unavailable and future-stage signals.
- PG-04 checks verified that changing the active profile preserves the original assessment and version on replay. Concurrent same-mode requests replayed the stored result, and changing mode for an assessed booking returned a conflict.
- PG-05 checks with PostgreSQL and Redis produced the same count of five in two fixture runs and exercised the elevated velocity rule. The boundary checks excluded bookings exactly 60 seconds earlier, bookings with future event timestamps, and earlier bookings received after the target. The API returned distinct evidence for a missing amount and an explicit zero and reported `COLD_START`.
- PG-06 checks verified typed Jev lineage and the timeout outcome `UNRESOLVED` with a `MANUAL_REVIEW` handoff. Independent Chrome verification exercised the missing-setup state, navigation from saved setup, explicit approval, a zero-amount booking, timeout, and same-booking retry after a lost response. The token remained out of persistent browser storage and the page fit a 390 px viewport.
- Rebuilding and recreating the local backend preserved the pinned assessment. The replay returned identical JSON, and the full API smoke checks passed again.
- Go unit tests, race tests and vet passed, as did frontend type checking, the isolated production build, and `git diff --check`. The broader action plan status is unchanged.

Implementation lives in the [profile API](../backend/booking_profiles.go), [assessment API](../backend/booking_assessments.go), [booking risk tests](../backend/booking_risk_test.go), and [demo screen](../frontend/app/settings/booking-assessment/page.tsx).

## PG-02 merchant setup

Implemented and independently verified the [initial local preview](http://127.0.0.1:3001/settings/merchant-setup). The form saves the merchant business model, Booking stage, and available signals as a schema version 1 record in the current browser. [Requirements](requirements/merchant-setup.md) · [UI](ui/merchant-setup.md)

- `npm run typecheck`, `npm run build` in an isolated directory, and `git diff --check` passed.
- Chrome 157 verified navigation from Simulation Lab and System Rules, active navigation, missing-stage feedback, and save and reload for an OTA setup with booking amount. Editing the setup to e-commerce with only the required `occurredAt` signal persisted after reload.
- Chrome verified field-level errors for an invalid saved stage and signal, followed by a successful correction. A simulated `QuotaExceededError` retained the selections and save succeeded on retry. The page fit a 390 px viewport without horizontal overflow.
- Implementation adds the [route](../frontend/app/settings/merchant-setup/page.tsx), [form](../frontend/app/settings/merchant-setup/setup-form.tsx), and [shared navigation](../frontend/app/settings/navigation.tsx), and connects navigation in [Simulation Lab](../frontend/app/page.tsx) and [System Rules](../frontend/app/settings/system-rules/dashboard.tsx).
- The broader action plan status and integrated PG-01 verification are pending.

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
- The [Google Drive System Handbook](https://docs.google.com/document/d/1RtzQI1VIRXabDKUBPGcfQM8NxJZs1u_geFWFYC8Hq_I/edit) records the implemented local scope in section 12. Production rollout, merchant authentication, live Jev and payment integrations are future work.
