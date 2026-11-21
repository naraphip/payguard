# Project delivery

Use the [project brief](docs/project.md), [current work](docs/current-work.md), and [delivery workflow](docs/workflow.md) for feature, fix, and docs-driven delivery requests. For docs-driven delivery, start with the intake in the workflow. The request can name a feature or source note, give a commit range, or ask for actionable updates across `docs/`. Treat team documents as source material and preserve the originals.

The main Codex session coordinates delivery. Delegate to the project subagents in order: `planner`, `designer` when design decisions are needed, `coder`, `tester`, and `deployer` for release readiness or an authorized rollout. Pass the relevant scope, paths, acceptance criteria, and verified results at each handoff. Keep handoffs lightweight for small changes and sequence edits that touch the same files.

When verification fails, send the findings back to `coder` and have `tester` verify the updated changes. Follow the delivery workflow if the failure keeps recurring. Delegate documentation updates to `knowledge-curator` at meaningful handoffs and wait for those updates before starting a stage that relies on them.

Finish docs-driven delivery with verified test results and updated handoff notes. Run the deployment stage only when the user requests release readiness or a rollout. Follow the deployment authorization and verification rules in the delivery workflow.
