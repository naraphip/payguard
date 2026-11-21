# Delivery workflow

## Intake from team documents

For PayGuard, review new or changed materials in the [team Drive folder](https://drive.google.com/drive/folders/1lKpQdDOkrqpF61vxN_wNN2HHo9NQjNnH) and reconcile them with the [local task list](payguard-tasks.md) and its linked live action plan. Preserve recorded test evidence and flag material conflicts for the team. Follow the task list's completion and sync procedure after verification.

Run intake when the user asks to deliver work from `docs/`. Start at [Home](home.md) and [Current work](current-work.md), then read the relevant documents and follow their source links. Search the entire docs tree for related notes, including files the team has added without linking from an index. If the user supplies a commit range, review the docs diff for that range and open the affected notes in context. For a broad request, use the previous reviewed revision or listed sources in Current work if available. When there is no baseline, review the relevant docs tree and check existing implementation before selecting work. Include local docs changes when they are part of the requested input.

Distinguish accepted decisions and requirements from proposals, open questions, and superseded decisions. Use explicit status and supersession links to resolve precedence. Preserve source documents and cite them with relative links in derived notes. If sources disagree on a decision that changes what gets built, record the conflict and ask about that decision while continuing independent work. If several unrelated items are ready, use the team's stated priorities. If no priority is recorded, pick a small independent item and record the selection.

Have `knowledge-curator` synthesize the selected sources into a feature note under [Requirements](requirements/index.md). Capture the user need, agreed behavior, scope, constraints, acceptance criteria, and unresolved decisions with source links. Update the relevant indexes and [Current work](current-work.md) with the selected scope, reviewed sources or revision, and pending items. Reuse and update existing feature notes where applicable. Keep proposals visible as open items until the team makes a decision.

## Delivery stages

For a delivery request, follow stage dependencies for each task. For each delegation, supply the user's scope, relevant paths, decisions, and accepted outputs from earlier stages. At meaningful handoffs, pass the scope, stage results, and blockers to the knowledge-curator so a resumed workflow can pick up from the last verified stage.

| Stage | Agent | Handoff |
| --- | --- | --- |
| Plan | `planner` | Scope, constraints, risks, implementation steps, and measurable acceptance criteria |
| Design | `designer` | Applicable UI states, API and data contracts, compatibility decisions, and implementation notes |
| Develop | `coder` | Changed paths, implementation summary, and local check results |
| Test | `tester` | Independent results mapped to acceptance criteria, with commands, failures, and any skipped checks |
| Deploy when requested | `deployer` | Release readiness or verified rollout results for the tested revision and authorized target |

For docs-driven delivery, record the accepted implementation plan in the feature note or link it from Current work. Save UI design in [UI designs](ui/index.md) when it applies. Record API and data contracts in the feature note or a linked design note. After implementation and testing, record changed paths, the actual verification results against acceptance criteria, and any remaining work. Mark the item complete once the requested scope passes verification.

Use lightweight handoffs for small changes. Resolve routine decisions with existing conventions and documented requirements. Ask the user when a material product decision is required. Have the knowledge-curator record durable decisions and commands in the relevant project documentation.

### Parallel work and acceptance

Have the planner identify dependencies, acceptance checks, and file ownership. Start independent tasks together when their inputs are ready. For example, the tester can prepare checks against an agreed contract while the coder implements it. Run independent implementation tasks concurrently only when they have disjoint write scopes or isolated worktrees. Sequence tasks that share files, generated outputs, or mutable test services unless those resources are isolated. The main session integrates the results before final verification.

Track every in-scope acceptance criterion as pending, passed, failed, or blocked, with evidence from the tester. If a check fails, send the coder the failing criterion, reproduction steps, and observed result. Retest the fix and affected behavior, then run the applicable integrated checks on the final combined changes. Return to planning for a requirements gap or to design when a fix changes the agreed approach.

Continue the fix and retest loop while there is a concrete next step. Mark the task complete once all in-scope criteria pass on the integrated changes. If a required check is blocked by a missing decision, dependency, or permission, continue independent work and record the specific blocker and pending checks. If a failure recurs with no new evidence or viable next step, report it with the last attempted check.

If the user requests release readiness or rollout, have the deployer check readiness after testing passes. Execute deployment only when the user has authorized the identified target and the project has a documented deployment command, health checks, and a recovery procedure. Confirm that the revision being deployed matches the tested revision. If deployment or its health checks fail, report the observed state and follow the documented recovery procedure within the authorization given.

Summarize the implemented change, actual test results, and deployment status when applicable. Treat skipped or blocked checks as unverified.
