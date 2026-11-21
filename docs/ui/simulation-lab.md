# Simulation Lab UI

Status **Implemented for local simulation**. See the [requirements](../requirements/simulation-lab.md) and [storefront flow](https://drive.google.com/file/d/1O30WveamPeeYuu_UR3s8QbQQNJ_usX_u/view).

1. Choose **Travel** or **Online store** in the purple storefront. Product cards use custom photos to distinguish products. Choose a date or delivery option and quantity in **Checkout details**.
2. For the first order, optionally enter a synthetic name, email, and phone in **Contact**. Use an address under `.test`, such as `buyer@traveler.test`. For flight tickets, turn on **Block flights for restricted.test** to exercise the demo policy.
3. Open **Simulation settings** to choose normal, fraud or mixed traffic and change the seed, accounts, orders/minute, and edge cases.
4. Choose **Run simulation** in the inline **Trip summary** or **Your cart**. When the saved checkout's first payment-stage event has final action `ALLOW`, **Pay in test mode** opens the Omise-hosted card form. The same panel shows the outcome, including pending or unknown, for the one attempt saved with the run. [Sandbox setup and test steps](../simulation-runbook.md#omise-sandbox-checkout)
5. Inspect **Simulation results** below checkout. With contact details entered, the lab selects the first order's payment-stage event. Both stages show the submitted contact from browser memory and the **Event email domain** used for assessment. Evidence includes platform action, merchant policy, action source, mock Jev answer and status, rule hits, missingness, event-time counts, and entity links.
6. Score the saved run from the **Offline QA** panel. The report shows metrics and the confusion matrix. An undefined rate displays an em dash, and timeouts are included in the unscored count.

The page keeps the storefront and setup beside the inline summary on desktop, and stacks them on mobile. Results appear below both. Product controls expose pressed state, fields have associated labels, asynchronous errors are announced, and entity cards give a text alternative to the Cytoscape graph. The graph centers on the selected event's account and shows up to five peers per entity. The cards show full peer counts.
