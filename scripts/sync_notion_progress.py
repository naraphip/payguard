"""Mirror GitHub activity into a dedicated Notion data source."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
import json
import os
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlencode
from urllib.request import HTTPRedirectHandler, Request, build_opener
from uuid import UUID


INITIAL_HISTORY = timedelta(days=7)
OVERLAP = timedelta(hours=1)
SYNC_WORKFLOW = ".github/workflows/notion-progress.yml"
PAGE_SIZE = 100


class SyncError(Exception):
    """An actionable error that is safe to print in a workflow log."""


class NoRedirects(HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, new_url):
        # The request carries an integration credential scoped to a single API.
        return None


class Api:
    def __init__(self, name, origin, token, headers):
        self.name = name
        self.origin = origin
        self.headers = {"Authorization": f"Bearer {token}", **headers}
        self.opener = build_opener(NoRedirects())

    def request(self, method, path, *, params=None, body=None, retry_safe=True):
        url = self.origin + path
        if params:
            url += "?" + urlencode(params)
        data = json.dumps(body).encode("utf-8") if body is not None else None
        request = Request(url, data=data, headers=self.headers, method=method)

        for attempt in range(3):
            try:
                with self.opener.open(request, timeout=20) as response:
                    return json.load(response)
            except HTTPError as error:
                retry_after = error.headers.get("Retry-After", "")
                limited = error.code == 429 or (error.code == 403 and retry_after)
                transient = retry_safe and error.code in (500, 502, 503, 504)
                if attempt == 2 or not (limited or transient):
                    raise SyncError(f"{self.name} {method} returned HTTP {error.code}") from None
                delay = int(retry_after) if retry_after.isdecimal() else 2 ** attempt
                if delay > 30:
                    raise SyncError(f"{self.name} requested a retry after {delay} seconds") from None
                time.sleep(delay)
            except (URLError, TimeoutError):
                if attempt == 2 or not retry_safe:
                    raise SyncError(f"{self.name} {method} failed to connect or timed out") from None
                time.sleep(2 ** attempt)
            except (json.JSONDecodeError, UnicodeDecodeError):
                raise SyncError(f"{self.name} returned an invalid JSON response") from None


def moment(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def timestamp(value):
    return value.astimezone(timezone.utc).isoformat(timespec="milliseconds")


def github_timestamp(value):
    return value.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


@dataclass(frozen=True)
class Entry:
    key: str
    name: str
    kind: str
    state: str
    updated: datetime | None
    url: str
    assignees: str = ""
    last_success: datetime | None = None


class GitHub:
    def __init__(self, api, repository):
        parts = repository.split("/")
        if len(parts) != 2 or not all(parts):
            raise SyncError("GITHUB_REPOSITORY must be an owner/repository pair")
        self.api = api
        self.path = "/repos/" + "/".join(quote(part, safe="") for part in parts)
        self.repository = repository
        info = self.api.request("GET", self.path)
        self.id = info["id"]
        self.default_branch = info["default_branch"]
        self.url = info["html_url"]

    def key(self, kind, item_id):
        return f"{self.id}:{kind}:{item_id}"

    def pages(self, endpoint, params, collection=None):
        page = 1
        while True:
            response = self.api.request(
                "GET", self.path + endpoint,
                params={**params, "per_page": PAGE_SIZE, "page": page},
            )
            if collection == "workflow_runs" and response["total_count"] > 1000:
                raise SyncError("GitHub returned more CI history than the filtered API supports")
            items = response[collection] if collection else response
            yield from items
            if len(items) < PAGE_SIZE or (collection and page * PAGE_SIZE >= response["total_count"]):
                return
            page += 1

    def issues(self, since):
        for item in self.pages("/issues", {"state": "all", "since": github_timestamp(since)}):
            if "pull_request" not in item:
                yield self.work_item_entry(item, "issue", "Issue", item["state"])

    def pull_requests(self, since):
        options = {"state": "all", "sort": "updated", "direction": "desc"}
        for item in self.pages("/pulls", options):
            if moment(item["updated_at"]) < since:
                break
            state = item["state"]
            if item["merged_at"]:
                state = "merged"
            elif state == "open" and item["draft"]:
                state = "draft"
            yield self.work_item_entry(item, "pr", "Pull request", state)

    def work_item_entry(self, item, key_kind, kind, state):
        return Entry(
            key=self.key(key_kind, item["id"]),
            name=f'#{item["number"]} {item["title"]}',
            kind=kind,
            state=state,
            updated=moment(item["updated_at"]),
            url=item["html_url"],
            assignees=", ".join(sorted(person["login"] for person in item["assignees"])),
        )

    def commits(self, since):
        options = {"sha": self.default_branch, "since": github_timestamp(since)}
        for item in self.pages("/commits", options):
            commit = item["commit"]
            sha = item["sha"]
            title = commit["message"].partition("\n")[0]
            yield Entry(
                key=self.key("commit", sha),
                name=f"{sha[:8]} {title}".rstrip(),
                kind="Commit",
                state=self.default_branch,
                updated=moment(commit["committer"]["date"]),
                url=item["html_url"],
            )

    def ci_runs(self, since, now):
        created_since = min(since, now - INITIAL_HISTORY)
        options = {"created": ">=" + github_timestamp(created_since)}
        for workflow in self.pages("/actions/workflows", {}, "workflows"):
            if workflow["path"] == SYNC_WORKFLOW:
                continue
            endpoint = f'/actions/workflows/{workflow["id"]}/runs'
            for item in self.pages(endpoint, options, "workflow_runs"):
                updated = moment(item["updated_at"])
                if updated < since:
                    continue
                name = item["name"] or "Workflow"
                yield Entry(
                    key=self.key("ci", item["id"]),
                    name=f'{name} #{item["run_number"]}',
                    kind="CI run",
                    state=item["conclusion"] or item["status"],
                    updated=updated,
                    url=item["html_url"],
                )


def rich_text(value):
    return [{"type": "text", "text": {"content": value[:1000]}}] if value else []


def date_property(value):
    return {"date": {"start": timestamp(value)} if value else None}


def entry_properties(entry):
    return {
        "Name": {"title": rich_text(entry.name)},
        "Key": {"rich_text": rich_text(entry.key)},
        "Kind": {"select": {"name": entry.kind}},
        "State": {"rich_text": rich_text(entry.state)},
        "Source Updated": date_property(entry.updated),
        "Assignees": {"rich_text": rich_text(entry.assignees)},
        "GitHub URL": {"url": entry.url},
        "Last Success": date_property(entry.last_success),
    }


def comparable_value(kind, value):
    if kind in ("title", "rich_text"):
        return "".join(part.get("plain_text", part.get("text", {}).get("content", ""))
                       for part in value)
    if kind == "select":
        return value["name"] if value else None
    if kind == "date":
        return moment(value["start"]) if value else None
    return value


def property_value(properties, name, kind):
    value = properties.get(name, {})
    if kind not in value:
        raise SyncError(f"Notion property {name} must have type {kind}")
    return comparable_value(kind, value[kind])


class Notion:
    def __init__(self, api, data_source_id):
        self.api = api
        try:
            self.data_source_id = str(UUID(data_source_id))
        except ValueError:
            raise SyncError("NOTION_DATA_SOURCE_ID must be a Notion data source UUID") from None

    def find_entry(self, key):
        response = self.api.request(
            "POST", f"/v1/data_sources/{self.data_source_id}/query",
            body={
                "filter": {"property": "Key", "rich_text": {"equals": key}},
                "page_size": 2,
            },
        )
        results = response["results"]
        if response["has_more"] or len(results) > 1:
            raise SyncError("The Notion data source contains duplicate GitHub activity keys")
        return results[0] if results else None

    def upsert(self, entry, existing):
        desired = entry_properties(entry)
        if existing is None:
            self.api.request(
                "POST", "/v1/pages",
                body={
                    "parent": {"type": "data_source_id", "data_source_id": self.data_source_id},
                    "properties": desired,
                },
                # An uncertain create can be discovered by its key on the next run.
                retry_safe=False,
            )
            return True

        current = existing["properties"]
        changes = {}
        for name, property_object in desired.items():
            kind, value = next(iter(property_object.items()))
            if property_value(current, name, kind) != comparable_value(kind, value):
                changes[name] = property_object
        if changes:
            self.api.request("PATCH", f'/v1/pages/{existing["id"]}', body={"properties": changes})
        return bool(changes)


def sync(github, notion, run_id, now=None):
    started = now or datetime.now(timezone.utc)
    health_key = github.key("health", "current")
    health = notion.find_entry(health_key)
    last_success = None
    newest = None
    if health:
        last_success = property_value(health["properties"], "Last Success", "date")
        newest = property_value(health["properties"], "Source Updated", "date")
    since = min(last_success, started) - OVERLAP if last_success else started - INITIAL_HISTORY
    feeds = (
        github.issues(since), github.pull_requests(since),
        github.commits(since), github.ci_runs(since, started),
    )
    read_count = 0
    changed_count = 0
    seen = set()
    for feed in feeds:
        for entry in feed:
            if entry.key in seen:
                continue
            seen.add(entry.key)
            changed_count += notion.upsert(entry, notion.find_entry(entry.key))
            read_count += 1
            if entry.updated and (newest is None or entry.updated > newest):
                newest = entry.updated

    # The checkpoint advances only after every activity lookup and write succeeds.
    notion.upsert(
        Entry(
            key=health_key,
            name=f"{github.repository} sync health",
            kind="Sync health",
            state="success",
            updated=newest,
            url=f"{github.url}/actions/runs/{run_id}" if run_id else github.url,
            last_success=now or datetime.now(timezone.utc),
        ),
        health,
    )
    return read_count, changed_count


def config(name):
    value = os.environ.get(name, "").strip()
    if not value:
        raise SyncError(f"Missing configuration: {name}")
    return value


def main():
    try:
        github_token = config("GITHUB_TOKEN")
        notion_token = config("NOTION_TOKEN")
        data_source_id = config("NOTION_DATA_SOURCE_ID")
        github_api = Api("GitHub", "https://api.github.com", github_token, {
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
        })
        notion_api = Api("Notion", "https://api.notion.com", notion_token, {
            "Content-Type": "application/json",
            "Notion-Version": "2025-09-03",
        })
        notion = Notion(notion_api, data_source_id)
        github = GitHub(github_api, config("GITHUB_REPOSITORY"))
        read_count, changed_count = sync(github, notion, os.environ.get("GITHUB_RUN_ID"))
    except SyncError as error:
        print(f"Notion progress sync failed: {error}", file=sys.stderr)
        return 1
    except (KeyError, TypeError, ValueError, AttributeError):
        print("Notion progress sync failed: unexpected API response", file=sys.stderr)
        return 1
    print(f"Notion progress sync succeeded: {read_count} items checked, {changed_count} changed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
