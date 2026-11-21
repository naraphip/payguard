# Simulation Lab UI

Status **Implemented for local simulation**. See the [requirements](../requirements/simulation-lab.md) and [storefront flow](https://drive.google.com/file/d/1O30WveamPeeYuu_UR3s8QbQQNJ_usX_u/view).

1. Choose OTA or E-commerce, then a product and date or delivery, plus quantity.
2. Choose normal, fraud or mixed traffic. Set seed, accounts, orders/minute, and optional edge cases.
3. Run the scenario and select an event. The evidence panel shows platform action, merchant policy, final action source, mock Jev answer/status, rule hits, missingness, event-time counts and entity links.
4. Score the saved run from the Offline QA panel. The report shows metrics and the confusion matrix. An undefined rate displays an em dash, and timeouts are included in the unscored count.

The page keeps scenario setup beside results on desktop and stacks panels on mobile. Product controls expose pressed state, fields have associated labels, asynchronous errors are announced, and entity cards give a text alternative to the Cytoscape graph. The graph centers on the selected event's account and shows up to five peers per entity. The cards show full peer counts.
