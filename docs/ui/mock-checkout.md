# Mock checkout UI

Status **Local browser checks passed**. The `/settings/mock-checkout` page connects the synthetic product selection to the merchant booking API. [Task breakdown](../payguard-task-breakdown.md#demo-work)

1. Select a flight, hotel room, physical item, or digital item. Check that the order summary reflects the selected product.
2. Enter the local demo merchant token and approve the demo profile. Use the normal mock Jev response or select **Simulate timeout**.
3. Submit the synthetic checkout. Follow the booking receipt to the assessment, including rule hits, feature evidence, profile and baseline versions, and mock Jev status, answer, model, and rubric.
4. On timeout, show `UNRESOLVED` with a manual-review handoff. On a request failure, keep the pending booking available for retry with its original identifiers.

The page keeps the token in memory for the current session. A mock checkout creates a synthetic booking and can generate assessment evidence. It does not submit a payment.
