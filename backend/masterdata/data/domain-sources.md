# Disposable email master sources

The 21 November 2026 snapshot applies DNS-name validation, ASCII lowercase normalization, deduplication and deterministic sorting. Each gzip row is a domain, one tab, and its comma-separated source IDs in lexical order. Lookups select the most specific matching domain on a DNS label boundary. A match records feed membership. A miss records only that the domain was absent from the loaded snapshot.

| Source ID | Pinned feed | Distribution | Valid distinct rows |
| --- | --- | --- | ---: |
| `validemailchecker` | [Valid Email Checker TXT](https://github.com/Validemailchecker/disposable-email-domains/blob/08a0de67bb0cce3e46c6843cba07d5776091e4f6/data/domains.txt) | Bundled under the [MIT license](LICENSE.validemailchecker.txt) with [upstream notices](THIRD_PARTY_NOTICES.validemailchecker.md) | 231,476 |
| `disposable` | [disposable/disposable-email-domains non-strict TXT](https://github.com/disposable/disposable-email-domains/blob/41c4cec232e520bcda8bc9afeb9f8c42e58213d9/domains.txt) | Bundled under the [MIT license](LICENSE.disposable.txt) | 108,824 |
| `vegastack` | [VegaStack CSV](https://github.com/vegastack/disposable-emails-detector/blob/109659eaa86a1ac753dfd0813cac177688613fca/outputs/disposable_email_domains.csv) | Local overlay. Upstream asks users to check underlying data licenses for its consolidated feed, which includes StopForumSpam toxic domains. [VegaStack source and license notes](https://github.com/vegastack/disposable-emails-detector/blob/109659eaa86a1ac753dfd0813cac177688613fca/README.md#-license) | 125,124 |
| `tempmaildetector` | [TempMailDetector TXT](https://github.com/TempMailDetector/Temporary-Email-Domain-Blocklist/blob/11b6b72778bdc9543af1e09fd0602916396c7c08/domains.txt) | Local overlay. The publisher offers a delayed public dump and its repository supplies no data redistribution license. [TempMailDetector repository](https://github.com/TempMailDetector/Temporary-Email-Domain-Blocklist/tree/11b6b72778bdc9543af1e09fd0602916396c7c08) | 20,913 |

The bundled snapshot contains 231,582 distinct domains. The local overlay contains 138,682 distinct domains and increases combined coverage to 244,118. The overlay is intended for internal use and stored outside the bundled data. Enable it with `PAYGUARD_EMAIL_MASTER_LOCAL` after generating it locally.

From the backend module, run `go run ./cmd/refresh-email-master` to reproduce the bundled snapshot. Add `-local` to create `masterdata/local/domains.txt.gz` as well. Update the pinned feed revisions and this source record when adopting a new snapshot.

Bundled gzip SHA-256 `a08c6951712cefde490e86a59242aa1274a773c3a7dfa477df388d29e83cd82b`.
