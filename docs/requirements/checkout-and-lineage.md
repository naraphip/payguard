# Mock checkout, baseline and booking lineage

Status **Local acceptance checks passed**. PG-07 through PG-10 connect the demo storefront to the booking assessment, capture outcome provenance, and expose the recorded booking history for review. The local smoke check exercises the integrated flow twice. [Verification](../current-work.md#pg-07-through-pg-10-mock-checkout-baseline-and-booking-lineage) · [PayGuard task list](../payguard-tasks.md#delivery-tasks) · [Implementation breakdown](../payguard-task-breakdown.md#demo-work) · [Runbook](../simulation-runbook.md#mock-checkout-and-booking-lineage)

| Task | Acceptance criteria |
| --- | --- |
| PG-07 Mock checkout | Each OTA or e-commerce product choice produces a matching synthetic booking and displays its assessment. Show the rule hits, versions, and Jev result, including `UNRESOLVED` and manual review on timeout. Retry the same pending booking with its original identifiers. |
| PG-08 Verified outcomes and baseline | Keep source, source reference, label, and timestamps for imported outcomes. Only distinct bookings with a verified demo normal outcome and a completed mock assessment advance the baseline. With a configured local sample threshold, qualifying samples publish immutable baseline versions. Ineligible outcomes preserve the current baseline and rule version. |
| PG-09 Booking smoke | Two runs of the same relative fixtures produce matching normal, elevated, and timeout assessment signatures. Exercise replay, merchant scope, explicit missingness, out-of-order delivery, and exclusion of QA labels from the assessment contract. |
| PG-10 Booking lineage | Read the tenant-owned stored booking, immutable assessment, and separate outcomes. Preserve event and receipt timestamps for out-of-order imports. Distinguish observed, unavailable, and unknown SDK status from other signal missingness and outcome provenance. |

The demo baseline uses `PAYGUARD_DEMO_BASELINE_MIN_SAMPLES` to set a positive sample threshold locally. Without a threshold the service does not activate new baselines. A booking pins the baseline active at acceptance, including an empty baseline before activation. Outcome import preserves the assessment and any baseline version it previously recorded. The team still needs to decide the threshold and production label eligibility in DEC-04 and DEC-05. [Decisions](../payguard-tasks.md#decisions)
