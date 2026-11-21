# Delivery workflow

## Intake from team documents

For PayGuard, review new or changed materials in the [team Drive folder](https://drive.google.com/drive/folders/1lKpQdDOkrqpF61vxN_wNN2HHo9NQjNnH) and reconcile them with the [local task list](payguard-tasks.md) and its linked live action plan. Preserve recorded test evidence and flag material conflicts for the team. Follow the task list's completion and sync procedure after verification.

Run intake when the user asks to deliver work from `docs/`. Start at [Home](home.md) and [Current work](current-work.md), then read the relevant documents and follow their source links. Search the entire docs tree for related notes, including files the team has added without linking from an index. If the user supplies a commit range, review the docs diff for that range and open the affected notes in context. For a broad request, use the previous reviewed revision or listed sources in Current work if available. When there is no baseline, review the relevant docs tree and check existing implementation before selecting work. Include local docs changes when they are part of the requested input.

Distinguish accepted decisions and requirements from proposals, open questions, and superseded decisions. Use explicit status and supersession links to resolve precedence. Preserve source documents and cite them with relative links in derived notes. If sources disagree on a decision that changes what gets built, record the conflict and ask about that decision while continuing independent work. If several unrelated items are ready, use the team's stated priorities. If no priority is recorded, pick a small independent item and record the selection.

Have `knowledge-curator` synthesize the selected sources into a feature note under [Requirements](requirements/index.md). Capture the user need, agreed behavior, scope, constraints, acceptance criteria, and unresolved decisions with source links. Update the relevant indexes and [Current work](current-work.md) with the selected scope, reviewed sources or revision, and pending items. Reuse and update existing feature notes where applicable. Keep proposals visible as open items until the team makes a decision.

## Delivery stages

For a delivery request, run the stages in order. For each delegation, supply the user's scope, relevant paths, decisions, and accepted outputs from earlier stages. At meaningful handoffs, pass the scope, stage results, and blockers to the knowledge-curator so a resumed workflow can pick up from the last verified stage.

| Stage | Agent | Handoff |
| --- | --- | --- |
| Plan | `planner` | Scope, constraints, risks, implementation steps, and measurable acceptance criteria |
| Design | `designer` | Applicable UI states, API and data contracts, compatibility decisions, and implementation notes |
| Develop | `coder` | Changed paths, implementation summary, and local check results |
| Test | `tester` | Independent results mapped to acceptance criteria, with commands, failures, and any skipped checks |
| Deploy when requested | `deployer` | Release readiness or verified rollout results for the tested revision and authorized target |

For docs-driven delivery, record the accepted implementation plan in the feature note or link it from Current work. Save UI design in [UI designs](ui/index.md) when it applies. Record API and data contracts in the feature note or a linked design note. After implementation and testing, record changed paths, the actual verification results against acceptance criteria, and any remaining work. Mark the item complete once the requested scope passes verification.

Use lightweight handoffs for small changes. Resolve routine decisions with existing conventions and documented requirements. Ask the user when a material product decision is required. Have the knowledge-curator record durable decisions and commands in the relevant project documentation.

If design exposes a requirements gap, return it to planning. If tests fail, send the reproducible findings to the coder, then ask the tester to rerun affected checks on the updated code. Return to design if the fix changes the agreed approach. Continue while each iteration makes progress. If the same failure recurs without new evidence, report the blocker and the checks still pending.

If the user requests release readiness or rollout, have the deployer check readiness after testing passes. Execute deployment only when the user has authorized the identified target and the project has a documented deployment command, health checks, and a recovery procedure. Confirm that the revision being deployed matches the tested revision. If deployment or its health checks fail, report the observed state and follow the documented recovery procedure within the authorization given.

Summarize the implemented change, actual test results, and deployment status when applicable. Treat skipped or blocked checks as unverified.
