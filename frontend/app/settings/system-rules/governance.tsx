"use client";

import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { useRuleCredential } from "../rule-composer";
import { outlineButton, primaryButton, ruleRequest, rulesEndpoint } from "./contracts";
import type { Preview, Rule, RuleCatalogue } from "./contracts";

type GovernanceVersion = {
  version: number;
  draftVersion: number;
  status: "PENDING" | "ACTIVE" | "DISABLED";
  action: string;
  rule: Rule;
  preview: Preview;
  reason: string;
  actor: string;
  createdAt?: string;
};
type GovernanceState = { activeVersion: number; versions: GovernanceVersion[] };
type Operation = "submit" | "approve" | "rollback";

const endpoint = "/api/v1/booking/rule-composer/platform";
const versionLabels = { PENDING: "Pending approval", ACTIVE: "Active", DISABLED: "Disabled" };
const actorLabels: Record<string, string> = { "platform-admin": "Platform administrator", "platform-checker": "Independent checker" };

function request<T>(path: string, token: string, init?: RequestInit) {
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (init?.body) headers.set("Content-Type", "application/json");
  return ruleRequest<T>(path, { ...init, headers, cache: "no-store" });
}

function failureText(cause: unknown) {
  return cause instanceof Error ? cause.message : "The platform approval service is unavailable.";
}

export function PlatformGovernance({ catalogueRevision }: { catalogueRevision: number }) {
  return <details className="rounded-xl border border-line bg-white p-4 sm:p-5">
    <summary className="cursor-pointer text-lg font-semibold">Platform rule approval</summary>
    <GovernanceWorkflow catalogueRevision={catalogueRevision} />
  </details>;
}

function GovernanceWorkflow({ catalogueRevision }: { catalogueRevision: number }) {
  const administrator = useRuleCredential("PLATFORM");
  const [checker, setChecker] = useState("");
  const [catalogue, setCatalogue] = useState<RuleCatalogue | null>(null);
  const [selection, setSelection] = useState("");
  const [history, setHistory] = useState<GovernanceState | null>(null);
  const [runId, setRunId] = useState("");
  const [reason, setReason] = useState("");
  const [checkerReason, setCheckerReason] = useState("");
  const [version, setVersion] = useState("");
  const [busy, setBusy] = useState<Operation | "load" | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const active = useRef<AbortController>(null);
  const drafts = catalogue?.rules.filter(rule => rule.status === "DRAFT") ?? [];
  const draft = drafts.find(rule => rule.id === selection);

  useEffect(() => {
    const controller = new AbortController();
    void request<RuleCatalogue>(rulesEndpoint, administrator, { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) setCatalogue(result); })
      .catch(cause => { if (!controller.signal.aborted) setError(failureText(cause)); });
    return () => controller.abort();
  }, [administrator, catalogueRevision]);

  useEffect(() => () => active.current?.abort(), []);

  function selectDraft(id: string) {
    setSelection(id);
    setHistory(null);
    setRunId("");
    setVersion("");
    setError("");
    setNotice("");
  }

  async function update(token: string, operation: Operation | "load", body?: unknown) {
    if (active.current || !draft || !token) return;
    const controller = new AbortController();
    const { signal } = controller;
    active.current = controller;
    setBusy(operation);
    setNotice("");
    setError("");
    const rulePath = `${endpoint}/${encodeURIComponent(draft.id)}`;
    try {
      if (operation !== "load") {
        const result = await request<GovernanceVersion>(`${rulePath}/${operation}`, token, {
          method: "POST", signal, body: JSON.stringify(body),
        });
        if (signal.aborted) return;
        setNotice(`Version ${result.version} · ${versionLabels[result.status]}`);
      }
      const result = await request<GovernanceState>(`${rulePath}/versions`, token, { signal });
      if (!signal.aborted) {
        setHistory(result);
        setVersion("");
      }
    } catch (cause) {
      if (!signal.aborted) setError(failureText(cause));
    } finally {
      if (!signal.aborted) setBusy(null);
      active.current = null;
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!draft || !runId || !reason.trim()) return;
    void update(administrator, "submit", { draftVersion: draft.version, runId, reason: reason.trim() });
  }

  function check(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const button = (event.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null;
    const operation = button?.value;
    const target = Number(version);
    if (!history || !version || !checkerReason.trim() || !checker.trim()) return;
    if (operation === "approve") void update(checker.trim(), operation, { version: target, reason: checkerReason.trim() });
    if (operation === "rollback") void update(checker.trim(), operation, {
      version: target, expectedActiveVersion: history.activeVersion, reason: checkerReason.trim(),
    });
  }

  const selectedVersion = history?.versions.find(item => item.version === Number(version));
  const loaderToken = checker.trim() || administrator;
  return <div className="mt-4 space-y-4 text-sm">
    {error && <p role="alert" className="text-red-800">{error}</p>}
    {notice && <p role="status" className="font-semibold text-emerald-800">{notice}</p>}
    {!catalogue && !error && <p role="status" className="text-muted">Loading drafts…</p>}
    {catalogue && drafts.length === 0 && <p className="text-muted">Save a platform draft to begin approval.</p>}
    {drafts.length > 0 && <>
      <label className="block max-w-md font-semibold">Platform draft
        <select className="control mt-1" value={selection} disabled={busy !== null} onChange={event => selectDraft(event.currentTarget.value)}>
          <option value="">Choose a draft</option>
          {drafts.map(item => <option key={item.id} value={item.id}>{item.name} · Draft v{item.version}</option>)}
        </select>
      </label>
      {draft && administrator && <form className="space-y-3 border-t border-line pt-3" onSubmit={submit}>
        <h3 className="font-semibold">Submit for approval</h3>
        <label className="block max-w-md font-semibold">Saved simulation run
          <select className="control mt-1" required value={runId} onChange={event => setRunId(event.currentTarget.value)} disabled={busy !== null}>
            <option value="">Choose evidence for replay</option>
            {(catalogue?.runs ?? []).map(run => <option key={run.id} value={run.id}>{run.product} · Seed {run.seed} · {run.id.slice(0, 8)}</option>)}
          </select>
        </label>
        <label className="block max-w-md font-semibold">Reason for submission
          <input className="control mt-1" required maxLength={500} value={reason} onChange={event => setReason(event.currentTarget.value)} disabled={busy !== null} />
        </label>
        <button className={primaryButton} type="submit" disabled={busy !== null || !runId || !reason.trim()}>
          {busy === "submit" ? "Submitting…" : "Submit for approval"}
        </button>
      </form>}
      {draft && <section className="space-y-3 border-t border-line pt-3" aria-label="Checker approval">
        <h3 className="font-semibold">Checker approval</h3>
        <label className="block max-w-md font-semibold">Checker token
          <input className="control mt-1" type="password" value={checker} autoComplete="off" disabled={busy !== null} onChange={event => {
            setChecker(event.currentTarget.value);
            setHistory(null);
            setVersion("");
          }} />
        </label>
        <button className={outlineButton} type="button" disabled={busy !== null || !loaderToken} onClick={() => void update(loaderToken, "load")}>
          {busy === "load" ? "Loading versions…" : "Load versions"}
        </button>
        {history && <>
          <p className="text-muted">{history.activeVersion ? `Active version ${history.activeVersion}` : "No version is active."}</p>
          <ul className="divide-y divide-line">
            {history.versions.map(item => <li key={item.version} className="py-2">
              <p className="font-semibold">Version {item.version} · {versionLabels[item.status]}</p>
              <p className="text-xs text-muted">
                {actorLabels[item.actor] ?? item.actor}{item.createdAt && ` · ${new Date(item.createdAt).toLocaleString()}`}
              </p>
              {item.preview && <p className="text-muted">{item.preview.matched} of {item.preview.eligible} eligible events matched · {item.preview.skipped} missing signals</p>}
              <p className="text-muted wrap-anywhere">{item.reason}</p>
            </li>)}
          </ul>
          {checker.trim() && history.versions.length > 0 && <form className="space-y-3" onSubmit={check}>
            <label className="block max-w-md font-semibold">Version for approval or rollback
              <select className="control mt-1" required value={version} onChange={event => setVersion(event.currentTarget.value)} disabled={busy !== null}>
                <option value="">Choose a version</option>
                {history.activeVersion > 0 && <option value="0">Withdraw the active version</option>}
                {history.versions.filter(item => item.version !== history.activeVersion && (item.status === "PENDING" || item.status === "ACTIVE")).map(item =>
                  <option key={item.version} value={item.version}>Version {item.version} · {versionLabels[item.status]}</option>)}
              </select>
            </label>
            <label className="block max-w-md font-semibold">Reason for decision
              <input className="control mt-1" required maxLength={500} value={checkerReason} disabled={busy !== null} onChange={event => setCheckerReason(event.currentTarget.value)} />
            </label>
            <div className="flex flex-wrap gap-2">
              {selectedVersion?.status === "PENDING" && <button className={primaryButton} type="submit" value="approve" disabled={busy !== null || !checkerReason.trim()}>Approve version</button>}
              {(selectedVersion?.status === "ACTIVE" || version === "0") && <button className={outlineButton} type="submit" value="rollback"
                disabled={busy !== null || !checkerReason.trim()}>{version === "0" ? "Withdraw rule" : "Restore version"}</button>}
            </div>
          </form>}
        </>}
      </section>}
    </>}
  </div>;
}
