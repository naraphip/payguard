# Provider enrichment

The Go API returns evidence about an IP address, a card BIN, or a disposable-email domain for an authenticated demo merchant. Local master lookups are available by default. External IP and BIN providers are opt-in. Merchant Policy, System Rules, booking intake and Simulation Lab operate independently of enrichment. [API setup and contract](../simulation-runbook.md#provider-enrichment)

| Area | Acceptance criterion |
| --- | --- |
| Input | Require a valid demo merchant token and at least one of `ip`, `bin`, and `emailDomain`. Accept a literal IP, a BIN of 6–8 ASCII digits, and a domain without an email local part. Private, reserved and TEST-NET IPs return `UNKNOWN` without an IP provider request. |
| BIN master | Look up PostgreSQL first. A complete exact match satisfies the lookup locally. A prefix match, missing record, partial record, or unavailable master can use provider fallback when enabled. Report the match and dataset provenance in `bin.master`. |
| Canonical BIN | Use the same Go BIN schema and normalization for master import, provider adapters, and cached provider evidence. Normalize aliases and omit blank or placeholder values. Keep exact PostgreSQL fields and fill blanks from exact external evidence when available. Reject mismatched provider echoes and require an exact echo for 7–8 digit requests. Report field provenance in `bin.fieldSources`. A shorter prefix is reference evidence under `bin.master.reference`. |
| Email-domain master | Check the local snapshot for the domain and its parent domains. A successful lookup returns `email.data` with the normalized domain, a `listed` boolean, and the most specific matching domain and sources when listed. Return `UNKNOWN` if the master is unavailable. |
| External fallback | When enabled, try ipapi.is then proxycheck.io for IPs and BIN Scanner then CardBinCheck for BINs. Give each provider an 800 ms timeout. Fall back on timeouts, network failures, 429, 5xx, malformed results and a BIN 404. Stop on authentication or other client rejection. |
| Evidence | Use the same response envelope for IP, BIN, and email evidence. Return lookup status, provider when available, lookup time, optional provider data timestamp, and cache status. Known `false` and zero remain visible. Missing values stay absent. A BIN is `OK` when all four canonical fields are available, with partial evidence allowed under `UNKNOWN`. |
| Cache | Store normalized external successes in Redis under an HMAC key scoped to the merchant and lookup kind. Cache BINs for 24 hours and IP results for 5 minutes if any risk flag is true or risk score is at least 60, otherwise 15 minutes. Continue the lookup if the cache is unavailable. |

## Local master sources

- **BIN.** The Techbuddie Solutions snapshot at `fcc907c81cf02b12e396b0c12a8c16472b47619a` contains 458,051 distinct prefixes. The PayGuard snapshot normalizes fields and retains the 454,496 six-digit and 3,548 eight-digit rows. The seven five-digit rows fall outside the accepted format, leaving 458,044 rows for import. [Pinned upstream source](https://github.com/Techbuddie-Solutions/binlist-data/tree/fcc907c81cf02b12e396b0c12a8c16472b47619a), [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)
- **Bundled domains.** The normalized snapshots from Valid Email Checker and the non-strict `disposable/disposable-email-domains` list contain 231,582 unique domains together. The bundle retains both MIT license files and the Valid Email Checker third-party notices. [Pinned Valid Email Checker source](https://github.com/Validemailchecker/disposable-email-domains/tree/08a0de67bb0cce3e46c6843cba07d5776091e4f6), [Pinned disposable source](https://github.com/disposable/disposable-email-domains/tree/41c4cec232e520bcda8bc9afeb9f8c42e58213d9), [Third-party notices](../../backend/masterdata/data/THIRD_PARTY_NOTICES.validemailchecker.md)
- **Local domain overlay.** The pinned VegaStack and TempMailDetector feeds bring the local union to 244,118 unique domains. VegaStack asks users to check individual source licenses for the aggregated data, and TempMailDetector's source has no explicit license. The overlay is generated locally and kept out of the distributed snapshot. [Pinned VegaStack source](https://github.com/vegastack/disposable-emails-detector/tree/109659eaa86a1ac753dfd0813cac177688613fca), [Pinned TempMailDetector source](https://github.com/TempMailDetector/Temporary-Email-Domain-Blocklist/tree/11b6b72778bdc9543af1e09fd0602916396c7c08)

The [local verification record](../current-work.md#provider-enrichment) includes imported counts, canonical field completeness, and results for exact, partial, prefix, and no-match fixtures.

## Provider limits and terms

Published limits depend on the provider plan. PayGuard can use optional server-side keys for the two IP providers. The BIN adapters use the keyless endpoints.

| Provider | Published allowance | Source |
| --- | --- | --- |
| ipapi.is | 1,000 lookups per day with a free account. Anonymous lookups are limited to 30 per querying IP per UTC day and return reduced data. | [Developer documentation](https://ipapi.is/developers.html) |
| proxycheck.io | 1,000 queries per day on a registered free plan or 100 per day without a key. | [API documentation](https://proxycheck.io/api/), [Terms](https://proxycheck.io/terms/) |
| BIN Scanner | 60 requests per minute per querying IP. Commercial use remains unconfirmed. | [API documentation](https://binscan.live/api) |
| CardBinCheck | 100 requests per day and 30 per minute per querying IP on the keyless endpoint. | [API documentation](https://cardbincheck.com/bin-lookup-api), [OpenAPI](https://cardbincheck.com/openapi.json) |

Confirm the intended commercial use and plan with CardBinCheck before rollout. Its FAQ permits production within fair use, while the plan description presents free access for evaluation and small projects and lists a commercial licence on paid plans. [CardBinCheck API and FAQ](https://cardbincheck.com/bin-lookup-api), [CardBinCheck terms](https://cardbincheck.com/terms), [IPNOVA API terms](https://api.ipnova.com/terms)
