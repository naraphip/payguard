# System Rules dashboard

Status **Implemented and verified for the local sandbox**. Operators need a place to see the platform rules that the Simulation Lab actually runs and try changes against saved synthetic decisions before considering a rollout. The settings page is at `/settings/system-rules`. See the [UI design](../ui/system-rules.md) and [API and smoke checks](../simulation-runbook.md#system-rules).

| Area | Acceptance criterion |
| --- | --- |
| Active rules | Show the read-only built-in rules, their conditions and actions, in evaluation order. Their precedence and the recorded Simulation Lab results are preserved. |
| Rule drafts | Create and edit PostgreSQL-backed drafts. Reloading returns the saved drafts. Updates require the current version and surface a conflict if another update won. |
| Validation | Accept one to five AND conditions. Check stage and signal availability, numeric and boolean operators and values, and the selected `REVIEW` or `CHALLENGE` action on the server. |
| Preview | Apply the candidate conditions to recorded features in a selected synthetic run. Return eligible, matched and skipped counts with matching event IDs. Stage-matching events with a missing signal are skipped. |
| Scope | Creating, editing or previewing a draft leaves the active rules, saved simulation evidence, and Merchant Policy behavior unchanged. |

This milestone is local draft editing and preview. Full [PG-14 replay and shadow, PG-15 approval, and PG-19 advanced rule tooling](../payguard-tasks.md#delivery-tasks) remain open.
