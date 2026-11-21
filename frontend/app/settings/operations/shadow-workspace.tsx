"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";

export type ShadowWindow = {
  holdoutStart: string;
  holdoutEnd: string;
  asOf: string;
  reviewThreshold: number;
};
type Metrics = { tp: number; tn: number; fp: number; fn: number; fpr: number | null; recall: number | null };
export type ShadowReport = {
  window: ShadowWindow;
  bookings: number;
  excluded: {
    assessmentUnavailable: number;
    labelUnavailable: number;
    ambiguousLabel: number;
    baselineOverlap: number;
    unresolved: number;
    evidenceUnavailable: number;
  };
  stored: Metrics;
  shadow: Metrics;
  lineage: {
    profileVersion: string;
    ruleVersion: string;
    jevRuleVersion?: string;
    baselineVersion: string | null;
    modelVersion: string;
    rubricVersion: string;
  }[];
};

const excludedLabels: Record<keyof ShadowReport["excluded"], string> = {
  assessmentUnavailable: "Assessment unavailable at cutoff",
  labelUnavailable: "Verified outcome unavailable at cutoff",
  ambiguousLabel: "Conflicting verified outcomes",
  baselineOverlap: "Baseline overlaps holdout",
  unresolved: "Unresolved assessment",
  evidenceUnavailable: "Pinned evidence unavailable",
};
const percentage = new Intl.NumberFormat(undefined, { style: "percent", maximumFractionDigits: 1 });
const panel = "min-w-0 rounded-xl border border-slate-200 bg-white p-4 shadow-xs sm:p-5";

function localTimestamp(date: Date) {
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 19);
}

function percent(value: number | null) {
  return value === null ? "—" : percentage.format(value);
}

export function ShadowWorkspace({ token, onReport }: { token: string; onReport: (report: ShadowReport | null) => void }) {
  const [dates, setDates] = useState(() => {
    const now = Date.now();
    const end = localTimestamp(new Date(now));
    return { holdoutStart: localTimestamp(new Date(now - 86400000)), holdoutEnd: end, asOf: end };
  });
  const [threshold, setThreshold] = useState("5");
  const [report, setReport] = useState<ShadowReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const activeRequest = useRef<AbortController>(null);

  useEffect(() => () => activeRequest.current?.abort(), []);

  async function evaluate(event: FormEvent) {
    event.preventDefault();
    activeRequest.current?.abort();
    const request = new AbortController();
    activeRequest.current = request;
    setReport(null);
    onReport(null);
    setError("");
    setBusy(true);
    try {
      const response = await fetch("/api/v1/booking/shadow", {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        signal: request.signal,
        body: JSON.stringify({
          holdoutStart: new Date(dates.holdoutStart).toISOString(),
          holdoutEnd: new Date(dates.holdoutEnd).toISOString(),
          asOf: new Date(dates.asOf).toISOString(),
          reviewThreshold: Number(threshold),
        }),
      });
      if (!response.ok) {
        const problem = await response.json().catch(() => null);
        throw new Error(typeof problem?.error === "string" ? problem.error : `Shadow API returned HTTP ${response.status}.`);
      }
      const result: ShadowReport = await response.json();
      if (request.signal.aborted) return;
      setReport(result);
      onReport(result);
    } catch (cause) {
      if (!request.signal.aborted) setError(cause instanceof Error ? cause.message : "Shadow evaluation is unavailable.");
    } finally {
      if (!request.signal.aborted) setBusy(false);
    }
  }

  return <div className="space-y-4">
    <section className={panel} aria-labelledby="shadow-heading">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 id="shadow-heading" className="text-lg font-semibold">Booking rule evaluation</h2>
          <p className="mt-1 text-sm text-slate-600">Compare the recorded decisions with a candidate velocity rule on a fixed window.</p>
        </div>
        <Link href="/settings/system-rules" className="text-sm font-semibold text-indigo-700 hover:underline">Open visual rule builder</Link>
      </div>
      <form className="mt-4 space-y-4" onSubmit={evaluate}>
        <fieldset disabled={busy} className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <label className="block min-w-0 text-xs font-semibold text-slate-600">Holdout start
            <input className="control mt-1.5 min-w-0" type="datetime-local" step="1" required value={dates.holdoutStart} onChange={event => setDates({ ...dates, holdoutStart: event.currentTarget.value })} />
          </label>
          <label className="block min-w-0 text-xs font-semibold text-slate-600">Holdout end
            <input className="control mt-1.5 min-w-0" type="datetime-local" step="1" required value={dates.holdoutEnd} onChange={event => setDates({ ...dates, holdoutEnd: event.currentTarget.value })} />
          </label>
          <label className="block min-w-0 text-xs font-semibold text-slate-600">Data cutoff
            <input className="control mt-1.5 min-w-0" type="datetime-local" step="1" required value={dates.asOf} onChange={event => setDates({ ...dates, asOf: event.currentTarget.value })} />
          </label>
          <label className="block min-w-0 text-xs font-semibold text-slate-600">Review at bookings per minute
            <input className="control mt-1.5" type="number" min="1" max={Number.MAX_SAFE_INTEGER} step="1" required value={threshold} onChange={event => setThreshold(event.currentTarget.value)} />
          </label>
        </fieldset>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <button type="submit" disabled={busy} className="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50">{busy ? "Evaluating…" : "Run shadow report"}</button>
          <p className="text-xs text-slate-500">Uses verified synthetic outcomes and pinned evidence.</p>
        </div>
      </form>
      {error && <p className="mt-3 text-sm text-red-800" role="alert">{error}</p>}
    </section>
    {report && <ShadowResults report={report} />}
  </div>;
}

function ShadowResults({ report }: { report: ShadowReport }) {
  const scored = report.stored.tp + report.stored.tn + report.stored.fp + report.stored.fn;
  return <section className={panel} aria-label="Shadow report results">
    <h3 className="text-base font-semibold">Shadow report</h3>
    <p className="mt-1 text-sm text-slate-600">{scored} scored of {report.bookings} bookings · Candidate threshold {report.window.reviewThreshold}</p>
    <div className="mt-4 grid gap-3 sm:grid-cols-2">
      {([{ title: "Recorded", metrics: report.stored }, { title: "Candidate", metrics: report.shadow }]).map(({ title, metrics }) => <div className="rounded-lg border border-slate-200 p-3" key={title}>
        <h4 className="text-sm font-semibold">{title}</h4>
        <dl className="mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
          {[
            ["False positive rate", percent(metrics.fpr)], ["Recall", percent(metrics.recall)],
            ["True positives", metrics.tp], ["True negatives", metrics.tn],
            ["False positives", metrics.fp], ["False negatives", metrics.fn],
          ].map(([label, value]) => <div key={label}><dt className="text-xs text-slate-600">{label}</dt><dd className="font-semibold">{value}</dd></div>)}
        </dl>
      </div>)}
    </div>
    {Object.entries(report.excluded).some(([, count]) => count > 0) && <div className="mt-4">
      <h4 className="text-sm font-semibold">Excluded from scoring</h4>
      <ul className="mt-1 text-sm text-slate-600">
        {Object.entries(excludedLabels).map(([key, label]) => {
          const count = report.excluded[key as keyof ShadowReport["excluded"]];
          return count > 0 ? <li key={key}>{label} · {count}</li> : null;
        })}
      </ul>
    </div>}
    {report.lineage.length > 0 && <details className="mt-4 text-sm">
      <summary className="cursor-pointer font-semibold">Pinned versions</summary>
      <ul className="mt-2 space-y-2 break-all text-xs text-slate-600">
        {report.lineage.map((entry, index) => <li key={index}>{entry.profileVersion} · {entry.ruleVersion}{entry.jevRuleVersion && ` · Jev rules ${entry.jevRuleVersion}`} · {entry.baselineVersion ?? "Cold start"} · {entry.modelVersion} · {entry.rubricVersion}</li>)}
      </ul>
    </details>}
  </section>;
}
