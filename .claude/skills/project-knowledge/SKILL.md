---
name: project-knowledge
description: Read and maintain the project's Markdown second brain when resuming work, capturing discussions, updating requirements or UI designs, and recording delivery outcomes.
---

Start with `docs/home.md`. Read the project brief and follow only the links relevant to the current task.

- Capture substantive discussions as dated notes in `docs/discussions/`. Summarize context, outcomes, and open questions. Attribute decisions to their source when available.
- Keep current requirements in `docs/requirements/` and UI designs in `docs/ui/`. Give each substantial feature or screen its own note. Link to the originating discussion and related notes. Include testable acceptance criteria for requirements and interaction states, accessibility, and linked mockups for UI.
- Record significant decisions and their rationale in `docs/decisions/`. When a decision changes, retain the prior record and link to its replacement. Give each requirement or design a clear status such as Proposed, Accepted, or Superseded. Mark something Accepted only when the user or an existing accepted record establishes approval.
- Store UI image files alongside their UI notes or in `docs/ui/assets/` and link to them with relative Markdown paths. Keep original designs and supporting evidence linked from the appropriate index.
- Update `docs/current-work.md` at meaningful handoffs for active delivery work. Record the latest verified stage, links to acceptance criteria and designs, observed test or release outcomes, blockers, and the next action. Store each durable fact in its owning note and link to it elsewhere.
- Re-read the target notes before editing, preserve existing user edits, and preserve conflicting evidence as an open question. Leave credentials, private data, and raw conversation dumps out of the vault.
