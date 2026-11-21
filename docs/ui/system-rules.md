# System Rules dashboard UI

Status **Implemented and verified for the local sandbox**. The dashboard uses the sidebar and table patterns from the [shadcn/ui dashboard blocks](https://ui.shadcn.com/blocks/dashboard).

1. Open **System Rules** from the [Simulation Lab](simulation-lab.md) or visit `/settings/system-rules`. The navigation links to both pages.
2. Search or filter the rule list. Active built-ins are read-only. Draft actions open a side sheet for creating or editing a name, description, stage, action, and conditions.
3. Review inline validation and save the draft. A stale version shows a conflict so the newer draft can be reloaded.
4. Choose a saved synthetic run for a preview. Review **Evaluated**, **Matched**, and **Skipped**, along with the matching event IDs. The preview uses the run's recorded features.

The navigation and editor remain usable on narrow screens. Form fields have labels, the sheet supports keyboard access, and loading, empty, and error states describe the next available action. When there are no saved runs, the operator can create one in Simulation Lab. See the [scope and acceptance criteria](../requirements/system-rules.md).
