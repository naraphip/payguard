# PayGuard implementation task breakdown

Parent IDs match the [local task list](payguard-tasks.md) and the [live action plan](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260117). This breakdown uses the live plan, the [master requirements package](https://drive.google.com/file/d/1vEKS3-get8bIeceeQG0DI_2fT7awD6v3/view), and a source review at revision `52c1b5e`. The linked System Handbook requires access. Reconcile this breakdown against the handbook when available.

The [Simulation Lab](requirements/simulation-lab.md) already has synthetic storefronts, seeded event generation, run-level replay, Redis velocity, typed mock Jev results, and a tenant-scoped synthetic graph. Reuse these pieces. Verify each parent task against its own acceptance criteria before updating its Sheet status. Source review alone establishes overlap, and runtime checks remain pending because Go was unavailable in this environment.

## Local development environment

Environment tasks enable the demo backlog and are tracked here. The `ENV` IDs are local.

| ID | Task | Dependencies | Acceptance criteria | Verification |
| --- | --- | --- | --- | --- |
| ENV-01 | Add a repeatable Docker Compose startup. | Docker with Compose | `docker compose config -q` passes. `docker compose up --build -d --wait` builds and starts all four services from the repo root with Docker as the only required host runtime. Published ports bind to loopback. | Passed. All four services reported healthy and published ports were loopback-only. |
| ENV-02 | Containerize the Go backend. | ENV-04, ENV-05 | The Go image builds using the repository's Go module. The API waits for healthy PostgreSQL and Redis. `GET http://127.0.0.1:8081/healthz` returns HTTP 200 and `status: ready`. | Passed in Docker Compose. |
| ENV-03 | Containerize the TypeScript/Next.js frontend. | ENV-02 | The production build and TypeScript checks pass in Docker. The UI responds on `http://127.0.0.1:3000/`. Requests to `/api/v1/simulation/` reach the Go API through Next.js. | Passed. The production build, UI request, and simulation proxy worked. |
| ENV-04 | Configure PostgreSQL 16 and persistent storage. | Docker with Compose | PostgreSQL is healthy on startup and the API initializes its demo tables. Compose stores data in the named volume. A saved run remains readable after ordinary `docker compose down` and startup. | Database health, schema initialization, and saving a run passed. The volume is configured. The stop/start persistence check is pending. |
| ENV-05 | Configure Redis 7 for simulation velocity. | Docker with Compose | Redis answers its health check, and the Go API can reach it over the Compose network. | Passed. Redis and the API both reported healthy. |
| ENV-06 | Document and smoke-test local startup. | ENV-01 through ENV-05 | The README gives startup, app URL, logs, and shutdown commands. The runbook's synthetic scenario can be submitted through port 3000, then read back as a QA report with 12 payment samples. | Passed. The report returned 12 samples via the frontend proxy. |

See the [README](../README.md) and [simulation runbook](simulation-runbook.md) for commands.

## Demo work

| Parent | Implementation tasks | Dependencies | Acceptance checks |
| --- | --- | --- | --- |
| PG-01 | 01a Define the versioned booking-event contract and responses. 01b Resolve the demo tenant from authentication. 01c Persist idempotency by tenant and event, reusing the current run replay pattern. | Start here | Reject invalid stage and unauthorized tenant. Replay the same event and reject a changed payload under the same key. Distinguish absent fields from zero. |
| PG-02 | 02a Design and build the merchant setup form. 02b Save and reload the business model, stage, and available signals. | PG-01 contract. Form design can start in parallel. | Save and reload a valid setup. Show a field-level error for an invalid stage or available signal. |
| PG-03 | 03a Build a deterministic fallback profile proposal from merchant setup. 03b Validate proposed signals against the stage catalogue and persist an approved, immutable profile version. 03c Route optional LLM output through the same validator. | PG-02 setup shape | Reject unknown and future-stage features. The fallback yields the same proposal for the same inputs. An assessment can reference an approved version. |
| PG-04 | 04a Load the tenant's active approved profile. 04b Connect the booking endpoint to shared evaluation logic. 04c Return evidence and pinned profile, rule, and baseline lineage where applicable. | PG-01, PG-03 | The same event and pinned inputs produce traceable results. An inactive profile cannot affect the assessment. |
| PG-05 | 05a Reuse Redis distinct-order velocity and record feature provenance or missingness. 05b Expose cold-start state before an eligible baseline exists. | PG-01, PG-04 | Seeded counts reproduce. Missing input stays distinguishable from zero. Cold-start state appears consistently. |
| PG-06 | 06a Extend typed mock Jev evidence with model and rubric versions. 06b Test demo timeout and operator handoff. | PG-04, PG-05. DEC-01 and DEC-03 for live behavior. | Valid typed output carries lineage. A demo timeout produces a visible unresolved assessment and no payment action. |
| PG-07 | 07a Connect the existing synthetic checkout to the booking assessment. 07b Show rule hits, Jev status and reason codes, and available version metadata. | PG-04, PG-06 | An OTA or e-commerce synthetic checkout shows the matching assessment and handles timeout visibly. |
| PG-08 | 08a Define an outcome import with provenance. 08b Build immutable baseline versions from verified eligible normal outcomes. 08c Show sample count and cold-start versus active baseline. | PG-04, PG-05, DEC-04, DEC-05 | Ineligible outcomes leave the baseline unchanged. Activation respects the approved sample threshold and leaves rule activation separate. |
| PG-09 | 09a Expand existing Go tests for the new endpoint and feature paths. 09b Run two repeatable demo passes and capture acceptance evidence. | PG-01 through PG-08 for final acceptance. Test drafting can begin with PG-01. | Verify deterministic normal and fraud fixtures, duplicate replay, tenant scope, timeout, and QA labels excluded from assessment inputs. |

## Later phases

| Parent | Implementation split and verification |
| --- | --- |
| PG-10 | Add immutable event and assessment lineage. Model outcome provenance and SDK missingness separately. Verify traceability and out-of-order ingestion. |
| PG-11 | Define tenant access and roles. Add masking and audited reveal. Verify cross-tenant denial and authorized reveal audit records. |
| PG-12 | Model HOLD deadlines, assignment locking, and escalation. Implement expiry after DEC-02 approval. Verify concurrent claim and deadline handling. |
| PG-13 | Build queue filters, details, and required KPI views. Verify filters, freshness, and latency against recorded data. |
| PG-14 | Build pinned replay and shadow evaluation. Verify held-out metric calculation and that shadow runs produce no payment action. |
| PG-15 | Add approval, activation, expiry, and rollback as a new version. Verify self-approval is rejected and actions are audited. |
| PG-16 | Add versioned product schemas and separate merchant policy evaluation. Verify exact domain matching, tenant scope, and action attribution. |
| PG-17 | Generate dossiers asynchronously from validated evidence references. Verify masked export and rejection of unsupported citations. |
| PG-18 | Reuse the synthetic graph. Define broader approved entity scope, then verify tenant isolation and that a shared IP alone cannot trigger a block. |
| PG-19 | Design and build the advanced rule editor after replay and approvals. Verify validation parity with the underlying rule format. |
| PG-20 | Obtain approved verified labels, run a temporal shadow evaluation, and submit metrics and conversion impact for sign-off. |
| PG-21 | Confirm privacy and PCI scope, retention, and provider handling. Record approved security and legal sign-off. |

Keep completion updates at the parent PG ID level under the [task list sync procedure](payguard-tasks.md#completion-and-sync).
