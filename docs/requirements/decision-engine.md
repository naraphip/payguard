# Decision Engine

The administrator's Decision Engine navigation groups **Platform Rules**, **Merchant AI Rules**, and **AI Configuration**. The opt-in loopback Platform Admin workspace uses its server-side credential for merchant and rule management. Merchant AI Rules provides one authoring experience for typed policies, including conditions Jev can evaluate at transaction time. Jev diagnostics use a fixed synthetic request. [AI Rule Management](ai-rule-management.md) · [Merchant Management](merchant-management.md) · [Local setup and API](../simulation-runbook.md#jev-ai-configuration)

| Surface | Owner and scope | Runtime |
| --- | --- | --- |
| Platform Rules | Platform administrators create, view, edit and delete drafts without approval history. Independent checkers approve or roll back an eligible global revision. | New batch Simulation Lab runs use approved versions. Merchant-selected transactions use eligible rules when stage and observed evidence permit. Account age is available as an optional input. Velocity and payment-token signals are available in applicable batch flows. |
| Merchant Business Policies | A merchant manages its own versioned policies. Platform Admin manages the selected merchant’s policies through the same editor under Merchant Details → AI Rule. | Mock Checkout and Simulation Lab evaluate active policies using the merchant’s declared products and supported evidence, including registered custom fields. The assessment records matches, unavailable signals, and any semantic Jev answers. |
| Existing booking review rules | Platform Admin and the authorized merchant edit or remove existing amount-based rules under AI Rule → Saved rules. These changes append revisions against the current ruleset version. | New tests use the active amount-based rules. Stored assessments retain their prior versions and metadata. The legacy Booking API uses its own merchant credential and rules. |
| AI Configuration | Platform administrator saves versioned Jev settings and can run a synthetic diagnostic. | The effective configuration selects mock or OpenRouter Jev for new merchant transactions. Legacy Booking API and batch simulation keep their existing mock paths. |
| Advanced Risk Configuration | Merchant Details displays the scoped scoring profile version and available signals. | The existing Booking Assessment workflow owns profile approval and assessment history. |

A selected merchant's old `?tab=jev` link opens **AI Rule** for that merchant. The old `/settings/jev-rules` route opens the unified merchant rule entry, with merchant selection in the automatic local preview. [Review rule contract and access](jev-rules.md)

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
