# PayGuard task list

Sources include the [PayGuard requirements folder](https://drive.google.com/drive/folders/1lKpQdDOkrqpF61vxN_wNN2HHo9NQjNnH), the [master package](https://drive.google.com/file/d/1vEKS3-get8bIeceeQG0DI_2fT7awD6v3/view), and the [live action plan](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260117). The local IDs below map to the action plan's Order values. Status reflects the live plan at intake unless local verification is recorded.

## Delivery tasks

| ID | Track | Estimate (min) | Owner | Task | Depends on | Acceptance | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PG-01 | DEMO / P0 | 35 | Backend | Define one-stage event contract and authenticated demo tenant + idempotency | Current Go API | Schema checked, duplicate events replayed, missing fields explicit | To do |
| PG-02 | DEMO / P0 | 30 | Frontend | Minimal merchant business setup / journey form | 01 | Merchant enters business model, stage and available signals | To do |
| PG-03 | DEMO / P0 | 45 | AI/Backend | LLM/seeded scenario proposal → validated Scoring Profile v1 | 02 | Go rejects unavailable/future-stage signals; profile version pinned | To do |
| PG-04 | DEMO / P0 | 45 | Backend | Go profile loader and booking risk assessment endpoint | 01,03 | Event uses active Scoring Profile, returns evidence and versions | To do |
| PG-05 | DEMO / P0 | 40 | Backend | Trusted synthetic feature/velocity computation + cold start | 01,04 | Identical seed => same observed counts; missing != 0 | To do |
| PG-06 | DEMO / P0 | 45 | Backend/AI | Jev typed risk question + timeout simulation | 04,05 | Valid answers, model/rubric versions; timeout UNRESOLVED live policy, demo handoff | To do |
| PG-07 | DEMO / P0 | 40 | Frontend | OTA/ecommerce mock checkout + evidence/report panel | 04,06 | Choose product and inspect assessment with rule hits and Jev status | To do |
| PG-08 | DEMO / P0 | 45 | Backend | Verified outcome import → merchant Baseline v1 | 04,05 | Only confirmed eligible normal; sample count and version; no rule auto-activation | To do |
| PG-09 | DEMO / P0 | 45 | QA | Fixture + idempotency + normal/fraud + timeout smoke | 01–08 | Repeatable demo 2 runs and no leaked ground truth | To do |
| PG-10 | PHASE 1 / P0 | — | Backend | Immutable event stream + label provenance + SDK missingness | Demo contract | Decision lineage, chargeback/analyst outcome differentiated | To do |
| PG-11 | PHASE 1 / P0 | — | Security | Tenant auth/RBAC, PII masking, reveal audit | 10 | Two-tenant isolation; all PII access logged | To do |
| PG-12 | PHASE 1 / P0 | — | Risk Ops/Backend | HOLD lifecycle: SLA, assignment lock, escalation and expiry | Risk policy decision | No double-action, no silent expiry policy | Blocked |
| PG-13 | PHASE 1 / P0 | — | Frontend | Transactions queue/detail + essential dashboard KPIs | 10–12 | Latency and freshness measured, filters use real data | To do |
| PG-14 | PHASE 2 / P1 | — | Backend/QA | Replay/shadow with temporal holdout + baseline versions | 10, 08 | No production action from shadow; FPR and Recall computed | To do |
| PG-15 | PHASE 2 / P1 | — | Risk/Security | Maker-checker for profile/rule/rubric/list changes | 14 | Different approver, expiry and audit; rollback new version | To do |
| PG-16 | PHASE 2 / P1 | — | Backend | Typed product modules + separate merchant policy resolver | 01, 11 | OTA/e-commerce/gateway, exact domain matching and action_source | Done |
| PG-17 | PHASE 2 / P1 | — | AI/Security | Evidence-backed narrative dossier, injection tests | 10, 12 | Post-decision only, citations validated and masked export | To do |
| PG-18 | PHASE 3 / P2 | — | Frontend/Backend | Fraud Simulation Lab graph / linked entity network | 05, 10, 16 | Graph evidence and no false-positive IP-only block | To do |
| PG-19 | PHASE 3 / P2 | — | Frontend | Advanced visual rule builder and personalized dashboards | 14–16 | No premature visual work before replay and approvals | Deferred |
| PG-20 | PRE-PROD GATE | — | Risk/QA | Real labels, shadow cycle, recall/fpr precision signoff | 10–19 | Recall>=98% only if demonstrated; conversion loss measured | Blocked |
| PG-21 | PRE-PROD GATE | — | Security | PDPA/PCI, retention, cross-border provider assessment | 11,17 | Approved security/legal review | Blocked |

The local Go and Next.js Simulation Lab provides a base for the demo. See the [implementation task breakdown](payguard-task-breakdown.md) for reusable code, dependencies, and acceptance checks. Use synthetic data for the demo.

## Verified implementation progress

The Action Plan status snapshot above records team tracking. The table below summarizes local implementation and verification. PG-20 requires approved real outcomes and shadow results, and PG-21 requires Security and Legal review. [Earlier Simulation Lab merge](https://github.com/pwatakul/payguard/pull/1), [earlier Contact and System Rules merge](https://github.com/pwatakul/payguard/pull/2)

| ID | Verified in the local sandbox | Evidence |
| --- | --- | --- |
| PG-01–02 | Versioned booking intake verifies authentication and idempotency. The merchant setup form saves and reloads the business model and signals. | [Booking intake](current-work.md#pg-01-booking-intake), [merchant setup](current-work.md#pg-02-merchant-setup) |
| PG-03–06 | Approved profile versions, booking assessment, trusted velocity and cold-start evidence, typed mock Jev results and the timeout handoff. | [Booking assessment](current-work.md#pg-03-through-pg-06-booking-assessment) |
| PG-07–10 | Synthetic mock checkout, baseline activation, repeatable fixtures and tenant-scoped lineage. | [Checkout and lineage](current-work.md#pg-07-through-pg-10-mock-checkout-baseline-and-booking-lineage) |
| PG-11–19 | Role isolation, HOLD cases, Risk Operations, pinned shadow replay, approvals, product modules, dossier, tenant-scoped network evidence, and dashboard preferences. | [Local evaluation](current-work.md#pg-11-through-pg-19-local-evaluation) |

Sandbox work outside these task rows includes the supplied PayGuard logo and the integrated provider enrichment and Omise test checkout. [Integration handoff](current-work.md#integrated-main-candidate)

## Decisions

See the [live review decisions](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260116) for demo recommendations and approval details. An open decision stays open until its owner approves it.

| ID | Decision | Owner | Impact | Status |
| --- | --- | --- | --- | --- |
| DEC-01 | Timeout: UNSUCCESSFUL manual vs FAILSAFE_HOLD vs rules-only | Risk Ops Lead | Must decide before live payment integration | Open |
| DEC-02 | On hold expiry: auto-release / auto-reject / escalate | Fraud Ops Lead | Needed for analyst queue | Open |
| DEC-03 | Risk answer choices vs numeric score and actions | Risk Lead + Backend | API contract needs signing | Open |
| DEC-04 | Minimum sample count for new merchant baseline | Risk Lead | Needed for demo learning replay | Open |
| DEC-05 | What constitutes confirmed fraud vs analyst disposition | Risk Lead + QA | Metrics correctness | Open |
| DEC-06 | Merchant domain BLOCK vs platform fraud evidence | Product + Security | Precedence and audit | Approved concept |
| DEC-07 | Who approves rule/blacklist changes | Security + Risk Lead | Before production | Open |
| DEC-08 | Who writes narrative; injection and evidence validation | AI/Compliance | Dossier should not block checkout | Open |
| DEC-09 | How to avoid label leakage and misleading graph | QA + Risk Lead | Needed for credible demo | Approved concept |
| DEC-10 | 6–7h hackathon vs Weeks 1–8+ platform | Tech Lead | Prevents over-scoping | Open |

## Completion and sync

Write verified completions to [01 ACTION PLAN LATEST](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260117) in the PayGuard team workbook. Match the current Order value and task title, then update only the matching Status cell using the tab's completion convention. Use `Done` if the tab has no completion convention. Check the current headers and cell before writing and read the value back afterward. If an order is missing, ambiguous, or has been reused, leave the sheet unchanged and report the mismatch.

Mark a task complete only when its acceptance checks pass. Update the local status and record test evidence, then sync the matching sheet row. If the sheet update fails, record it as pending and retry by task ID on the next run. Preserve unrelated sheet values and team notes.

### Completion log

| Task ID | Verification evidence | Sheet sync |
| --- | --- | --- |
| PG-16 | Typed OTA, e-commerce, and gateway modules, independent merchant policy outputs, exact-domain and precedence checks, and pinned replay passed in the local suites. [Policy verification](current-work.md#ai-rule-management) · [Product module verification](current-work.md#pg-11-through-pg-19-local-evaluation) | 21 November 2026. Set `01 ACTION PLAN LATEST!H18` to `Done` and verified by readback. |

This log records team Action Plan completions and Sheet sync. See the [verified implementation progress](#verified-implementation-progress) for local verification evidence.

