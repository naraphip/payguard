# PayGuard project brief

PayGuard evaluates booking and payment risk for travel and e-commerce merchants. The current repository delivers a local Simulation Lab for synthetic checkout scenarios. The earlier numeric MVP described in the Drive package has a separate runtime and contract.

| Component | Responsibility |
| --- | --- |
| Next.js App Router, TypeScript, Tailwind CSS | Storefront selection, scenario controls, event decisions, Cytoscape evidence graph and offline QA UI |
| Go HTTP API | Validation, deterministic generation, event sequencing, platform rules, mock Jev and demo merchant policy |
| Redis | Per-stage, event-time 60-second velocity by synthetic entity |
| PostgreSQL | Immutable run snapshots and a QA-only ground truth schema |

Start with the [Simulation Lab requirements](requirements/simulation-lab.md), [UI design](ui/simulation-lab.md), and [runbook](simulation-runbook.md).

The Sandbox [Decision Engine](requirements/decision-engine.md) keeps global Platform Rules, tenant Merchant Policies, and administrator AI configuration independent. The OpenAI composer returns proposed rules for Go validation. In Sandbox, OpenRouter Jev can assess new merchant scenario transactions and run a fixed synthetic diagnostic. Legacy Booking API and batch simulation keep their mock Jev paths.

## Deployment

Run `docker compose up --build -d --wait` from the repository root to start the Go API, TypeScript/Next.js frontend, PostgreSQL, and Redis. Open the [Dashboard](http://127.0.0.1:3000/dashboard) after the services are healthy, or go directly to [Simulation Lab](http://127.0.0.1:3000/simulation-lab). PostgreSQL stores data in a named volume that persists when the stack stops.

For host development, run `docker compose up -d --wait postgres redis` and start Go and Next.js using the [README instructions](../README.md). Production deployment, tenant authentication, and live payment or Jev integration remain outside this milestone.
