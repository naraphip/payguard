# Current work

## PayGuard corporate identity

The v4 logo update uses the outlined horizontal logo without the tagline in expanded navigation and the mobile drawer. The collapsed sidebar and light and dark browser icons use the v4 symbol. The public compact logo uses a 954 × 266 viewBox to show the full shield and preserve the source paths. Shared controls use the established navy and pale blue palette, and the interface self-hosts IBM Plex Sans Thai in weights 400 through 700. [Crisp Logo Source Kit v4](../PayGuard_Logo_v4_Crisp_Vector/README.md), [brand palette](../PayGuard_Source/brand-colors.csv)

- `npm run build` and `npm run typecheck` passed on Node.js 24.20.0. All five public SVGs parsed successfully. Path comparisons confirmed the production geometry matches the v4 source, and SHA-256 hashes matched for all 19 source files.
- Chrome verified the expanded and collapsed sidebar at 1440 px and drawer navigation at 320 and 390 px without horizontal overflow. The shield and body fit inside the compact viewBox. Both favicon variants decoded, and the browser confirmed the palette and Latin and Thai font loading.
- The [local dashboard](http://127.0.0.1:3000/dashboard) returned HTTP 200 and served all five v4 SVGs with contents matching the production files.

## GitHub progress monitoring in Notion

Created the [PayGuard GitHub Activity database](https://app.notion.com/p/41f4da4de98381dcac67fb442ef0c35b) under the monitoring plan. The workflow is active on GitHub `main` at `40341b5`. See the [monitoring requirements](requirements/notion-progress.md) for repository configuration and rollout checks.

- Independent fixture and workflow YAML checks passed. A live local sync changed 18 activity records in Notion. The GitHub Actions repository secret and variable are configured.
- The [manual GitHub Actions run](https://github.com/pwatakul/payguard/actions/runs/44868685163) succeeded. The Notion health row showed Last Success at 11:59 Bangkok time on 21 November 2026. Verification of two scheduled runs is ongoing.

## Decision Engine consolidation

The [Decision Engine](requirements/decision-engine.md) groups global Platform Rules, Merchant Rules, and administrator AI Configuration. The selected merchant's details retain booking review rule management and move scoped profile information to Advanced Risk Configuration. The combined backend uses upstream Jev for new merchant scenario transactions. Deployment environment settings provide the default until an administrator saves a configuration version. A saved version controls new merchant scenario evaluations. The diagnostic still uses its fixed synthetic fixture. Legacy Booking API and batch simulation keep their own mock assessment paths, and saved transactions retain their original results.

### Integrated verification

- The standard and race Go suites each passed 265 tests and subtests with zero failures or skips against the same isolated PostgreSQL database. Go vet, Compose configuration validation, whitespace checks, and both the TypeScript check and production build on Node.js 24.22.3 passed.
- OpenRouter Jev returned `LIVE` for the integrated synthetic diagnostic in 0.99 seconds and for a merchant scenario in 0.49 seconds. The new scenario transaction pinned its Jev configuration version. After the administrator switched the saved configuration to Mock, an exact replay returned the previous decision and metadata.
- Chrome checked the final production frontend's live evidence display with simulated API responses matching the upstream result.

Before the upstream live Jev integration was combined with the Decision Engine changes, the following checks passed.

- The isolated Go diagnostic returned `MOCKED` for Mock. With OpenRouter configured, `POST /api/v1/booking/ai-configuration/jev/test` returned `LIVE` on `typesafe/jev-1.14` in 1.46 seconds with no fallback. The server sent its fixed synthetic fixture.
- Chrome checked the production frontend with simulated responses and then with the real isolated Go and PostgreSQL stack at 320, 390, 1024, and 1440 px. The real-stack check verified administrator save and a `MOCKED` test, both legacy redirects, distinct Merchant Rules sections, and the Advanced Risk Configuration route. The previous Jev rule API snapshot stayed intact. The Merchant UI denied direct access and the configuration GET, PUT, and test requests each returned 403 for the Merchant credential.
- `go test -count=2 -run TestJevConfigurationPreservesReviewRulesPostgres .` passed twice against the same disposable PostgreSQL database. The check toggled configuration versions and verified pinned legacy transaction replay. `go test -race -count=1 -run TestJev .` passed.
- The independent full standard and race Go suites each passed 252 tests and subtests with zero failures or skips. They ran sequentially against the same disposable PostgreSQL database. Three packages ran tests and two had none. Go vet, Compose configuration validation, `git diff --check`, TypeScript checking, and the Node.js production build passed.

## Merchant Management

The [Merchant Management slice](requirements/merchant-management.md) adds searchable and paginated directory reads, a persistent detail workspace, stored transaction metrics and filters, read-only policy inspection for Platform Admin, and a merchant-scoped audit view. It reuses the existing Sandbox merchant IDs, credentials, scenario selection, and transaction ledger. The production event contract from the team Sheets remains design input. The current Booking API retains its v1 validation. [Local workflow](simulation-runbook.md#merchant-management)

- Before Decision Engine consolidation, the Merchant Management standard and race suites on Go 1.26 each passed 248 tests and subtests with no failures or skips. Go vet, Compose configuration validation, whitespace checks, and the Node.js 24 TypeScript check and production build passed.
- The dedicated PostgreSQL management test covered legacy response compatibility, pagination, search and sort, real aggregates and ledger filters, scoped rule history, audit and revocation, and one-time issuance. Retrying merchant creation with the same idempotency key returned 409 without reissuing the credential.
- Independent Chrome verification against Go and PostgreSQL covered merchant creation, one-time credential issuance, direct detail links, all seven tabs, ledger and audit views, legacy redirects, and cross-tenant denial. The directory and details fit at 320, 390, 1024, and 1440 px.

The team Sheet records **Done (Sandbox)** for `ADM-MER-001` through `ADM-MER-006` and `ADM-MER-008`. The matching status values were read back after the scoped update. PG-16 retains its separate completion record.

## AI Rule Management

The [AI Rule Management slice](requirements/ai-rule-management.md) uses direct OpenAI Chat Completions to compose merchant policies and Platform Rules drafts. It reuses the existing merchant registration and evidence, applies active policies to new demo merchant transactions in Go, and records the separate decision sources and matched policy versions. The mock or live Jev merchant assessment runs separately. [Local configuration and API](simulation-runbook.md#ai-rule-composer)

- The dedicated PostgreSQL policy suite passed tenant isolation, exact-domain normalization, spoof and subdomain rejection, missing and stage-specific evidence, conflicts, disable and rollback, replay, and decision precedence. The latest run across 200 local samples with one rule measured a p95 of 0.401 ms for the policy configuration read and 1.084 µs for evaluation.
- The isolated booking smoke passed two fixture runs covering baseline activation, replay, tenant isolation, and lineage.
- Independent verification of the OpenAI adapter baseline passed 241 Go tests in both standard and race runs, plus Go vet, TypeScript checking, the frontend build, and Compose validation. The local Platform draft, submission, independent approval, and rollback flow passed.
- An isolated live direct OpenAI smoke test with `gpt-4.1-mini` returned Go-validated exact-domain flight proposals for Thai and English prompts using `example.test` in 5.10 s and 1.87 s respectively. The smoke test stopped at interpretation.
- The team Sheet status for PG-16 in `01 ACTION PLAN LATEST!H18` was set to **Done** and read back. This status covers the typed product modules and separation of merchant policy resolution tracked in that row. [Action plan](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260117)

## Merchant scenarios and Jev

The local preview implements a runnable slice of the [merchant scenarios requirements](requirements/merchant-scenarios.md). Under **Merchants**, a demo administrator can create and rename merchants and save multiple named scenarios. Each scenario previews and versions one booking amount threshold rule. Created merchants receive a scoped access token.

Mock Checkout and the **Merchant scenario simulation** panel in Simulation Lab select a merchant and scenario and save a decision with its source, reasons, and versions. Platform administrators can review transactions across active merchants and record a disposition alongside the saved assessment. A merchant credential opens a read-only list of its own transactions and results. [Evaluation steps](simulation-runbook.md#merchant-scenarios-and-jev)

The [original merchant scenario preview](http://127.0.0.1:3001/settings/merchants) uses a structured threshold form and direct demo review. [AI Rule Management](#ai-rule-management) adds the composer and separate Merchant Policy engine. The full HOLD lifecycle in this merchant workspace remains open.

The upstream live Jev slice uses the OpenRouter Decisions API from Go in the two merchant-selected tools. Absent a saved AI Configuration version, the sandbox can enable it with `PAYGUARD_AI_MODE=live` and a server-side `OPENROUTER_API_KEY`. The pinned default model is `typesafe/jev-1.14`. With an observed amount, the stored result includes the typed Jev choice, provider decision ID, and served model when available. Local review rules can escalate the assessment, and a provider failure sends the case to manual review. The integration sends only the booking amount, scenario threshold, and whether a configured review rule matched. Credentials stay server-side. The deployment default is Mock. [Team Blueprint](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=1604066438) · [OpenRouter Decisions API for Jev](https://openrouter.ai/typesafe/jev-1.14)

Before consolidation, independent Chrome verification on port 3001 recorded live `REVIEW` at 50,000 against a checkout threshold of 50,000 and live `LOW_RISK` at 50,000 against a single-scenario Simulation Lab threshold of 100,000. Both saved transactions showed distinct provider decision IDs and the served `typesafe/jev-1.14-20261014` model. Exact checkout replay preserved the saved metadata. The simulated timeout saved **Unresolved** under **Needs decision** without a provider decision ID.

The upstream backend passed `go test ./...` and `go vet ./...`. Tests covered the outbound evidence fields, typed response validation, provider failures, timeout, and replay flags. The Next.js production build and both local Compose configuration checks passed. Docker reported both app services healthy.

The next merchant assessment plan exposes `LOW_RISK`, `HIGH_RISK`, and `REVIEW`. **Review** covers both flagged cases and **Unresolved** assessments. Jev evidence, manual disposition, transaction stage, and payment action remain separate. The high-risk trigger and precedence are pending policy decisions. This is a planned contract update. [Planned PayGuard risk results](requirements/merchant-scenarios.md#planned-payguard-risk-results)

The opt-in loopback preview opens the Merchants workspace and scenario pickers as a demo administrator. Set the Next server’s `PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN` to the API’s demo administrator token to enable it. The server uses that token for the loopback proxy. After creating a merchant, the administrator can copy that merchant’s scoped access token from the one-time confirmation. The old Risk Operations route redirects to **Merchants → Transactions** in this preview. Standalone routes and the ordinary authenticated flow remain available when the opt-in is disabled.

The administrator manages a selected merchant’s scenarios, **Jev Rules**, and **Platform Rules**. Published amount-based Jev rules apply to Mock Checkout and the single-booking merchant scenario simulation, with the supplemental ruleset version pinned in the result. Platform built-ins remain read-only, and Platform drafts are for testing. A Platform draft preview requires a compatible saved Simulation Lab run for the selected merchant.

Merchant creation and editing now save the original setup fields on the merchant record. The demo supports OTA or e-commerce, the Booking stage, the required booking timestamp, and optional booking amount availability. Existing name-only records appear as **Setup incomplete**, and the legacy name-only API remains compatible. The former standalone setup page opens **Merchants** in the automatic preview, and its separate navigation entry is removed. A confirmed **Delete merchant** removes the merchant from the directory and test pickers and deactivates its access. Stored history is retained and excluded from normal access.

- Live API checks verified the administrator directory and merchant-scoped access. In both Mock Checkout and Simulation Lab, an amount of 50,000 minor units returned **Low risk** at a threshold of 100,000 and **Review** at a threshold of 50,000.
- Live API checks verified missing amount versus zero, timeout and review resolution, replay pinned to an earlier revision, and cross-merchant denial.
- Go tests, race tests, and vet passed. The isolated frontend production build, TypeScript checks, and existing booking smoke checks also passed.
- Independent browser checks verified merchant creation, one-time token display, and saving two scenario drafts. The browser showed a **Low risk** checkout, a **Review** simulation with a matching rule, and an **Unresolved** simulation after a mock timeout. The merchant’s ledger contained exactly three transactions with the expected sources and outcomes. A direct link opened the saved transaction, and sign-in with the issued token showed only the merchant’s own workspace with merchant creation and editing unavailable.
- Before the merge, Chrome verified automatic loopback access, creating a merchant and scenario, following a deep link to checkout, and reopening the saved transaction after refreshing the ledger.
- Independent Chrome checks verified that the three old operations and rule URLs open their merchant workspace views without login. Two merchants started with the same scenario threshold and both returned **Low risk**. After publishing a Jev rule for merchant A, A returned **Review** and B remained **Low risk**. A Platform draft stayed scoped to its merchant and persisted on reload. Previously saved transactions retained their results.
- Earlier API checks verified rejection of unauthenticated direct access and disallowed preview origins and hosts, and isolated create and edit operations for Platform drafts. New checkout and scenario simulation results pinned the selected merchant’s supplemental Jev revision, and replay returned the original result.
- On the merged preview, Chrome verified delivery of the one-time scoped merchant credential. Two merchant credentials opened independently scoped, read-only ledgers. The Merchant workspace rejected an invalid credential and an administrator credential. Opening an administrator URL returned the signed-in merchant to **Transactions**. Merchant views had no mutation controls.
- The merged API returned HTTP 401 without credentials, HTTP 404 for a transaction belonging to another merchant, and HTTP 403 when a merchant requested configured rules or attempted a mutation. Each merchant could list and open its own saved transactions.
- Independent Chrome checks verified create, edit, and reload for two merchants. The legacy setup URL redirected to Merchants, and the sidebar had no standalone setup link. Canceling deletion kept the test merchant. Confirming deletion removed only that merchant, and both Mock Checkout and Simulation Lab omitted it from their pickers.

## P0 Dashboard layout refactor

The [Next.js root layout](../frontend/app/layout.tsx) wraps every page in one [AppShell](../frontend/app/app-shell.tsx). `/` redirects to `/dashboard`, and [Simulation Lab](../frontend/app/simulation-lab/page.tsx) opens at `/simulation-lab`. The desktop sidebar collapses from 240 px to 64 px, and a drawer serves narrow screens. The topbar holds the breadcrumb, workspace, notifications, user controls, and Sandbox status.

[Fraud Overview](../frontend/app/dashboard/page.tsx) summarizes saved merchant records and shows recent transactions. An empty ledger displays an empty state. The administrator’s **Transactions** link opens the merchant transaction workspace in the automatic preview. The merchant sidebar has one destination, **Transactions**, and the merchant credential scopes the list and detail on the API. **Fraud Network** opens the evidence section in Simulation Lab. Mock Checkout and Simulation Lab continue to generate synthetic inputs on demand for testing.

The menu review compared the combined workspace with GitHub `origin/main` at `74e7a88` and the current Drive action plan. The plan puts merchant setup, scoring profiles, Jev, and Mock Checkout in the demo, with the HOLD queue, dashboard KPIs, advanced rule builder and fraud-network work in later phases. The current request gives Platform Admin the merchant configuration and cross-merchant transaction workflows and limits merchant access to its own transaction list and details. The linked System Handbook returned a Drive permission error. The current Blueprint and Rule Builder Manual were accessible. [Team Drive](https://drive.google.com/drive/folders/1lKpQdDOkrqpF61vxN_wNN2HHo9NQjNnH) · [Action plan](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260117) · [Simulation Lab](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260115) · [Merchant policies](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260109) · [Rule Builder Manual](https://docs.google.com/document/d/1vYpo8hkpWS3jQ21ZiRBNThf8uUh2ZdVf_AtoNL7hQho/edit)

Chrome followed all eight Platform Admin sidebar links in the merged automatic preview. **Dashboard**, **Merchants**, **Scoring Profiles**, **Mock Checkout**, and **Simulation Lab** opened their respective pages. **Transactions** opened the ledger, **Fraud Network** jumped to **Event decisions** in Simulation Lab, and both **Settings** links worked. The check also verified `/` opens `/dashboard` and a legacy root link with merchant and scenario parameters opens `/simulation-lab` with the exact saved scenario selected.

Before the local reset, Dashboard matched API reads for 22 active merchants, 37 saved transactions, and 20 transactions needing a decision. A recent transaction link opened its exact saved detail. Both the Jev and Platform rule catalogues loaded from the selected merchant’s workspace.

The final backend Docker build stage passed `go test ./...` and `go vet ./...`. The Next.js webpack production build, TypeScript checks, Compose configuration validation, and whitespace checks passed.

The PostgreSQL reset on the active local assessment stack for port 3001 left zero merchants, scenarios, transactions, bookings, drafts, simulation runs, and QA labels. Omise charges remained at zero. The reset was scoped to that local stack.

Before the merchant-workspace merge, the dashboard branch recorded these checks.

- `go test ./...`, `npm run typecheck`, `npm run build` on Node.js 24.22.3, and `git diff --check` passed. HTTP checks verified the root redirect, the frontend routes, and read API parity for the Platform Rules catalogue, Omise configuration, Jev Rules, and Risk Operations.
- Chrome checked all 13 destinations at 320, 390, 1024, and 1440 px with one shell and no horizontal page scrolling. Desktop checks verified 240 and 64 px sidebar widths, breadcrumbs, and active links. Mobile checks covered drawer navigation, keyboard dismissal, focus return, and backdrop dismissal. Platform Admin could open and cancel a draft. In Merchant view, a direct Platform Rules reload displayed the restricted view and sent no Platform Rules API requests.

## Integrated main candidate

The earlier local integration candidate combined merchant and booking flows, Jev Rules, Risk Operations and governance with the Simulation Lab storefront, provider enrichment, Omise Sandbox checkout, and earlier project documentation.

- Go race tests and vet, Docker Compose configuration validation, and the frontend type check and Next.js production build on Node.js 24 passed.
- The booking smoke script passed two fixtures and verified `COLD_START` to `ACTIVE` baseline activation, replay, tenant isolation, and lineage.
- The integrated Go API with PostgreSQL and Redis returned both Jev Rules and Platform Rules. Live local checks verified the same run ID across separate tenants, tenant-scoped QA, Gateway, and rejection of blocked and cross-workspace Omise requests before a provider call. The enrichment check imported all 458,044 BINs and matched a disposable-email domain. These integration fixtures used local data and made no external charges.
- Independent Chrome checks verified all seven navigation routes and active-link states, Jev Rules, Mock Checkout, both storefronts, and the Omise control on an eligible shared-demo run. Gateway stayed simulated. Scoped run and QA requests carried the merchant credential, isolation held, and clearing the token removed previous evidence. The 390 px viewport had no horizontal overflow. Chrome sent no external requests in this check.

Omise uses the original synthetic demo prices for the shared Travel and Online store checkout. Merchant-scoped and Gateway flows use their simulated checkout. The [implementation progress table](payguard-tasks.md#verified-implementation-progress) and the historical verification below record their respective scopes.

## Earlier merges through `e58e59a`

- Simulation Lab delivered the synthetic storefronts, seeded runs, evidence and entity graph, and offline QA. [GitHub PR #1](https://github.com/pwatakul/payguard/pull/1)
- Checkout Contact, the platform System Rules dashboard, and the supplied PayGuard logo merged. Chrome verified the PNG in both headers at desktop and mobile widths. [GitHub PR #2](https://github.com/pwatakul/payguard/pull/2)

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
- The implementation added the [route](../frontend/app/settings/merchant-setup/page.tsx), [form](../frontend/app/settings/merchant-setup/setup-form.tsx), and shared navigation for Simulation Lab and System Rules. [PG-02 through PG-06 implementation](https://github.com/pwatakul/payguard/commit/c08fe09fb3b4b9cacb19ab9f32411078586151ad)
- The broader action plan status and integrated PG-01 verification are pending.

## Simulation Lab storefront and Omise Sandbox

Implemented and verified locally on `codex/omise-simulation-checkout`. The lab offers **Travel** and **Online store**, an inline THB summary, and the existing synthetic controls and contact. Eligible first orders in the shared synthetic demo can use the Omise-hosted test card form. Merchant-scoped scenarios and the Gateway checkout use simulated payments. [Storefront UI](ui/simulation-lab.md), [sandbox runbook](simulation-runbook.md#omise-sandbox-checkout)

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

The pasted merchant data synthesis is recorded in the [Merchant event intake contract](requirements/merchant-event-contract.md). The note distinguishes Booking Event v1 from the proposed production envelope and stage and product extensions. Resolving the hotel guest-count and digital delivery-method requirements and publishing a single OpenAPI contract are pending.

- Go tests and vet passed in the Docker Go toolchain. Live checks passed for validation, authentication, concurrent idempotency, conflicting retries, tenant separation, and missing versus zero amounts.
- An identical request returned the saved response after a backend restart. Verification against the latest upstream application changes is pending. The team Sheet remains open until that integration is verified.

## System Rules dashboard · 21 November 2026

The dashboard shows active platform built-ins in evaluation order, saves editable drafts in PostgreSQL, and previews candidate conditions against one selected synthetic run. Active rule execution and Merchant Policy retain their existing behavior. See the [requirements](requirements/system-rules.md), [UI](ui/system-rules.md), and [runbook](simulation-runbook.md#system-rules).

- Go race tests and vet passed. Frontend type checking and the Next.js production build passed on Node.js 24.
- Live API checks with PostgreSQL and Redis verified the fresh rule catalogue, built-in protection, typed and stage-aware validation, draft edits, stale-version rejection, and persistence after a backend restart.
- Repeated previews returned the same results and skipped events with missing signals. Both the saved decision record and a fresh same-seed run were unchanged after draft and preview operations.
- Chrome checks on desktop and mobile covered navigation, search and filters, built-in details, draft creation and editing, retry and conflict states, and the one-to-five-condition limit. Changing stages cleared unavailable signals. A saved-run preview evaluated 12 events and matched 5, with matching event IDs shown in the editor.

## Simulation Lab checkout contact · 21 November 2026

The first order displays the submitted synthetic contact in both stages. The full contact stays in browser memory, and Go receives only the normalized `.test` email domain. See the [requirements](requirements/simulation-lab.md), [UI](ui/simulation-lab.md), and [runbook](simulation-runbook.md#verify-checkout-contact-and-merchant-policy).

- Go race tests and vet passed. Frontend type checking and the Next.js production build passed on Node.js 24.
- Live API tests with PostgreSQL and Redis verified domain validation, case-insensitive replay, a 409 for a different domain under the same run ID, first-order scope, exact policy matching, disabled and non-flight policy behavior, and timeout precedence.
- Chromium verified editable contact and the submitted snapshot, including invalid email and a simulated 503. Request and response checks and a PostgreSQL query confirmed that the submitted name, full email, and phone were absent from API traffic and persisted run data.

## Simulation Lab · 21 November 2026

Local implementation and verification complete for the [requirements](requirements/simulation-lab.md) and [UI](ui/simulation-lab.md). Use the [runbook](simulation-runbook.md) to start the demo and repeat the checks.

- Go race tests and vet passed. Frontend clean install, type check and Next.js production build passed on Node.js 24.
- A live Go API smoke test with PostgreSQL 16 and Redis 7 passed for all four products, concurrent idempotent replay, conflicting run IDs, deterministic results across run IDs, event ordering, duplicate PSP retries, shared Wi-Fi, rotating ring IPs, isolated labels, exact-domain policy, missing token and mock timeout. A Redis outage returned 503 and left no completed run.
- Chrome smoke checks exercised the setup, decisions and offline QA for normal e-commerce and mixed OTA, including the mobile layout. The default mixed run produced 24 decisions, deduplicated 12 PSP retries and scored 12 synthetic payment samples.
- The [Google Drive System Handbook](https://docs.google.com/document/d/1RtzQI1VIRXabDKUBPGcfQM8NxJZs1u_geFWFYC8Hq_I/edit) records the original local scope in section 12. Production rollout, production merchant authentication, live Jev, and production payment integration are future work.
