# Current work

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
