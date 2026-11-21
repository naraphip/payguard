# Decision Engine

The administrator's Decision Engine navigation groups **Platform Rules** and **AI Configuration**. To edit a shop's rules as Platform Admin, open **Merchants**, select the shop, and open **AI Rule**. Merchants use **Merchant AI Rules** in their own workspace. The opt-in loopback Platform Admin workspace uses its server-side credential for merchant and rule management. The shared editor supports typed policies, including conditions Jev can evaluate at transaction time. Jev diagnostics use a fixed synthetic request. [AI Rule Management](ai-rule-management.md) · [Merchant Management](merchant-management.md) · [Local setup and API](../simulation-runbook.md#jev-ai-configuration)

| Surface | Owner and scope | Runtime |
| --- | --- | --- |
| Platform Rules | Platform administrators create, view, edit and delete drafts without approval history. Independent checkers approve or roll back an eligible global revision. | New batch Simulation Lab runs use approved versions. Merchant-selected transactions use eligible rules when stage and observed evidence permit. Account age is available as an optional input. Velocity and payment-token signals are available in applicable batch flows. |
| Merchant Business Policies | A merchant manages its own versioned policies. Platform Admin manages the selected merchant’s policies through the same editor under Merchant Details → AI Rule. | Mock Checkout and Simulation Lab evaluate active policies using the merchant’s declared products and supported evidence, including registered custom fields. The assessment records matches, unavailable signals, and any semantic Jev answers. |
| Existing booking review rules | Platform Admin and the authorized merchant edit or remove existing amount-based rules under AI Rule → Saved rules. These changes append revisions against the current ruleset version. | New tests use the active amount-based rules. Stored assessments retain their prior versions and metadata. The legacy Booking API uses its own merchant credential and rules. |
| AI Configuration | Platform administrator saves versioned Jev settings and can run a synthetic diagnostic. | The effective configuration selects mock or OpenRouter Jev for new merchant transactions. Legacy Booking API and batch simulation keep their existing mock paths. |
| Advanced Risk Configuration | Merchant Details displays the scoped scoring profile version and available signals. | The existing Booking Assessment workflow owns profile approval and assessment history. |

A selected merchant's old `?tab=jev` link opens **AI Rule** for that merchant. Legacy `/settings/merchant-rules` and `/settings/jev-rules` links keep access to the unified rules flow. [Review rule contract and access](jev-rules.md)

## Transaction status and manual review

The merchant ledger, assessment results, and dashboard use **Approved**, **Rejected**, and **Review**. Automated Allow or Low risk appears as **Approved**. Block or High risk appears as **Rejected**. Review, Unresolved, and Challenge appear as **Review**. Platform Admin can open any **Review** transaction and select **Approve** or **Reject**. The recorded manual decision determines its new status and displays **Manually approved** or **Manually rejected**, with the actor and decision time. Merchants can read the decision in their own ledger.

The single **Status** filter accepts `APPROVED`, `REJECTED`, or `REVIEW` in the API's `status` query parameter. Older API clients can still pass `outcome` or `reviewState`. Original action, risk result, Jev answers, and the Risk Score remain available as saved decision evidence. [Local transaction walkthrough](../simulation-runbook.md#merchant-management)

## Merchant transaction Risk Score

Merchant-selected Mock Checkout and Simulation Lab save a 0–100 advisory Risk Score with the transaction. Expand **Risk score** to see its factors and formula version. The final action still follows the existing rules and Jev decision. The transaction API exposes `policyDecision.riskScore` with `version`, `value` (integer or `null`), `partial`, and `factors` containing `code` and `points`. An exact replay or a manual review disposition preserves the saved score. Transactions created before scoring display **Not recorded**. [Score and email-change walkthrough](../simulation-runbook.md#test-the-risk-score-and-email-changes)

| Evidence | Contribution |
| --- | --- |
| Starting score | 20 points |
| Observed account age of 7 days or less | +10 points |
| Email domain listed in the local disposable-email master | +15 points |
| Customer changes email after a disposable-email booking | +15 points with `merchant-risk-v2` |
| Matched Platform or Merchant rule | Minimum 40 for **Challenge**, 55 for **Review**, and 85 for **Block** |
| Matched amount-based review rule or a **Review** answer from live or mock Jev | Minimum 55 |
| **Low risk** answer from live or mock Jev | 0 points |

Applicable rules and Jev increase the score only by the points needed to reach their respective minimums. Jev confidence is displayed separately. Missing or unresolved evidence contributes 0 points and marks the score as **Partial evidence**. A transaction with no usable scored evidence displays **Insufficient evidence**.

The optional `customerId` is an opaque, case-sensitive merchant identifier. Use 1–128 ASCII letters, digits, dots, underscores, colons, or hyphens, starting with a letter or digit. PayGuard stores a merchant-keyed HMAC reference and keeps the identifier out of Jev requests and transaction responses. It checks the latest earlier transaction with email evidence for the same merchant and Customer ID. If the saved score classified that email as disposable, differing mailbox references or differing known domains can add the transition factor. The factor applies to the change itself. Repeated bookings using the new address use the new address as the reference point. Account association comes from the merchant's Customer ID. Confirming who owns an email address requires separate verification.

## Guest booking comparison

For a new merchant-selected transaction with a full email, PayGuard compares the latest saved transaction that uses the same merchant-scoped mailbox reference. The lookup spans Mock Checkout and Simulation Lab for guests and for bookings with a Customer ID. A first booking, a bare domain, a different mailbox, or a different merchant has no matching prior booking for this feature. A guest who changes addresses needs a separate stable reference to associate those addresses. [Guest booking walkthrough](../simulation-runbook.md#compare-guest-bookings-with-jev)

When prior evidence exists, the existing live Jev advisory requests `booking_risk` (`LOW_RISK` or `REVIEW`) and `booking_similarity` (`SIMILAR`, `DIFFERENT`, or `INSUFFICIENT`) together. The comparison state contains prior and current product categories and nullable amounts, prior manual disposition, a description comparison that ignores case and surrounding whitespace, and counts of shared registered custom fields that match or changed. Email addresses and digests, merchant and customer IDs, raw descriptions, and custom field keys and values stay outside the outbound comparison state. The API saves the comparison and its Jev result as `policyDecision.bookingComparison`.

An email match supplies comparison context. Booking similarity and risk are separate Jev answers. A Jev **Review** still has a score minimum of 55 and uses the existing manual review workflow. Mock Jev recommends **Review** from a prior manual rejection combined with a changed product or amount. **Previous booking comparison** shows the evidence and Jev status in both result views and the saved transaction.

## Jev configuration

The Go API requires a Platform Admin bearer credential for reading, saving, and testing configuration. PostgreSQL appends a version only when settings change and stores the administrator role and timestamp. A stale `expectedVersion` returns HTTP 409. The current view includes `configured`, which means Mock is selected or the server has a syntactically valid OpenRouter key. Run a diagnostic to establish connectivity.

| Setting | Supported values |
| --- | --- |
| Provider | `mock` or `openrouter` |
| Primary and fallback model | `typesafe/jev-1.14` or `typesafe/jev-1.13`. A fallback is optional and must differ from the primary. |
| Rubric | `standard-v1` or `conservative-v1` |
| Deadline | `timeoutMs` from 250 through 10,000 ms for the whole test, including fallback |

Version `0` means there is no stored configuration. The environment then selects the provider through `PAYGUARD_AI_MODE`, which defaults to `mock`. The live environment default uses `OPENROUTER_MODEL_PRIMARY` or `typesafe/jev-1.14` with a five-second deadline and no fallback. A saved configuration takes precedence over those deployment defaults. A new merchant transaction pins `jevConfigurationVersion` when it uses a saved version. Previously stored decisions and exact replays retain their original result and provider metadata.

Mock diagnostics return a deterministic synthetic result. OpenRouter diagnostics send a fixed synthetic fixture and request one constrained choice. A configured fallback can use the remaining time after a failed primary attempt. Authorization failures bypass fallback. The diagnostic reports `MOCKED`, `LIVE`, `TIMEOUT`, or `UNAVAILABLE`, the model and rubric used, and whether fallback ran. The request body is an empty JSON object. The OpenRouter credential is held in the Go process.

## Compatibility checks

- Verify global, merchant, and configuration navigation with a Platform Admin credential and confirm that a merchant credential cannot read, save, or test Jev configuration.
- Open a selected merchant's AI Rule and Advanced Risk Configuration. Follow the old Jev route into AI Rule and load the same stored amount-based rules under Saved rules.
- Save a changed configuration, test the configured provider with synthetic input, and verify reload and stale-version handling. Check the version on a new merchant transaction and compare an exact replay of a previous result. Verify the legacy Booking API and batch simulation still use mock Jev.
