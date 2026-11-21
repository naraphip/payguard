# Decision Engine

The administrator's Decision Engine navigation groups **Platform Rules**, **Merchant Rules**, and **AI Configuration**. Each surface keeps its existing owner and persistence model. The global and merchant rule workflows use the direct OpenAI composer for natural-language drafts. The Jev Diagnostics surface uses a separate provider and request contract. [AI Rule Management](ai-rule-management.md) · [Merchant Management](merchant-management.md) · [Local setup and API](../simulation-runbook.md#jev-ai-configuration)

| Surface | Owner and scope | Runtime |
| --- | --- | --- |
| Platform Rules | Platform administrators draft global fraud rules. Independent checkers approve or roll back an eligible revision. | New generated Simulation Lab runs use the approved platform versions. Saved decisions retain their original evidence. |
| Merchant Business Policies | A merchant manages its own versioned policies. The administrator inspects them under Merchant Details → Merchant Rules. | New merchant scenario transactions evaluate active policies deterministically and store a separate merchant decision. |
| Booking review rules | The Merchant Rules workspace also exposes the existing amount-based demo review rules for the selected merchant. The Booking API and merchant demo review rule endpoints continue to use `booking_jev_rules` and their existing authorization. | The mock booking and demo scenario paths use their applicable review rules. Saved assessments retain their pinned rule versions. |
| AI Configuration | Platform administrator saves versioned Jev settings and can run a synthetic diagnostic. | The effective configuration selects mock or OpenRouter Jev for new merchant scenario transactions. Legacy Booking API and batch simulation keep their existing mock paths. |
| Advanced Risk Configuration | Merchant Details displays the scoped scoring profile version and available signals. | The existing Booking Assessment workflow owns profile approval and assessment history. |

Legacy entry points remain navigable. A selected merchant's old `?tab=jev` link opens **Merchant Rules → Booking review rules** for that same merchant. Outside the automatic local preview, `/settings/jev-rules` opens `/settings/merchant-rules?tab=review` for the legacy Booking API workflow. The automated preview sends the old entry point to the Merchant Rules directory. [Review rule contract and access](jev-rules.md)

## Jev configuration

The Go API requires a Platform Admin bearer credential for reading, saving, and testing configuration. PostgreSQL appends a version only when settings change and stores the administrator role and timestamp. A stale `expectedVersion` returns HTTP 409. The current view includes `configured`, which means Mock is selected or the server has a syntactically valid OpenRouter key. Run a diagnostic to establish connectivity.

| Setting | Supported values |
| --- | --- |
| Provider | `mock` or `openrouter` |
| Primary and fallback model | `typesafe/jev-1.14` or `typesafe/jev-1.13`. A fallback is optional and must differ from the primary. |
| Rubric | `standard-v1` or `conservative-v1` |
| Deadline | `timeoutMs` from 250 through 10,000 ms for the whole test, including fallback |

Version `0` means there is no stored configuration. The environment then selects the provider through `PAYGUARD_AI_MODE`, which defaults to `mock`. The live environment default uses `OPENROUTER_MODEL_PRIMARY` or `typesafe/jev-1.14` with a five-second deadline and no fallback. A saved configuration takes precedence over those deployment defaults. A new merchant scenario transaction pins `jevConfigurationVersion` when it uses a saved version. Previously stored decisions and exact replays retain their original result and provider metadata.

Mock diagnostics return a deterministic synthetic result. OpenRouter diagnostics send a fixed synthetic fixture and request one constrained choice. A configured fallback can use the remaining time after a failed primary attempt. Authorization failures bypass fallback. The diagnostic reports `MOCKED`, `LIVE`, `TIMEOUT`, or `UNAVAILABLE`, the model and rubric used, and whether fallback ran. The request body is an empty JSON object. The OpenRouter credential is held in the Go process.

## Compatibility checks

- Verify global, merchant, and configuration navigation with a Platform Admin credential and confirm that a merchant credential cannot read, save, or test Jev configuration.
- Open a selected merchant's Merchant Rules and Advanced Risk Configuration. Follow existing Jev and profile links to their compatible destinations. Load and preview the same stored legacy rules without rewriting their versions.
- Save a changed configuration, test the configured provider with synthetic input, and verify reload and stale-version handling. Check the version on a new merchant scenario transaction and compare an exact replay of a previous result. Verify the legacy Booking API and batch simulation still use mock Jev.
