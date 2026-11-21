# GitHub progress monitoring in Notion

**Status** — Workflow active on GitHub `main`. Manual execution verified. Scheduled execution verification is ongoing.

The team needs GitHub delivery evidence in Notion on a ten-minute polling schedule. The [PayGuard Progress Monitoring Plan](https://app.notion.com/p/41f4da4de98381cb82e6c26dca52613e?pvs=204) describes the scope. Milestones and approval status come from the [Drive action plan](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260117) and [decision register](https://docs.google.com/spreadsheets/d/1bW-Vv10o795JTBwESZnbeY9URWNRkXN3gFVWjCTXX6c/edit#gid=260116).

## Configuration

| Item | Setting |
| --- | --- |
| Repository | [pwatakul/payguard](https://github.com/pwatakul/payguard) |
| Notion destination | [PayGuard GitHub Activity](https://app.notion.com/p/41f4da4de98381dcac67fb442ef0c35b) |
| GitHub Actions repository variable | `NOTION_DATA_SOURCE_ID=41f4da4d-e983-8107-97e3-000bd445a27e` |
| GitHub Actions repository secret | `NOTION_TOKEN`. Grant the integration read and write access to the activity database. |
| Implementation paths | `.github/workflows/notion-progress.yml` and `scripts/sync_notion_progress.py` |
| Triggers | Manual dispatch and `3-59/10 * * * *` on the default branch. GitHub can delay or skip scheduled runs. [GitHub scheduling documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule) |

The local `.env` file is ignored by Git. The runner reads the token from the repository secret.

## Acceptance

| Area | Evidence required |
| --- | --- |
| GitHub activity | Available issues, pull requests, default-branch commits, and CI runs appear with links and current states. The monitoring workflow’s runs are excluded from activity. |
| Repeat runs | Stable keys identify each object. Repeating a fetch or retrying after partial writes updates existing rows without duplicates. |
| Sync health | A successful empty run advances Last Success. A failed run preserves it. The health row links to the workflow run. Treat a last success older than 30 minutes as stale. |
| Ownership | The sync writes activity metadata and health fields. Milestones and approvals stay with their owners. |
| Activation | Verify the first manual run, then check that two distinct scheduled runs advance Last Success in Notion. |

## Verified so far

The activity database was created with the agreed schema. The local token returned HTTP 200 for the integration identity and the monitoring plan page. Independent fixture and workflow YAML checks passed. A live local sync changed 18 activity records, confirming that the integration can write to the database. The GitHub Actions repository secret and variable are configured.

The workflow is published on GitHub `main` at `40341b5`. The [manual GitHub Actions run](https://github.com/pwatakul/payguard/actions/runs/44868685163) succeeded. The Notion health row showed Last Success at 11:59 Bangkok time on 21 November 2026. Verification of two distinct scheduled runs against that row is ongoing.
