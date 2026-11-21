"use client";

import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { bookingRequest } from "../booking-assessment/booking-api";
import type { Proposal } from "../booking-assessment/booking-api";
import { operationsRequest, products } from "./contracts";
import type { Actor, Queue } from "./contracts";
import type { ShadowReport, ShadowWindow } from "./shadow-workspace";

const kinds = { rule: "Velocity rule", profile: "Scoring profile", rubric: "Jev rubric", list: "Product review list" };
type Kind = keyof typeof kinds;
type Change = {
  id: string;
  kind: Kind;
  makerId: string;
  candidate: { reviewThreshold?: number; elevatedVelocity?: number; reviewProducts?: string[]; setup?: Proposal["setup"] };
  shadow?: ShadowReport;
  rollbackOf?: string;
  expiresAt: string;
  status: "PENDING" | "APPROVED" | "REJECTED" | "EXPIRED";
  revision?: string;
  active: boolean;
  decision?: { checkerId: string; decision: "approve" | "reject"; decidedAt: string };
};
type ChangeDraft = { kind: Kind; candidate?: unknown; rollbackOf?: string; shadowWindow?: ShadowWindow };
type ChangeRequest = ChangeDraft & { id: string; expiresAt: string };

const card = "min-w-0 rounded-xl border border-slate-200 bg-white p-4 shadow-xs sm:p-5";
const button = "rounded-lg border border-slate-300 bg-white px-3.5 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50";
const primary = "rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50";
const field = "block min-w-0 text-xs font-semibold text-slate-600";
const time = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

async function readChanges(token: string, signal: AbortSignal) {
  const response = await fetch("/api/v1/booking/changes", {
    headers: { Authorization: `Bearer ${token}` }, cache: "no-store", signal,
  });
  if (!response.ok) {
    const problem = await response.json().catch(() => null);
    throw new Error(typeof problem?.error === "string" ? problem.error : `Changes API returned HTTP ${response.status}.`);
  }
  return response.json() as Promise<Change[]>;
}

function candidateSummary(change: Change) {
  const { candidate } = change;
  switch (change.kind) {
    case "rule": return `Review at ${candidate.reviewThreshold} bookings per minute`;
    case "rubric": return `Elevated at ${candidate.elevatedVelocity} bookings per minute`;
    case "list": return candidate.reviewProducts?.length
      ? candidate.reviewProducts.map(product => products[product as keyof typeof products] ?? product).join(", ")
      : "No products";
    case "profile":
      return `${candidate.setup?.businessModel === "ota" ? "Travel" : "E-commerce"} · ${candidate.setup?.availableSignals.includes("amountMinor") ? "Timestamp and amount" : "Timestamp only"}`;
  }
}

export function ChangeGovernance({ token, actor, report }: { token: string; actor: Actor; report: ShadowReport | null }) {
  const [changes, setChanges] = useState<Change[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [kind, setKind] = useState<Kind>("rule");
  const [duration, setDuration] = useState("60");
  const [rubric, setRubric] = useState("5");
  const [profile, setProfile] = useState<"ota" | "ecommerce">("ota");
  const [amountSignal, setAmountSignal] = useState(true);
  const [listed, setListed] = useState<string[]>([]);
  const [checkerInput, setCheckerInput] = useState("");
  const [checker, setChecker] = useState<{ token: string; actor: Actor } | null>(null);
  const pending = useRef<{ signature: string; request: ChangeRequest } | null>(null);
  const listRequest = useRef<AbortController>(null);
  const maker = actor.role === "merchant";
  const effectiveChecker = actor.role === "analyst" ? { token, actor } : checker;
  const scored = report ? report.shadow.tp + report.shadow.tn + report.shadow.fp + report.shadow.fn : 0;

  useEffect(() => {
    const controller = new AbortController();
    listRequest.current = controller;
    readChanges(token, controller.signal).then(
      result => { if (!controller.signal.aborted) setChanges(result); },
      cause => { if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "Error loading change proposals."); },
    ).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => listRequest.current?.abort();
  }, [token]);

  async function withAction(action: () => Promise<string>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      setNotice(await action());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not complete the change request.");
    } finally {
      setBusy(false);
    }
  }

  async function refresh() {
    listRequest.current?.abort();
    const controller = new AbortController();
    listRequest.current = controller;
    const next = await readChanges(token, controller.signal);
    if (!controller.signal.aborted) setChanges(next);
  }

  async function submitChange(draft: ChangeDraft) {
    if (!Number.isInteger(Number(duration)) || Number(duration) < 1 || Number(duration) > 1440) {
      throw new Error("Set proposal lifetime from 1 to 1440 minutes.");
    }
    const signature = JSON.stringify({ draft, duration });
    if (pending.current?.signature !== signature) {
      pending.current = {
        signature,
        request: { ...draft, id: crypto.randomUUID(), expiresAt: new Date(Date.now() + Number(duration) * 60000).toISOString() },
      };
    }
    const proposal = await bookingRequest<Change>("/changes", token, pending.current.request);
    pending.current = null;
    setChanges(rows => [proposal, ...rows.filter(row => row.id !== proposal.id)]);
    return "Proposal submitted for independent review.";
  }

  function propose(event: FormEvent) {
    event.preventDefault();
    void withAction(async () => {
      if (kind === "rule") {
        if (!report || scored === 0) throw new Error("Run a shadow report with at least one scored booking.");
        return submitChange({ kind, candidate: { reviewThreshold: report.window.reviewThreshold }, shadowWindow: report.window });
      }
      if (kind === "rubric") return submitChange({ kind, candidate: { elevatedVelocity: Number(rubric) } });
      if (kind === "list") return submitChange({ kind, candidate: { reviewProducts: listed } });
      const candidate = await bookingRequest<Proposal>("/profiles/proposal", token, {
        schemaVersion: 1, stage: "booking", businessModel: profile,
        availableSignals: amountSignal ? ["occurredAt", "amountMinor"] : ["occurredAt"],
      });
      return submitChange({ kind, candidate });
    });
  }

  function connectChecker(event: FormEvent) {
    event.preventDefault();
    void withAction(async () => {
      const credential = checkerInput.trim();
      const result = await operationsRequest<Queue>("?limit=1", credential);
      if (result.actor.role !== "analyst" || result.actor.merchantId !== actor.merchantId || result.actor.actorId === actor.actorId) {
        throw new Error("Use a different analyst actor from this merchant.");
      }
      setChecker({ token: credential, actor: result.actor });
      setCheckerInput("");
      await refresh();
      return `Checker connected · ${result.actor.actorId}.`;
    });
  }

  function decide(change: Change, decision: "approve" | "reject") {
    if (!effectiveChecker) return;
    void withAction(async () => {
      const decided = await bookingRequest<Change>(`/changes/${change.id}/decision`, effectiveChecker.token, { decision });
      setChanges(rows => rows.map(row => {
        if (row.id === decided.id) return decided;
        return decided.active && row.kind === decided.kind ? { ...row, active: false } : row;
      }));
      return decision === "approve" ? "Change approved." : "Change rejected.";
    });
  }

  function rollback(change: Change) {
    const draft: ChangeDraft = { kind: change.kind, rollbackOf: change.id };
    if (change.kind === "rule") {
      if (report?.window.reviewThreshold !== change.candidate.reviewThreshold || scored === 0) {
        setError(`Run a shadow report with threshold ${change.candidate.reviewThreshold} and at least one scored booking before requesting this rollback.`);
        return;
      }
      draft.shadowWindow = report?.window;
    }
    void withAction(() => submitChange(draft));
  }

  return <section className={card} aria-labelledby="governance-heading">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div>
        <h2 id="governance-heading" className="text-lg font-semibold">Change approvals</h2>
        <p className="mt-1 text-sm text-slate-600">Submit a versioned change for review by a separate analyst.</p>
      </div>
      <button type="button" className={button} disabled={busy || loading} onClick={() => void withAction(async () => {
        await refresh();
        return "Change list refreshed.";
      })}>Refresh changes</button>
    </div>
    {maker && <form className="mt-4 space-y-3 rounded-lg border border-slate-200 p-3" onSubmit={propose}>
      <fieldset disabled={busy} className="grid gap-3 sm:grid-cols-2">
        <label className={field}>Change type
          <select className="control mt-1.5" value={kind} onChange={event => setKind(event.currentTarget.value as Kind)}>
            {Object.entries(kinds).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <label className={field}>Expires in minutes
          <input className="control mt-1.5" type="number" min="1" max="1440" step="1" required value={duration} onChange={event => setDuration(event.currentTarget.value)} />
        </label>
        {kind === "rubric" && <label className={field}>Elevated at bookings per minute
          <input className="control mt-1.5" type="number" min="1" max={Number.MAX_SAFE_INTEGER} step="1" required value={rubric} onChange={event => setRubric(event.currentTarget.value)} />
        </label>}
        {kind === "profile" && <>
          <label className={field}>Business model
            <select className="control mt-1.5" value={profile} onChange={event => setProfile(event.currentTarget.value === "ecommerce" ? "ecommerce" : "ota")}>
              <option value="ota">Travel</option><option value="ecommerce">E-commerce</option>
            </select>
          </label>
          <label className="flex items-center gap-2 self-end py-2 text-sm"><input type="checkbox" checked={amountSignal} onChange={event => setAmountSignal(event.currentTarget.checked)} />Include booking amount</label>
        </>}
        {kind === "list" && <fieldset className="sm:col-span-2">
          <legend className={field}>Products requiring review</legend>
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-2">
            {Object.entries(products).map(([value, label]) => <label className="flex items-center gap-2 text-sm" key={value}>
              <input type="checkbox" checked={listed.includes(value)} onChange={event => setListed(event.currentTarget.checked ? [...listed, value] : listed.filter(item => item !== value))} />
              {label}
            </label>)}
          </div>
        </fieldset>}
      </fieldset>
      {kind === "rule" && <p className="text-sm text-slate-600">{report && scored > 0 ? `Previewed at ${report.window.reviewThreshold} bookings per minute over ${scored} scored bookings.` : "Run a shadow report with at least one scored booking to propose a velocity rule."}</p>}
      <button type="submit" className={primary} disabled={busy || (kind === "rule" && scored === 0)}>Submit for approval</button>
    </form>}
    {maker && <form className="mt-4 flex flex-wrap items-end gap-2 rounded-lg border border-slate-200 p-3" onSubmit={connectChecker}>
      {checker
        ? <><p className="flex-1 text-sm text-slate-700">Checker connected · {checker.actor.actorId}</p><button type="button" className={button} disabled={busy} onClick={() => setChecker(null)}>Disconnect checker</button></>
        : <>
          <label className={field + " flex-1"}>Independent analyst token
            <input className="control mt-1.5" type="password" autoComplete="off" disabled={busy} value={checkerInput} onChange={event => setCheckerInput(event.currentTarget.value)} />
          </label>
          <button type="submit" className={button} disabled={busy || !checkerInput.trim()}>Connect checker</button>
        </>}
    </form>}
    {error && <p className="mt-3 text-sm text-red-800" role="alert">{error}</p>}
    {notice && <p className="mt-3 text-sm text-emerald-800" role="status">{notice}</p>}
    {loading ? <p className="mt-4 text-sm text-slate-600" role="status">Loading changes…</p>
      : changes.length === 0 ? <p className="mt-4 text-sm text-slate-600">No change proposals yet.</p>
        : <ul className="mt-4 divide-y divide-slate-200 rounded-lg border border-slate-200">
          {changes.map(change => <li key={change.id} className="space-y-2 p-3 text-sm">
            <div className="flex flex-wrap justify-between gap-2"><strong>{kinds[change.kind]}</strong><span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-semibold">{change.active ? "Active" : change.status}</span></div>
            <p>{candidateSummary(change)}</p>
            <p className="text-xs text-slate-600">Maker {change.makerId} · Expires {time.format(new Date(change.expiresAt))}{change.decision && ` · Checker ${change.decision.checkerId}`}</p>
            {change.revision && <p className="break-all font-mono text-xs text-slate-500">{change.revision}</p>}
            {change.rollbackOf && <p className="break-all text-xs text-slate-600">Rollback of {change.rollbackOf}</p>}
            {change.shadow && <details className="text-xs text-slate-600">
              <summary className="cursor-pointer font-semibold">Pinned shadow report</summary>
              <p className="mt-1">Window {time.format(new Date(change.shadow.window.holdoutStart))} to {time.format(new Date(change.shadow.window.holdoutEnd))} · Cutoff {time.format(new Date(change.shadow.window.asOf))}</p>
              <p className="mt-1">Candidate TP {change.shadow.shadow.tp} · TN {change.shadow.shadow.tn} · FP {change.shadow.shadow.fp} · FN {change.shadow.shadow.fn}</p>
            </details>}
            {change.status === "PENDING" && effectiveChecker && effectiveChecker.actor.actorId !== change.makerId && <div className="flex flex-wrap gap-2">
              <button type="button" className={primary} disabled={busy} onClick={() => decide(change, "approve")}>Approve</button>
              <button type="button" className={button} disabled={busy} onClick={() => decide(change, "reject")}>Reject</button>
            </div>}
            {maker && change.status === "APPROVED" && !change.active && <button type="button" className={button} disabled={busy} onClick={() => rollback(change)}>Propose rollback</button>}
          </li>)}
        </ul>}
  </section>;
}
