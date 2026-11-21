# Merchant Scenarios and Rules

Status **Chrome and live API checks verified merchant transaction isolation in the merged preview.** Platform administrators manage merchant setup, scenarios and rules, and see transactions across active merchants. A merchant sees only its own transactions and saved results. This requirement extends the browser-local [Merchant Setup](merchant-setup.md) and the existing [Risk Operations workflow](../simulation-runbook.md#local-evaluation). Connecting a live Jev service is deferred until after the flow is validated. [Mock preview runbook](../simulation-runbook.md#merchant-scenarios-and-mock-jev) · [Verification](../current-work.md#merchant-scenarios-and-mocked-jev)

The preview uses a structured form with one booking amount threshold per scenario. Saving the reviewed preview makes that revision available for tests. It includes a single merchant-scenario assessment panel in Simulation Lab and direct clear or reject actions for mock transactions. AI-generated drafts, broader scenario inputs, configurable multi-rule sets, and the full analyst HOLD lifecycle in the new merchant workspace remain requirements beyond this slice.

## Administrator and test flow

1. Open **Merchants** to see saved merchants. Create a merchant or open an existing merchant to update its details.
2. Define one or more named risk scenarios for the merchant. For each scenario, specify its business model, assessment stage, available signals, and the behaviors or examples its rules should address.
3. Use AI to generate a draft rule set for the selected scenario. Review, configure, and validate the rules, then designate the version to use for testing that scenario.
4. Open **Mock Checkout** or **Simulation Lab**, select an authorized merchant and one of its scenarios, and submit a test to Jev. The result shows Jev’s decision and reasons and identifies the merchant, scenario, and versions used.
5. Open **Transactions** to inspect saved results for any active merchant and record a disposition when a transaction needs a human decision. Dashboard totals and recent activity come from stored records, with an empty state before any transactions are saved.

## Merchant flow

1. Open the merchant workspace with the credential issued for that merchant. **Transactions** is its only menu destination.
2. Use the read-only list to see the merchant’s transactions across all outcomes. Use **Needs decision** to focus on transactions awaiting manual review.
3. Open a transaction to see Jev’s saved decision, reasons, rule hits, and the rule-set version used. The detail also shows the current review state and any recorded disposition.

## Functional requirements

| Area | Requirement |
| --- | --- |
| Merchant directory and access | Only administrators can list all merchants, create, edit, or delete merchant records, and view rules across merchants. Require a display name and assign a stable merchant identifier. Saving changes to one merchant leaves other merchant records intact. Merchants and their configuration remain available after reloading or reopening the app. |
| Merchant setup | Store the merchant’s business model, assessment stage, and declared signals on the merchant record. The preview supports OTA or e-commerce, Booking, a required booking timestamp, and optional booking amount availability. Older name-only merchants remain usable and show **Setup incomplete** until their details are saved. |
| Merchant removal | Require explicit administrator confirmation. Remove the merchant from the directory and test pickers, and deactivate its credential and normal workspace access. Retain the merchant’s historical records. |
| Merchant risk scenarios | A merchant can have multiple independently named, selectable scenarios. Save each scenario under its merchant with its own stable identifier and versions. Capture business model, assessment stage, available signals, and a description or examples of the behavior the merchant wants to detect. A merchant risk scenario guides rule generation. A Simulation Lab fixture such as **Normal**, **Mixed**, or **Fraud** selects the synthetic test data. |
| Rule generation | Use AI to generate a draft from the saved scenario. Explain which scenario inputs each generated rule uses and show when there is insufficient information to propose a rule. Surface unknown or unavailable signals and unsupported stage or operator combinations. The draft requires human review and validation against supported signals and operations before it can be used. |
| Rule configuration | Give each scenario its own versioned rule set. Show its generated rules and allow authorized users to edit their conditions and actions within the supported rule model. Validate changes before the rules can be used. Associate every usable rule-set version with its merchant and scenario revision. |
| Merchant and scenario selection in both tools | Mock Checkout and Simulation Lab each let users choose an authorized merchant and one of that merchant’s scenarios before submitting a test. Supply the transaction signals and that scenario’s configured rule-set version to Jev. Use the same rule-set version in both tools while the scenario configuration is unchanged. Existing tool-specific checkout and fixture options can still supply test inputs. |
| Jev decision | Invoke the mocked Jev decision contract for every new automated decision in either test tool during flow validation. The mock uses the transaction’s available signals and the selected scenario’s rule-set version to return an outcome, decision reasons, and matched rules. Save the result with its merchant, scenario and rule-set versions, and mock Jev version where available. An idempotent replay returns the saved result without invoking a new decision. Record when the mock simulates a timeout or other failure. |
| Merchant rules management | Show the selected merchant’s scenarios, rules, and versions to Platform Admin. Jev Rules and Platform Rules management sit inside the merchant workspace. A merchant credential grants access to the merchant’s own transaction list and detail. |
| Merchant transaction list | Show the merchant’s transactions submitted for risk assessment, including transactions awaiting assessment, transactions with a saved automated result, and transactions that need a human decision. Provide a **Needs decision** filter. A resolved review remains available in the full list. |
| Decision and result detail | Show the transaction identifier, saved Jev outcome or failure status, decision reasons, matched rules, observed evidence, and pinned scenario and rule-set versions. Show current review state and recorded analyst disposition separately from the original Jev assessment. |
| Transaction management | A Platform Admin can record a disposition on a pending transaction. Update the case status, retain who decided and when, and preserve the original assessment and evidence. A resolved case cannot receive a second decision. |
| Missing or unsupported configuration | Show an actionable message when a selected merchant has no usable rule set or a tool cannot evaluate it for the selected stage. If a rule depends on a signal such as booking amount, the test must supply it at the relevant stage or explicitly report it as unavailable. A missing value must remain distinct from an explicit zero. |
| Isolation and evidence | Scope configuration, generation, evaluation, and results to the authorized merchant and selected scenario where applicable. Apply the merchant boundary to rule and transaction lists, filters, direct transaction access, and review actions. A test must never fall back to another merchant’s or scenario’s rules. Retain the merchant, scenario identity and version, rule-set version, and rule results on a completed test so later edits cannot misattribute it. |

## Proposed first release defaults

- Allow one rule-set version per scenario to be designated for testing at a time. Keep prior versions available in completed test results.
- Generate a reviewable AI draft when the user requests it. Require explicit designation of a reviewed and validated revision before it affects subsequent tests.
- When a scenario changes, keep the previously selected version traceable and identify it as based on the earlier scenario. Allow users to regenerate and review a replacement.
- Start with the business models, stages, and signals supported by the local demo where both tools can evaluate the selected scenario. Keep existing demo merchants and synthetic test workflows usable. An operator can pick an authorized demo merchant without entering that merchant’s token again in each tool.
- The merchant workspace records the administrator’s direct disposition with the acting user and timestamp. The standalone operations workflow retains its separate HOLD lifecycle.
- In the sandbox, if Jev times out or fails before returning a decision, PayGuard records the failure, marks the assessment **Unresolved**, and routes the transaction to manual review. The workflow records outcomes and review activity without taking a payment action.

## Mock Jev result examples

These examples use the same illustrative booking amount of **50,000 minor units** for Demo Merchant A. Decision labels and rule details are examples for validating the flow. In both tools, the result identifies the merchant, scenario revision, rule-set version, and that Jev is mocked. The queue column describes the expected merchant transaction state for a checkout.

| Merchant and scenario | Rules used | Mock Jev result and evidence | Merchant transaction state |
| --- | --- | --- | --- |
| Demo Merchant A · Everyday bookings v1 | Everyday rules v3 | **Low risk**. The booking amount is below the configured review threshold of 100,000 minor units. Evidence shows the observed amount and no matching review rule. | Appears in the full transaction list with its saved result. |
| Demo Merchant A · High-value bookings v1 | High-value rules v2 | **Review**. The amount meets the 50,000-minor-unit review threshold. Evidence shows the observed amount and the matched amount rule. | Appears in **Needs decision** with the saved result available in its detail. |
| Demo Merchant A · High-value bookings v1 | High-value rules v2 | **Timeout simulated**. Mock Jev returns no decision or matched-rule evidence. PayGuard records the timeout and marks the assessment **Unresolved**. | Appears in **Needs decision**. A later reviewer disposition is shown separately from the saved timeout. |

## Acceptance criteria

1. Create merchants A and B with distinct setup values. Reload the list, then change A’s name and setup. A retains its identifier and returns the updated setup after reload. B retains its previous details.
2. Save different scenarios for A and B. Generate and configure rules for A, then open B. B displays its own scenario and rules only.
3. Attempt to generate or save a rule using an unknown signal or a signal unavailable at the selected stage. The app identifies the invalid input and the rule cannot be selected for testing.
4. Configure rule sets for A and B that produce different decisions or matches for a supported test input. Run the equivalent input through Jev for each merchant in **Mock Checkout** and **Simulation Lab**. Both tools show Jev’s outcome, reasons, and rule hits for the selected merchant, along with the rule-set version actually used.
5. Switch from A to B while a test for A is running, then repeat when switching between two scenarios for A. When the first result arrives, it retains the original merchant, scenario, and versions. A later submission uses the new selection.
6. Save a new scenario or rule-set revision for A. Open a completed test from before the update. Its recorded versions and rule hits remain the same.
7. Select a scenario with no usable or compatible rule set. The app identifies the setup needed and does not substitute another scenario’s or merchant’s rules. Run a compatible checkout or fixture with an omitted amount and with an explicit zero. The results distinguish the unavailable signal from the observed zero wherever the rule depends on amount.
8. Prepare a transaction awaiting assessment, a low-risk checkout, and a timeout checkout for A, plus a timeout checkout for B. In A’s workspace, verify that the full list contains all three A transactions, **Needs decision** shows only A’s unresolved case, and selecting that case displays its saved outcome, rule hits, evidence, pinned versions, and review state.
9. Resolve A’s case as an administrator. It leaves **Needs decision**, remains visible in the full list, and displays the recorded decision, actor, and time alongside the unchanged assessment. Attempting to record a different decision keeps the original disposition. A merchant user can read the case and sees a read-only view.
10. As an administrator, verify that the merchant directory lists A and B, merchant records can be edited, and both sets of rules and transaction lists are accessible. With merchant A access, check that the sidebar has only **Transactions** and A’s own list and details are available. Check that direct URLs for administrator pages, configured rules, or B’s list or transaction detail are restricted. Verify the API rejects a review action on A’s or B’s case with a merchant credential.
11. Simulate a Jev timeout or failure for A in both test tools. The result identifies A and its selected scenario, reports Jev’s failure and the unresolved outcome, and indicates manual review. The case remains scoped to A, and a subsequent analyst disposition leaves the original Jev result intact.
12. Define two independently named scenarios for A with different rules for the same supported input. Generate an AI draft for each, review and validate it, and designate a rule-set version for testing. Run both scenarios in Mock Checkout and Simulation Lab. Each tool identifies the selected scenario and uses its designated version. The scenarios produce the expected different decisions or rule hits.
13. For each tool, submit a new test and verify the mock Jev contract is invoked for the selected scenario. Replay that submission with the same idempotency key and verify the saved outcome and versions are returned without another invocation. A different new submission invokes the contract again.
14. Cancel deletion of a test merchant and verify it remains available. Then confirm deletion and verify it disappears from the directory and both test pickers after reload, its issued token no longer works, and another merchant is unaffected. Verify historical records remain stored.
15. Open Platform Admin Dashboard before submitting transactions and verify its empty state. Submit a test and confirm that the dashboard and ledger show the saved record and that the dashboard links to its detail.

## Open decisions

- Which inputs the AI generator accepts, which may use free text, and which provider or model will be used.
- How to connect live Jev, the final decision vocabulary, and the resolution policy for timeout or failure beyond the sandbox.
- How incoming transactions select a scenario outside the demo, and whether presets or shared templates are needed.
- Which production administrator permissions govern approving and activating rule versions or deciding transaction cases.
- How the existing browser-local setup and any existing demo merchant profiles should be imported or linked to the new directory.

Related requirements [Booking assessment](booking-assessment.md) · [Mock Checkout and lineage](checkout-and-lineage.md) · [Simulation Lab](simulation-lab.md) · [Jev Rules](jev-rules.md)
