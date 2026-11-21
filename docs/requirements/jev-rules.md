# Jev Rules

Status **Local demo implemented and verified**. A merchant can use the demo to turn a rule from AI chat into a review policy for booking assessments. SEON's custom rule builder provides the reference for translating natural language into typed conditions, reviewing those conditions, and testing a candidate on saved data before publication. [SEON custom rules and parameters](https://docs.seon.io/knowledge-base/transactions-scoring/custom-rules-and-parameters)

The evaluator uses the evidence and lineage from the existing mock Jev booking flow. Score adjustments, payment actions, production Jev and AI provider connections, and the maker-checker policy in PG-15 and DEC-07 are outside the demo scope. [Booking assessment](booking-assessment.md) · [PayGuard task list](../payguard-tasks.md) · [Jev Rules UI](../ui/jev-rules.md)

Platform Admin manages a selected merchant's review rules under **Merchant Details → Merchant Rules**. The merchant test tools evaluate conditions on `amountMinor`. Publish, edit and removal create new ruleset revisions. Existing results retain the revision used. The legacy Booking API uses the demo booking credential and its own rule catalogue with both signals below. Configure or test the advisory provider in the separate AI Configuration workspace. [Merchant workflow and APIs](../simulation-runbook.md#composer-and-policy-apis) · [AI Configuration](decision-engine.md#jev-configuration)

## Rule contract

A custom rule has a name of 1–80 characters without control characters, the `REVIEW` action, and one to five conditions joined by AND. Each condition compares one observed booking signal with a nonnegative JavaScript-safe integer, up to 9,007,199,254,740,991. The operators are `gte`, `gt`, `lte`, `lt`, and `eq`.

| Signal | Evidence |
| --- | --- |
| `amountMinor` | Booking amount in minor units. The approved Scoring Profile must include the amount, and the assessment must have an observed value. An explicit zero is a valid observed value. |
| `bookingVelocity60s` | The existing merchant-scoped count in the booking's trailing 60-second window, including the current booking. |

JSON body for a booking amount of at least 50,000 minor units and at least three bookings in the velocity window:

```json
{
  "name": "High amount and booking velocity",
  "action": "REVIEW",
  "conditions": [
    {"signal": "amountMinor", "operator": "gte", "value": 50000},
    {"signal": "bookingVelocity60s", "operator": "gte", "value": 3}
  ]
}
```

Give your AI chat this instruction with the policy you want it to draft.

```text
Turn my booking policy into one Jev rule. Return a JSON object with name, action
set to REVIEW, and a conditions array with one to five AND conditions. Each
condition has signal, operator, and value. Allowed signals are amountMinor
(booking amount in minor units) and bookingVelocity60s (bookings in 60 seconds).
Allowed operators are gte, gt, lte, lt, and eq. Use nonnegative integers up to
9007199254740991. Ask me for any missing units, window, or threshold. If the
policy requires another signal, operator, action, or OR logic, tell me which
part Jev cannot express.

Policy: <describe the bookings you want reviewed>
```

Paste the resulting JSON into **Rule chat** to inspect and preview the draft before pushing it. See the [Jev Rules runbook](../simulation-runbook.md#jev-rules) for HTTP use.

## API and evaluation

All routes use the configured merchant demo bearer token. Send the JSON object above directly as the body of either POST request.

| Route | Behavior |
| --- | --- |
| `GET /api/v1/booking/jev/rules` | Returns the active `version` and custom `rules` for the authenticated merchant. Each rule includes its stable `id`. The default version is `booking-rules-v1` with an empty custom rule list. |
| `POST /api/v1/booking/jev/rules/preview` | Returns `evaluated`, `matched`, and `missing` for up to 200 recent saved assessments belonging to that merchant. The total sampled is `evaluated`. The `missing` count includes assessments lacking required observed evidence. Preview reads pinned features and leaves assessments and published rules intact. |
| `POST /api/v1/booking/jev/rules` | Publishes the validated rule for that merchant and returns the active snapshot. Publishing appends to the merchant's rules. Repeating the same named rule and conditions, including reordering the AND clauses, returns the active snapshot without appending a duplicate. |

The default `BOOKING_VELOCITY_60S` threshold is five bookings. Each new assessment pins the active Jev ruleset in `ruleVersion`, or in `jevRuleVersion` if `ruleVersion` already identifies an approved risk operations rule. A version identifies a fixed set of rules within the merchant’s history. In mock mode, a matching custom rule records its ID in `ruleHits` and sets the assessment to `REVIEW` with a `MANUAL_REVIEW` handoff. Jev reports `ELEVATED` with the default rubric. An approved rubric continues to determine the Jev answer independently. A timeout yields `UNRESOLVED` and `MANUAL_REVIEW` even when rules match. Requests replaying a saved assessment receive the original result, including its pinned ruleset version and hits.

## Acceptance checks

- An operator can turn a supported prompt or external AI JSON into a typed draft, inspect the conditions, preview it, and explicitly publish it.
- Preview and evaluation agree on threshold boundaries and fail to match when required evidence is missing. Both distinguish an observed zero amount from a missing amount.
- Invalid conditions and unauthorized requests cannot publish. Each merchant sees and evaluates its own snapshot.
- Identical publish retries do not duplicate rules. Concurrent distinct publications retain both rules.
- New assessments record the applied version and matches. A timeout preserves its handoff, and replay after publication returns the previously stored assessment.

[Verification record](../current-work.md#jev-rules)
