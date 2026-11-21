# Delivery workflow

For a delivery request, run the stages in order. For each delegation, supply the user's scope, relevant paths, decisions, and accepted outputs from earlier stages. At meaningful handoffs, pass the scope, stage results, and blockers to the knowledge-curator so a resumed workflow can pick up from the last verified stage.

| Stage | Agent | Handoff |
| --- | --- | --- |
| Plan | `planner` | Scope, constraints, risks, implementation steps, and measurable acceptance criteria |
| Design | `designer` | Applicable UI states, API and data contracts, compatibility decisions, and implementation notes |
| Develop | `coder` | Changed paths, implementation summary, and local check results |
| Test | `tester` | Independent results mapped to acceptance criteria, with commands, failures, and any skipped checks |
| Deploy | `deployer` | Release readiness or verified rollout results for the tested revision and authorized target |

Use lightweight handoffs for small changes. Resolve routine decisions with existing conventions and documented requirements. Ask the user when a material product decision is required. Have the knowledge-curator record durable decisions and commands in the relevant project documentation.

If design exposes a requirements gap, return it to planning. If tests fail, send the reproducible findings to the coder, then ask the tester to rerun affected checks on the updated code. Return to design if the fix changes the agreed approach. Continue while each iteration makes progress. If the same failure recurs without new evidence, report the blocker and the checks still pending.

After testing passes, have the deployer check readiness. Execute deployment only when the user has authorized the identified target and the project has a documented deployment command, health checks, and a recovery procedure. Confirm that the revision being deployed matches the tested revision. If deployment or its health checks fail, report the observed state and follow the documented recovery procedure within the authorization given.

Summarize the implemented change, actual test results, and deployment status. Treat skipped or blocked checks as unverified.
