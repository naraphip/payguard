# AI Rule Management

Platform Admin and Merchant use **AI Rule** to create and review typed proposals in Thai or English and manage saved rules. The selected merchant’s field catalogue controls what the editor offers. Existing amount-based booking review rules appear in **Saved rules**, where either role can edit or remove them for the authorized merchant. Platform Rules drafts have their own workspace. [Setup and API examples](../simulation-runbook.md#ai-rule-composer) · [Current verification](../current-work.md#unified-merchant-ai-rule)

## Administration and execution

| Component | Responsibility |
| --- | --- |
| Shared composer | Go handles supported explicit merchant prompts and JSON locally. When the general-purpose composer is enabled, Go can send the instruction and the scope's typed field catalogue to OpenAI for a `READY` proposal or `CLARIFICATION`. Go revalidates each proposal at save. Jev evaluates saved semantic conditions during transaction assessment independently of this setting. |
| Platform Rules | The demo administrator composes and saves a global `DRAFT` in the existing Platform Rules store. A saved shared Simulation Lab run supplies evidence for submission. A separate checker can approve a revision for new Simulation Lab runs. The built-in rules retain their existing version and behavior. |
| Merchant Policy | The merchant's own bearer credential supplies its tenant. Platform Admin selects a merchant and can manage its rules with the same editor. PostgreSQL stores immutable policy versions. Go evaluates active policies in both merchant test tools and records matches and versions. |
| Jev | Mock or OpenRouter Jev evaluates eligible semantic conditions and supplies the separate amount-based risk advisory for merchant transactions. The administrator can test the effective settings with a fixed synthetic fixture in [AI Configuration](decision-engine.md#jev-configuration). |
| Decision resolver | The resolver records the Platform, Merchant Policy, Jev and final outputs. The highest precedence belongs to a platform `BLOCK`, then a merchant `BLOCK`. Other actions rank `REVIEW`, `CHALLENGE`, `UNRESOLVED` or `UNSUCCESSFUL`, and `ALLOW`. The platform wins a tie. Local decisions win a tie with Jev. A merchant policy match records a business decision. Confirmed fraud uses the existing outcome path. |

## Merchant business context and fields

The profile reads the existing merchant record, its saved setup, optional `policyIntegration` metadata from registration, and up to 200 recent transactions belonging to that merchant. Product availability comes from the declared catalogue or a recorded product. Amount availability comes from a declared booking amount signal or an observed amount. Domain availability comes from declared domain support or an observed normalized domain. The composer exposes missing capabilities so the operator can adjust the policy. The ordinary rule-creation flow uses the saved profile directly. The profile can expose a legacy scoring profile marker for existing integrations. The existing Booking Assessment profile workflow retains its own approvals and historical versions.

| Field | Type and operators | Availability and missing data |
| --- | --- | --- |
| `product.type` | Enum, `eq` | `flight_ticket` and `hotel_room` use `BOOKING`. `physical` and `digital` use `CHECKOUT`. `payment_gateway` uses `CHECKOUT` in the merchant panel in Simulation Lab. The proposal includes the corresponding products and stages. |
| `customer.email_domain` | String, `eq` | Uses the normalized bare domain. IDNA normalization and case folding preserve exact equality. A subdomain, suffix lookalike, or absent domain fails the comparison. |
| `customer.mailbox_ref` | String, `eq` | Uses a server-derived reference scoped to the merchant. A different mailbox on the same domain or absent mailbox evidence fails the comparison. |
| `order.amount_minor` | Nonnegative integer, `eq`, `ne`, `gt`, `gte`, `lt`, `lte` | Uses minor units. A missing amount fails the comparison. An observed zero is a valid amount. |
| `transaction.description` | Text, `eq`, `contains`, `semantic_match` | Optional description of the item or service. Submit it as a `customSignals` entry with this key. |
| Registered `custom_*` field | Text (`eq`, `contains`, `semantic_match`), country code or boolean (`eq`, `ne`), whole number (`eq`, `ne`, `gt`, `gte`, `lt`, `lte`) | The merchant sends the field under `customSignals`. Text comparisons ignore case and whitespace at the ends. `contains` matches a literal substring. Country values use uppercase ISO alpha-2 codes. False and zero are observed values. |

Merchant rules contain `scope`, `name`, `product_types`, `stages`, `conditions`, and `action`. Conditions contain one flat `AND` or `OR` group with one to five typed entries and no executable expressions. Supported actions are `BLOCK`, `REVIEW`, and `CHALLENGE`. If an applicable policy cannot be evaluated due to absent evidence, the Merchant Policy result can report `UNAVAILABLE` and identify the fields in `missingSignals`. Register as many as 16 non-identifying fields on the merchant record. The field key and type stay stable once a rule or transaction uses the field. Country fields rely on evidence supplied by the integration. Payment failure windows, automatic network-country enrichment, check-in timing, subscriptions, nested groups, and explicit subdomain operators remain outside this contract.

A temporary exact-email rule uses `BLOCK`, one `customer.mailbox_ref` equality condition, and one duration. The preview request accepts `durationMinutes` from 1 to 525,600 or `durationHours` from 1 to 8,760. Both fields take whole numbers. Whole-hour durations expressed in minutes produce the corresponding hour-based proposal. The standalone command proposes the block for all supported products in the selected merchant's profile and shows those products before saving. The duration starts when the new version becomes `ACTIVE`. The API reports its `expiresAt` timestamp and derives `EXPIRED` at the cutoff. New transactions no longer apply that version after expiry. An identical create retry preserves the existing expiration, and replaying a saved transaction returns the original decision. [Browser workflow](../simulation-runbook.md#temporary-exact-email-block)

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

## Conditions checked by Jev

Use `semantic_match` on `transaction.description` or a registered custom text field when the meaning matters. Use the local `contains` operator for a literal substring and `eq` for exact values. For a country test, register `custom_ip_country` and supply the country code in the transaction. The preview labels each condition **Checked by PayGuard** or **Checked by Jev**. Checkout and Simulation Lab offer the same available inputs.

This rule blocks physical goods checkout when the supplied country is India and the transaction description means admission to Hong Kong Disneyland.

```json
{
  "scope": "MERCHANT",
  "name": "Hong Kong Disneyland admissions from India",
  "product_types": ["physical"],
  "stages": ["CHECKOUT"],
  "conditions": {
    "logic": "AND",
    "rules": [
      {"field": "custom_ip_country", "operator": "eq", "value": "IN"},
      {"field": "transaction.description", "operator": "semantic_match", "value": "admission to Hong Kong Disneyland"}
    ]
  },
  "action": "BLOCK"
}
```

Go evaluates eligible local conditions first. A local false in an AND group or a local true in an OR group settles that group without a semantic request. A conclusive Platform or Merchant block also skips the semantic request. If a semantic comparison is needed and its text is available, Go sends only the bounded observed text and saved criterion for each needed condition, with an opaque question key. Each value is limited to 120 characters. Detected email addresses, raw IP addresses, mailbox references, card data and credentials stay out of the request. In Mock mode, Jev returns `MATCH` for case-insensitive equality after trimming. Other comparisons return `UNKNOWN`. In Live mode, Go validates the typed Jev answers. The separate amount-based advisory still uses its own evidence and settings. [Jev configuration](decision-engine.md#jev-configuration)

| Answer | Effect |
| --- | --- |
| `MATCH` | The condition is true. PayGuard applies the saved action if the full rule matches. |
| `NO_MATCH` | The condition is false. PayGuard resolves the AND or OR group with the other conditions. |
| `UNKNOWN` | A timeout, unavailable or invalid provider response, missing safe evidence, or an uncertain answer can leave the group unresolved. An unresolved applicable group sends the transaction to **Review (unresolved)** under **Needs decision**, unless a conclusive block applies. |

Saved transactions pin the rule ID, version, zero-based condition index, field, answer and provider status in `policyDecision.merchantPolicyDecision.jevConditions`, with the model and provider decision ID when available. Missing safe evidence records `NOT_RUN` and `UNKNOWN`. The saved Jev settings version is recorded when available. Replay returns the stored decision and evidence. The rule preview uses recent stored evidence to count cases that would need Jev at transaction time. [Live test walkthrough](../simulation-runbook.md#test-a-condition-with-live-jev)

## Rule lifecycle and isolation

A Merchant Policy save or edit validates the tenant's current capabilities, runs a bounded preview on recent stored evidence, checks for overlap with other active policies that have a different action, and appends a version in one transaction. A valid proposal that qualifies for automatic activation becomes `ACTIVE`. A potential conflict or a product-wide block saves the new version as `PENDING_APPROVAL` and records the reason. An earlier active version can still apply while the new version is pending. The conflict check is conservative and may flag policies with disjoint numeric ranges. Activation checks the proposal and current conflicts again. Disable records `DISABLED`. Delete removes the rule from the current list and evaluation by appending a version, with the previous evidence retained for history. Rollback copies a previous proposal to a new version and checks it against current capabilities and conflicts. Updates require the latest `expectedVersion`. Each tenant can save up to 50 rules. The history retains the prior versions and previews.

The API checks the bearer credential before interpreting, reading or mutating tenant rules. The general-purpose composer rejects detected email addresses, IP addresses, card numbers, and credentials in its free-text instruction. Use bare email domains or synthetic signal descriptions for this mode. OpenAI receives the instruction and the permitted field catalogue for the scope. The temporary exact-email command runs locally. The browser keeps the full address and sends a digest for rule authoring and merchant transaction submission. The Go API derives a reference from that digest and a merchant-specific key for storage and exact comparison. This evidence stays within PayGuard. Backend composition logs record operation, scope, actor, outcome, timing, and provider or draft metadata. `OPENAI_API_KEY` stays in the backend environment.

Platform change revisions append the approved rule input, shared run preview, reason, actor, time, and source revision. The administrator submits the current draft version and the independent checker approves the pending revision. The checker can restore an earlier active revision or disable the rule by appending another version. New Simulation Lab runs pin the effective platform ruleset and matching rule revisions. Previously saved runs and replays retain their original result.

Go evaluates native policy conditions locally and calls the configured Jev provider only for semantic comparisons that still need a result. The Merchant Policy evaluator applies to the merchant-selected path in Mock Checkout and Simulation Lab. Approved Platform revisions apply to new generated Simulation Lab runs and to merchant-selected tests when the stage and signals are available. General booking assessments and live payment authorization have their existing evaluation paths. Automatic scoring-profile discovery and promotion across all booking flows and production role governance are separate work.

## Acceptance checks

- Compose a supported English or Thai policy, review the readable proposal, and save it under the right scope. Check that an ambiguous, unavailable, or malformed proposal cannot be saved through the composer.
- Verify exact normalized domain matching and missing amount handling. Check that a rule written by one merchant has no effect on the other merchant or on stored transaction snapshots.
- Preview and save a 12-hour exact-email block. In both merchant test tools, check the target mailbox, another mailbox on the same domain, and the bare domain. Verify the block is limited to the selected merchant, expires 12 hours after activation, and preserves earlier decision snapshots on replay.
- Verify `MATCH`, `NO_MATCH`, and unresolved results in the merchant's ledger from both tools. Test local short-circuiting, missing evidence, provider timeout, and exact replay after a rule version changes.
- Verify the recorded Platform, Merchant Policy, Jev and final outputs, a conflict held for review, version history, disable, and rollback. Check that a Merchant Policy rejection leaves confirmed fraud evidence untouched.
- Verify that a Platform Admin save produces a draft and that merchant and unauthenticated credentials cannot invoke admin-only writes. Verify that approval requires the independent checker, a new Simulation Lab run pins the active rule revision, and rollback preserves stored decisions.
