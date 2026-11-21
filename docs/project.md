# PayGuard project brief

PayGuard evaluates booking and payment risk for travel and e-commerce merchants. The current repository delivers a local Simulation Lab for synthetic checkout scenarios. The earlier numeric MVP described in the Drive package has a separate runtime and contract.

| Component | Responsibility |
| --- | --- |
| Next.js App Router, TypeScript, Tailwind CSS | Storefront selection, scenario controls, event decisions, Cytoscape evidence graph and offline QA UI |
| Go HTTP API | Validation, deterministic generation, event sequencing, platform rules, mock Jev and demo merchant policy |
| Redis | Per-stage, event-time 60-second velocity by synthetic entity |
| PostgreSQL | Immutable run snapshots and a QA-only ground truth schema |

Start with the [Simulation Lab requirements](requirements/simulation-lab.md), [UI design](ui/simulation-lab.md), and [runbook](simulation-runbook.md).

## Deployment

Local development uses Docker Compose for Redis and PostgreSQL and local Go and Node.js processes. Production deployment, tenant authentication, and live payment or Jev integration remain outside this milestone.
