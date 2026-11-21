"use client";

import { Clock3, RotateCw, Shield, Timer, TriangleAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { AssessmentResult } from "../booking-assessment/assessment-result";
import { holdStates, operationsRequest, outcomes, products } from "./contracts";
import type { Detail, Filters, Hold, Queue, Reveal, Transaction } from "./contracts";

const emptyFilters = { product: "", outcome: "", hold: "" };
const panel = "min-w-0 rounded-xl border border-slate-200 bg-white shadow-xs";
const button = "inline-flex items-center justify-center gap-2 rounded-lg border border-slate-300 bg-white px-3.5 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50";
const primary = "inline-flex items-center justify-center gap-2 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50";
const timestamp = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });
const number = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });

function formatDate(value: string) {
  return timestamp.format(new Date(value));
}

function errorMessage(cause: unknown) {
  return cause instanceof Error ? cause.message : "The operations API is unavailable.";
}

export function OperationsWorkspace() {
  const [token, setToken] = useState("");
  const [queue, setQueue] = useState<Queue | null>(null);
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [activeFilters, setActiveFilters] = useState<Filters>(emptyFilters);
  const [queueLoading, setQueueLoading] = useState(false);
  const [queueError, setQueueError] = useState("");
  const [selected, setSelected] = useState("");
  const [detail, setDetail] = useState<Detail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [revealed, setRevealed] = useState<Reveal | null>(null);
  const [reason, setReason] = useState("manual_review");
  const [resolution, setResolution] = useState("cleared");
  const [operation, setOperation] = useState("");
  const [operationError, setOperationError] = useState("");
  const [operationStatus, setOperationStatus] = useState("");
  const queueRequest = useRef<AbortController>(null);
  const detailRequest = useRef<AbortController>(null);
  const writeRequest = useRef<AbortController>(null);
  const detailHeading = useRef<HTMLHeadingElement>(null);
  const bearer = token.trim();

  useEffect(() => () => {
    queueRequest.current?.abort();
    detailRequest.current?.abort();
    writeRequest.current?.abort();
  }, []);

  function changeToken(value: string) {
    setToken(value);
    if (value.trim() === bearer) return;
    queueRequest.current?.abort();
    detailRequest.current?.abort();
    writeRequest.current?.abort();
    setQueue(null);
    setQueueLoading(false);
    setQueueError("");
    setFilters(emptyFilters);
    setActiveFilters(emptyFilters);
    setSelected("");
    setDetail(null);
    setDetailLoading(false);
    setDetailError("");
    setRevealed(null);
    setOperation("");
    setOperationError("");
    setOperationStatus("");
  }

  async function loadQueue(credential: string, criteria: Filters) {
    queueRequest.current?.abort();
    const request = new AbortController();
    queueRequest.current = request;
    const search = new URLSearchParams({ limit: "50" });
    Object.entries(criteria).forEach(([key, value]) => {
      if (value) search.set(key, value);
    });
    setQueueLoading(true);
    setQueueError("");
    try {
      const result = await operationsRequest<Queue>(`?${search}`, credential, { signal: request.signal });
      if (!request.signal.aborted) {
        setQueue(result);
        setActiveFilters(criteria);
      }
    } catch (cause) {
      if (!request.signal.aborted) setQueueError(errorMessage(cause));
    } finally {
      if (!request.signal.aborted) setQueueLoading(false);
    }
  }

  async function openDetail(eventId: string) {
    detailRequest.current?.abort();
    const request = new AbortController();
    detailRequest.current = request;
    setSelected(eventId);
    setDetail(null);
    setDetailLoading(true);
    setDetailError("");
    setOperationError("");
    setOperationStatus("");
    setRevealed(null);
    setReason("manual_review");
    setResolution("cleared");
    detailHeading.current?.focus();
    try {
      const result = await operationsRequest<Detail>(`/${eventId}`, bearer, { signal: request.signal });
      if (!request.signal.aborted) setDetail(result);
    } catch (cause) {
      if (!request.signal.aborted) setDetailError(errorMessage(cause));
    } finally {
      if (!request.signal.aborted) setDetailLoading(false);
    }
  }

  async function runAction(action: "reveal" | "claim" | "resolve" | "escalate") {
    if (!detail || operation) return;
    const request = new AbortController();
    writeRequest.current = request;
    const eventId = detail.booking.eventId;
    setOperation(action);
    setOperationError("");
    setOperationStatus("");
    try {
      if (action === "reveal") {
        const value = await operationsRequest<Reveal>(`/${eventId}/reveal`, bearer, { write: true, data: { reason }, signal: request.signal });
        if (!request.signal.aborted) setRevealed(value);
      } else {
        const hold = await operationsRequest<Hold>(`/${eventId}/hold/${action}`, bearer, {
          write: true, data: action === "resolve" ? { resolution } : undefined, signal: request.signal,
        });
        if (!request.signal.aborted) {
          setDetail({ ...detail, hold });
          setOperationStatus(`Review marked ${holdStates[hold.state].toLowerCase()}.`);
          void loadQueue(bearer, activeFilters);
        }
      }
    } catch (cause) {
      if (!request.signal.aborted) setOperationError(errorMessage(cause));
    } finally {
      if (!request.signal.aborted) setOperation("");
    }
  }

  function connect(event: FormEvent) {
    event.preventDefault();
    if (bearer) void loadQueue(bearer, activeFilters);
  }

  return <div className="space-y-5">
    <form className={`${panel} grid gap-3 p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end sm:p-5`} onSubmit={connect}>
      <div>
        <label htmlFor="operations-token" className="block text-sm font-semibold">Demo operations token</label>
        <input id="operations-token" className="control mt-2" type="password" autoComplete="off" value={token}
          onChange={event => changeToken(event.currentTarget.value)} aria-describedby="operations-token-help" />
        <p id="operations-token-help" className="mt-2 text-xs text-slate-500">This session keeps the token in memory.</p>
      </div>
      <button type="submit" className={primary} disabled={!bearer || queueLoading}>{queue ? "Reconnect" : "Load transactions"}</button>
    </form>
    {queueError && <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{queueError}</p>}
    {queueLoading && !queue && <p role="status" className="px-1 text-sm text-slate-600">Loading transactions…</p>}
    {!queueLoading && !queue && !queueError && <p className="px-1 text-sm text-slate-600">Enter an operations credential to see your merchant’s transactions.</p>}
    {queue && <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="flex min-w-0 items-center gap-2 break-all text-sm text-slate-600">
          <Shield className="size-4 shrink-0" aria-hidden="true" />{queue.actor.merchantId} · {queue.actor.actorId} · {queue.actor.role}
        </p>
        <button type="button" className={button} disabled={queueLoading || Boolean(operation)} onClick={() => {
          void loadQueue(bearer, activeFilters);
          if (selected) void openDetail(selected);
        }}>
          <RotateCw className="size-4" aria-hidden="true" />{queueLoading ? "Refreshing…" : "Refresh"}
        </button>
      </div>
      <QueueMetrics kpis={queue.kpis} />
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <section className={panel} aria-labelledby="transactions-heading" aria-busy={queueLoading}>
          <div className="p-4 sm:p-5">
            <h2 id="transactions-heading" className="text-lg font-semibold">Transactions</h2>
            <form className="mt-4 grid gap-3 sm:grid-cols-2" aria-label="Transaction filters" onSubmit={event => {
              event.preventDefault();
              void loadQueue(bearer, filters);
            }}>
              <Filter label="Product" values={products} value={filters.product} onChange={product => setFilters({ ...filters, product })} />
              <Filter label="Assessment" values={outcomes} value={filters.outcome} onChange={outcome => setFilters({ ...filters, outcome })} />
              <Filter label="Hold state" values={holdStates} value={filters.hold} onChange={hold => setFilters({ ...filters, hold })} />
              <button type="submit" className={button + " self-end"} disabled={queueLoading}>Apply filters</button>
            </form>
          </div>
          {queue.transactions.length === 0
            ? <p className="border-t border-slate-100 px-5 py-6 text-sm text-slate-600">No transactions match these filters.</p>
            : <ul className="divide-y divide-slate-100 border-t border-slate-100">
              {queue.transactions.map(row => <li key={row.eventId}>
                <button type="button" aria-pressed={row.eventId === selected} disabled={Boolean(operation)}
                  onClick={() => void openDetail(row.eventId)}
                  className={`block w-full px-4 py-3.5 text-left outline-offset-[-2px] hover:bg-slate-50 disabled:opacity-60 sm:px-5 ${row.eventId === selected ? "bg-indigo-50" : ""}`}>
                  <TransactionRow row={row} />
                </button>
              </li>)}
            </ul>}
          {queue.transactions.length === 50 && <p className="border-t border-slate-100 px-5 py-3 text-xs text-slate-500">Showing the 50 most recent matches.</p>}
        </section>
        <div className="min-w-0 space-y-4">
          <section className={`${panel} p-4 sm:p-5`} aria-labelledby="transaction-detail-heading" aria-busy={detailLoading}>
            <h2 id="transaction-detail-heading" ref={detailHeading} tabIndex={-1} className="text-lg font-semibold">Transaction detail</h2>
            {!selected && <p className="mt-2 text-sm text-slate-600">Select a transaction to inspect its evidence.</p>}
            {detailLoading && <p role="status" className="mt-2 text-sm text-slate-600">Loading transaction…</p>}
            {detailError && <div className="mt-3">
              <p role="alert" className="text-sm text-red-800">{detailError}</p>
              <button type="button" className={button + " mt-2"} onClick={() => void openDetail(selected)}>Retry detail</button>
            </div>}
            {detail && <div className="mt-4 space-y-4">
              <dl className="grid gap-3 text-sm sm:grid-cols-2">
                <div className="min-w-0"><dt className="text-xs font-medium text-slate-600">Event ID</dt><dd className="mt-1 break-all">{detail.booking.eventId}</dd></div>
                <div><dt className="text-xs font-medium text-slate-600">Product</dt><dd className="mt-1">{detail.booking.product ? products[detail.booking.product] ?? detail.booking.product : "Unspecified"}</dd></div>
              </dl>
              <div className="space-y-3 border-t border-slate-100 pt-4">
                <h3 className="text-sm font-semibold">Synthetic contact</h3>
                <p className="break-all text-sm text-slate-700">
                  {detail.contact.state === "MASKED" ? revealed?.email ?? detail.contact.email : "Unavailable"}
                </p>
                {revealed && <p role="status" className="break-all text-xs text-emerald-800">Reveal audit ID {revealed.auditId}</p>}
                {detail.actor.role === "analyst" && detail.contact.state === "MASKED" && <div className="flex flex-wrap items-end gap-2">
                  {revealed
                    ? <button type="button" className={button} onClick={() => setRevealed(null)}>Hide contact</button>
                    : <>
                      <Filter label="Reveal reason" values={{ manual_review: "Manual review", support: "Support" }} value={reason} onChange={setReason} optional={false} disabled={Boolean(operation)} />
                      <button type="button" className={button} disabled={Boolean(operation)} onClick={() => void runAction("reveal")}>Reveal contact</button>
                    </>}
                </div>}
              </div>
              {detail.hold && <HoldDetail hold={detail.hold} detail={detail} operation={operation} resolution={resolution} setResolution={setResolution} runAction={runAction} />}
              {operationError && <p className="text-sm text-red-800" role="alert">{operationError}</p>}
              {operationStatus && <p className="text-sm text-emerald-800" role="status">{operationStatus}</p>}
            </div>}
          </section>
          {detail?.assessment && <AssessmentResult assessment={detail.assessment} booking={detail.booking} />}
          {detail && !detail.assessment && <div className={`${panel} p-5 text-sm text-slate-600`}>This booking is awaiting assessment.</div>}
        </div>
      </div>
    </>}
  </div>;
}

function Filter({ label, values, value, onChange, optional = true, disabled = false }: {
  label: string; values: Record<string, string>; value: string; onChange: (value: string) => void; optional?: boolean; disabled?: boolean;
}) {
  return <label className="block min-w-0 text-xs font-semibold text-slate-600">{label}
    <select className="control mt-1.5" value={value} disabled={disabled} onChange={event => onChange(event.currentTarget.value)}>
      {optional && <option value="">All</option>}
      {Object.entries(values).map(([key, text]) => <option key={key} value={key}>{text}</option>)}
    </select>
  </label>;
}

function QueueMetrics({ kpis }: { kpis: Queue["kpis"] }) {
  const metrics = [
    { label: "Bookings", value: number.format(kpis.total), detail: `${number.format(kpis.assessed)} assessed`, icon: Shield },
    { label: "Manual reviews", value: number.format(kpis.manualReview), detail: `${number.format(kpis.openHolds)} open · ${number.format(kpis.overdueHolds)} overdue`, icon: TriangleAlert },
    { label: "Average assessment latency", value: kpis.averageAssessmentLatencyMs == null ? "—" : `${number.format(kpis.averageAssessmentLatencyMs)} ms`, detail: "Booking receipt to assessment", icon: Timer },
    { label: "Latest booking", value: kpis.freshnessSeconds == null ? "—" : `${number.format(kpis.freshnessSeconds)} s old`, detail: kpis.latestReceivedAt ? formatDate(kpis.latestReceivedAt) : "No bookings received", icon: Clock3 },
  ];
  return <dl className="grid grid-cols-2 gap-3 lg:grid-cols-4">
    {metrics.map(({ label, value, detail, icon: Icon }) => <div className={`${panel} p-4`} key={label}>
      <dt className="flex items-start gap-2 text-xs font-semibold text-slate-600"><Icon className="size-4 shrink-0 text-indigo-600" aria-hidden="true" />{label}</dt>
      <dd className="mt-2 break-words text-xl font-semibold tracking-tight">{value}</dd>
      <dd className="mt-1 text-xs text-slate-500">{detail}</dd>
    </div>)}
  </dl>;
}

function TransactionRow({ row }: { row: Transaction }) {
  return <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
    <div className="min-w-0">
      <div className="text-sm font-semibold">{row.product ? products[row.product] ?? row.product : "Booking"}</div>
      <div className="mt-1 text-xs text-slate-500"><time dateTime={row.receivedAt}>{formatDate(row.receivedAt)}</time> · {row.eventId.slice(0, 8)}</div>
    </div>
    <div className="flex flex-wrap gap-1.5">
      <span className="rounded-full bg-indigo-50 px-2 py-1 text-xs font-semibold text-indigo-800">{row.outcome ? outcomes[row.outcome] ?? row.outcome : "Awaiting assessment"}</span>
      {row.hold && <span className={`rounded-full px-2 py-1 text-xs font-semibold ${row.hold.overdue ? "bg-amber-100 text-amber-900" : "bg-slate-100 text-slate-700"}`}>{row.hold.overdue ? "Overdue" : holdStates[row.hold.state]}</span>}
    </div>
  </div>;
}

function HoldDetail({ hold, detail, operation, resolution, setResolution, runAction }: {
  hold: Hold; detail: Detail; operation: string;
  resolution: string; setResolution: (value: string) => void;
  runAction: (action: "claim" | "resolve" | "escalate") => Promise<void>;
}) {
  const analyst = detail.actor.role === "analyst";
  const canClaim = analyst && (hold.state === "OPEN" || hold.state === "ESCALATED") && !hold.overdue;
  const canResolve = analyst && hold.state === "CLAIMED" && hold.assignedActor === detail.actor.actorId && !hold.overdue;
  const canEscalate = analyst && (hold.state === "OPEN" || hold.state === "CLAIMED") && hold.overdue;

  return <div className="space-y-3 border-t border-slate-100 pt-4">
    <h3 className="text-sm font-semibold">Manual review</h3>
    <p className="text-sm text-slate-700">{holdStates[hold.state]} · Due <time dateTime={hold.dueAt}>{formatDate(hold.dueAt)}</time>{hold.overdue && " · Overdue"}</p>
    {hold.assignedActor && <p className="break-all text-xs text-slate-500">Assigned to {hold.assignedActor}</p>}
    {hold.resolution && <p className="text-xs text-slate-500">Disposition {hold.resolution}</p>}
    <div className="flex flex-wrap items-end gap-2">
      {canClaim && <button type="button" className={button} disabled={Boolean(operation)} onClick={() => void runAction("claim")}>Claim review</button>}
      {canResolve && <>
        <Filter label="Disposition" values={{ cleared: "Cleared", rejected: "Rejected" }} value={resolution} onChange={setResolution} optional={false} disabled={Boolean(operation)} />
        <button type="button" className={button} disabled={Boolean(operation)} onClick={() => void runAction("resolve")}>Resolve review</button>
      </>}
      {canEscalate && <button type="button" className={button} disabled={Boolean(operation)} onClick={() => void runAction("escalate")}>Escalate overdue review</button>}
    </div>
  </div>;
}
