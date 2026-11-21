# Jev Rules

Open **Jev Rules** from Settings at `/settings/jev-rules`. [Rule contract](../requirements/jev-rules.md) · [Runbook](../simulation-runbook.md#jev-rules)

1. Enter the merchant demo token and load the active rules. The page keeps the token in memory.
2. Choose an example prompt or paste structured rule JSON from an AI chat. Local prompts use a limited grammar such as “Review bookings when amount is at least 50000 minor units.” Use **Draft rule** to see the parsed conditions and action. Unsupported prompts return guidance in the conversation.
3. Select **Preview on saved assessments** to see the sample size, matches, and assessments missing required evidence. The draft is still local at this point.
4. Select **Push to Jev** to publish. The active rule list shows the updated version and the published conditions. A failed API request displays the error and leaves the draft available to retry.

The rule action goes to manual review. The active list shows the default threshold for the 60-second velocity rule and the named custom rules with their IDs for the entered merchant.
