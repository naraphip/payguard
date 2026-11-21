# Booking assessment

Status **Local demo implemented and verified**. PG-03 through PG-06 let a demo merchant review a Scoring Profile generated from their saved setup and use it to assess an authenticated booking with traceable evidence. The demo uses a deterministic proposal, a cold-start baseline, and a mock Jev service. [Verification](../current-work.md#pg-03-through-pg-06-booking-assessment) · [PayGuard task list](../payguard-tasks.md#delivery-tasks) · [Implementation breakdown](../payguard-task-breakdown.md#demo-work) · [Merchant setup](merchant-setup.md) · [UI](../ui/booking-assessment.md)

| Task | Acceptance criteria |
| --- | --- |
| PG-03 Scoring Profile | Equivalent setups produce the same candidate. Go rejects unsupported or future-stage signals and signals the setup did not declare. Explicit approval activates an immutable version for the authenticated merchant. |
| PG-04 Booking assessment | The operation reads an accepted booking and the merchant’s active profile, records assessment evidence and versions, and replays the saved result for the same event and request. Previous assessments retain their pinned profile after activation changes. |
| PG-05 Trusted features | Count distinct tenant-scoped booking events in the window `(T − 60s, T]`, where `T` is the target booking's event time. Use only history received by the target booking's receipt time. Replaying the same fixture history preserves counts. Evidence distinguishes an absent amount from zero and shows `COLD_START` with no eligible baseline version. |
| PG-06 Mock Jev | Return a typed question and answer with model and rubric versions. A simulated timeout returns `UNRESOLVED` with a `MANUAL_REVIEW` handoff. The demo issues no payment action. |

The launch contract accepts schema version `1`, business model `ota` or `ecommerce`, and stage `booking`. Profile setup always includes `occurredAt` and may include `amountMinor`. A candidate selects from signals available in that setup. Approval and assessment use the same demo bearer authentication as booking intake. Booking event IDs serve as the distinct booking identifier in the merchant-wide velocity calculation. Profile and assessment snapshots persist in PostgreSQL. A later task defines eligible baseline versions. [Booking intake](../simulation-runbook.md#booking-event-intake) · [API commands](../simulation-runbook.md#booking-assessment)

The task list tracks live timeout and risk-answer policy decisions separately. Merchant checkout integration and outcome import are PG-07 and PG-08. [Decisions](../payguard-tasks.md#decisions)
