# AI Rule Management

The composer turns a Thai or English policy into a typed proposal for review and saving. The current implementation targets the demo merchant transaction flow and the existing Platform Rules draft workspace. [Setup and API examples](../simulation-runbook.md#ai-rule-composer) · [Current verification](../current-work.md#ai-rule-management)

## Administration and execution

| Component | Responsibility |
| --- | --- |
| Shared composer | Go calls OpenAI Chat Completions directly for a structured `READY` proposal or `CLARIFICATION`. It validates returned fields and types and revalidates every save. The request contains the operator's instruction and supported fields. |
| Platform Rules | The demo administrator composes and saves a global `DRAFT` in the existing Platform Rules store. A saved shared Simulation Lab run supplies evidence for submission. A separate checker can approve a revision for new Simulation Lab runs. The built-in rules retain their existing version and behavior. |
| Merchant Policy | The merchant's own UUID-scoped bearer credential supplies the tenant. Rules are persisted as immutable PostgreSQL versions. The deterministic policy evaluator runs on new merchant scenario transactions and records the matching rule IDs and versions. |
| Jev | Mock or OpenRouter Jev supplies the risk assessment for merchant scenario transactions. The administrator can test the effective settings with a fixed synthetic fixture in [AI Configuration](decision-engine.md#jev-configuration). |
| Decision resolver | The resolver preserves the platform, merchant, and final outputs in the transaction. The highest precedence belongs to a platform `BLOCK`, then a merchant `BLOCK`. Other actions rank `REVIEW`, `CHALLENGE`, `UNRESOLVED` or `UNSUCCESSFUL`, and `ALLOW`. The platform wins a tie. A merchant policy match records a business decision and leaves confirmed fraud labels to the existing outcome path. |

## Merchant business context and fields

The profile reads the existing merchant record, its saved setup, optional `policyIntegration` metadata from registration, and up to 200 recent transactions belonging to that merchant. Product availability comes from the declared catalogue or a recorded product. Amount availability comes from a declared booking amount signal or an observed amount. Domain availability comes from declared domain support or an observed normalized domain. The composer exposes missing capabilities so the operator can adjust the policy. The ordinary rule-creation flow uses the saved profile directly. An available scenario amount profile appears as `merchant-scenario-amount-v1`. The existing Booking Assessment profile workflow retains its own approvals and historical versions.

| Field | Type and operators | Availability and missing data |
| --- | --- | --- |
| `product.type` | Enum, `eq` | `flight_ticket` and `hotel_room` use `BOOKING`. `physical` and `digital` use `CHECKOUT`. `payment_gateway` uses `CHECKOUT` in merchant scenario simulation. The proposal includes the corresponding products and stages. |
| `customer.email_domain` | String, `eq` | Uses the normalized bare domain. IDNA normalization and case folding preserve exact equality. A subdomain, suffix lookalike, or absent domain fails the comparison. |
| `customer.mailbox_ref` | String, `eq` | Uses a server-derived reference scoped to the merchant. A different mailbox on the same domain or absent mailbox evidence fails the comparison. |
| `order.amount_minor` | Nonnegative integer, `eq`, `ne`, `gt`, `gte`, `lt`, `lte` | Uses minor units. A missing amount fails the comparison. An observed zero is a valid amount. |

Merchant rules contain `scope`, `name`, `product_types`, `stages`, `conditions`, and `action`. Conditions contain one flat `AND` or `OR` group with one to five typed entries and no executable expressions. Supported actions are `BLOCK`, `REVIEW`, and `CHALLENGE`. If an applicable policy cannot be evaluated due to absent evidence, the Merchant Policy result can report `UNAVAILABLE` and identify the fields in `missingSignals`. Payment failure windows, network country, check-in timing, subscriptions, nested groups, and explicit subdomain operators are outside this contract.

A temporary exact-email rule uses `BLOCK`, one `customer.mailbox_ref` equality condition, and a `durationHours` value from 1 to 8,760 whole hours. The standalone command proposes the block for all supported products in the selected merchant's profile and shows those products before saving. The duration starts when the version becomes `ACTIVE`. The API reports its `expiresAt` timestamp and derives `EXPIRED` at the cutoff. New transactions no longer apply that version after expiry. An identical save retry preserves the existing expiration, and replaying a saved transaction returns the original decision. [Browser workflow](../simulation-runbook.md#temporary-exact-email-block)

```json
{
  "scope": "MERCHANT",
  "name": "Restricted flight email domain",
  "product_types": ["flight_ticket"],
  "stages": ["BOOKING"],
  "conditions": {
    "logic": "AND",
    "rules": [
      {
        "field": "customer.email_domain",
        "operator": "eq",
        "value": "skycannner.com"
      }
    ]
  },
  "action": "BLOCK"
}
```

Platform proposals use the existing stage-specific System Rules signal catalogue. Numeric fields support `eq`, `ne`, `gt`, `gte`, `lt`, and `lte`. The `payment_token_missing` boolean supports `eq` and `ne`. Platform drafts use `REVIEW` or `CHALLENGE` and join one to five conditions with AND. Get the current fields and their stages from `GET /api/v1/system/rules`. [Existing System Rules contract](system-rules.md)

## Rule lifecycle and isolation

A Merchant Policy save or edit validates the tenant's current capabilities, runs a bounded preview on recent stored evidence, checks for overlap with other active policies that have a different action, and appends a version in one transaction. A valid proposal that qualifies for automatic activation becomes `ACTIVE`. A potential conflict or a product-wide block saves the new version as `PENDING_APPROVAL` and records the reason. An earlier active version can still apply while the new version is pending. The conflict check is conservative and may flag policies with disjoint numeric ranges. Activation checks the proposal and current conflicts again. Disable records `DISABLED`. Rollback copies a previous proposal to a new version and checks it against current capabilities and conflicts. Updates require the latest `expectedVersion`. Each tenant can save up to 50 rules. The history retains the prior versions and previews.

The API checks the bearer credential before interpreting, reading or mutating tenant rules. The general-purpose composer rejects detected email addresses, IP addresses, card numbers, and credentials in its free-text instruction. Use bare email domains or synthetic signal descriptions for this mode. OpenAI receives the instruction and the permitted field catalogue for the scope. The temporary exact-email command runs locally. The browser keeps the full address and sends a digest for rule authoring and merchant scenario submission. The Go API derives a reference from that digest and a merchant-specific key for storage and exact comparison. This evidence stays within PayGuard. Backend composition logs record operation, scope, actor, outcome, timing, and provider or draft metadata. `OPENAI_API_KEY` stays in the backend environment.

Platform change revisions append the approved rule input, shared run preview, reason, actor, time, and source revision. The administrator submits the current draft version and the independent checker approves the pending revision. The checker can restore an earlier active revision or disable the rule by appending another version. New Simulation Lab runs pin the effective platform ruleset and matching rule revisions. Previously saved runs and replays retain their original result.

Policy matching runs in Go without a provider call. The Merchant Policy evaluator applies to the saved merchant scenario path in Mock Checkout and Simulation Lab. Approved Platform revisions apply to new generated Simulation Lab runs. General booking assessments and live payment authorization have their existing evaluation paths. Automatic scoring-profile discovery and promotion across all booking flows and production role governance are separate work.

## Acceptance checks

- Compose a supported English or Thai policy, review the readable proposal, and save it under the right scope. Check that an ambiguous, unavailable, or malformed proposal cannot be saved through the composer.
- Verify exact normalized domain matching and missing amount handling. Check that a rule written by one merchant has no effect on the other merchant or on stored transaction snapshots.
- Preview and save a 12-hour exact-email block. In both merchant scenario tools, check the target mailbox, another mailbox on the same domain, and the bare domain. Verify the block is limited to the selected merchant, expires 12 hours after activation, and preserves earlier decision snapshots on replay.
- Verify the three independent decision parts, a conflict held for review, version history, disable, and rollback. Check that a Merchant Policy rejection leaves confirmed fraud evidence untouched.
- Verify that a Platform Admin save produces a draft and that merchant and unauthenticated credentials cannot invoke admin-only writes. Verify that approval requires the independent checker, a new Simulation Lab run pins the active rule revision, and rollback preserves stored decisions.
