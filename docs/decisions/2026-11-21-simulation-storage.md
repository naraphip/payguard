# Simulation run storage and evidence

Status **Implemented for the local milestone**, 21 November 2026. See the [Simulation Lab requirements](../requirements/simulation-lab.md).

- Store run configuration, version identifiers and decisions together as an immutable PostgreSQL JSONB snapshot. Key it by merchant and run ID. The snapshot makes local replay independent of Redis expiry or later rule edits. Store labels separately under the QA schema and commit both in one transaction. A normalized production event store was considered, and is deferred until the broader event contract is agreed.
- Compute velocity in Redis sorted sets, partitioned by merchant, run, execution attempt, stage, entity kind and HMAC key. Score in event-time order and use a 60-second inclusive window with the preceding 59 seconds and the current second. Expire these transient keys after ten minutes. Isolating concurrent attempts keeps retries and incomplete attempts from polluting future runs.
- Use a deterministic typed Jev mock and a fixed exact-domain policy fixture. This lets the UI exercise timeout and action provenance while the source package's live timeout, analyst escalation and merchant controls remain open.
- QA treats platform `REVIEW`, `CHALLENGE` or `BLOCK` as flagged and `ALLOW` as clear. Timeouts are unscored, and merchant-only blocks do not count as platform positives. Undefined rates are returned as null.
