# Booking assessment UI

Status **Local demo implemented and verified**. The `/settings/booking-assessment` screen takes the setup saved in the same browser, then guides the merchant through profile approval and a synthetic booking assessment. [Requirements](../requirements/booking-assessment.md) · [Verification](../current-work.md#pg-03-through-pg-06-booking-assessment)

1. Open Booking Assessment and load the saved merchant setup. If the setup is missing or invalid, use Merchant Setup to save a valid configuration.
2. Enter the configured local demo bearer token. Keep the input in memory while the page is open.
3. Generate a proposal, review its stage and signals, and approve it explicitly. Display the approved profile version.
4. Run a synthetic booking assessment. Leave the amount blank to omit it or enter zero to send an explicit zero. Choose the normal mock response or simulated timeout.
5. Inspect the outcome, recorded profile and rule versions, baseline state, features and provenance, and the Jev question, answer, model, and rubric. Show the manual review handoff when a simulated timeout leaves the assessment unresolved.

Use labelled controls and visible feedback for pending, successful, and failed requests. Keep the saved setup and currently entered values available for recovery after an error.
