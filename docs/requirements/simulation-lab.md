# Simulation Lab requirements

Status **Implemented for local simulation**. Scope follows [FLOW_03 and ER_03](https://drive.google.com/drive/folders/1lKpQdDOkrqpF61vxN_wNN2HHo9NQjNnH), the bundled `SIMULATION_LAB_SPEC.md`, and the [development plan](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit) tab `15 SIMULATION LAB`. See the [implementation discussion](../discussions/2026-11-21-simulation-lab.md).

| Area | Acceptance criterion |
| --- | --- |
| Synthetic storefronts | Select flight/hotel or physical/digital goods with booking date or delivery type and quantity. Emit two stages for each account. |
| Scenario controls | Reproduce normal, fraud ring, and mixed traffic from the seed and configuration. Support 1–100 accounts, 1–60 orders/minute, rotating IPs, shared legitimate Wi-Fi, shared ring device/token, account age, and simulated retries. |
| Replay and ordering | Same seed, configuration, tenant, generator version and HMAC secret reproduce the event list and graph edges. An identical run ID and configuration return the saved result. Changed configuration under the same ID returns 409. Reversed input is ordered by event time and ID before scoring. |
| Risk and missingness | Redis supplies server-side distinct order counts per stage and entity. Go emits rule hits and typed mock Jev evidence, routes missing payment tokens to review or challenge, and returns `UNSUCCESSFUL` for mock timeouts. |
| Policy attribution | When enabled, `restricted.test` can block flight pre-payments through the demo merchant policy. Preserve platform assessment and action source. Shared IP alone triggers no block. |
| Entity evidence | HMAC-derived account, device, IP and token nodes use tenant, type and synthetic value. Edges derive from observed events. Show distinct IP nodes when the same ring device/token appears across rotating source IPs. |
| QA | Scoring sees events and derived signals only. Store synthetic labels and ring IDs in `simulation_qa.ground_truth`. Score payment-stage decisions separately after persisting the run. Report counts, precision, recall and FPR against synthetic labels. |

The demo has one local merchant. Production RBAC, live Jev and payment routing, rule activation, adaptive baselines, analyst HOLD, full ER normalization, multi-item carts and cluster scoring are future work. D01–D10 in the source package remain business decisions. The demo timeout and policy rules apply only to synthetic runs.
