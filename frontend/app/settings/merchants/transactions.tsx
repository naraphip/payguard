"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { merchantDemoRequest, merchantJevStatus, merchantOutcomeLabels, merchantPolicyProducts, policyActionLabels } from "../../merchant-demo";
import type { DemoMerchant, DemoTransaction } from "../../merchant-demo";
import { PolicyDecisionDetails } from "../../merchant-policy-decision";
import { MerchantJevEvidence } from "../../merchant-jev-evidence";
import { managementError } from "./management-access";
import { merchantDate, merchantDetailsHref } from "./management-contracts";
import type { MerchantLedgerPage } from "./management-contracts";

const button = "rounded-lg border border-line bg-white px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-60";
const count = new Intl.NumberFormat("en-US");
const limit = 20;
const reviewStates = { NONE: "No review", NEEDS_DECISION: "Needs decision", RESOLVED: "Resolved" } as const;
const finalFilters = { ALLOW: "Allow", CHALLENGE: "Challenge", BLOCK: "Block", ...merchantOutcomeLabels };
const products: Record<string, string> = merchantPolicyProducts;
const emptyFilters = { q: "", outcome: "", reviewState: "", from: "", to: "" };
type Filters = typeof emptyFilters;

function decisionLabel(item: DemoTransaction) {
  const decision = item.policyDecision?.finalDecision.action;
  return decision ? policyActionLabels[decision] ?? decision : merchantOutcomeLabels[item.outcome];
}

export function MerchantTransactions({ token, merchant, canDecide, initialTransaction }: {
  token: string; merchant: DemoMerchant; canDecide: boolean; initialTransaction?: string;
}) {
  const [result, setResult] = useState<MerchantLedgerPage | null>(null);
  const [detail, setDetail] = useState<DemoTransaction | null>(null);
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [applied, setApplied] = useState<Filters>(emptyFilters);
  const [page, setPage] = useState(1);
  const [refreshKey, refresh] = useState(0);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [detailError, setDetailError] = useState("");
  const [filterError, setFilterError] = useState("");
  const [updated, setUpdated] = useState(false);
  const basePath = `/${encodeURIComponent(merchant.id)}/transactions`;

  useEffect(() => {
    const controller = new AbortController();
    const { signal } = controller;
    const query = new URLSearchParams({ page: String(page), limit: String(limit) });
    for (const [key, value] of Object.entries(applied)) if (value) query.set(key, value);
    setLoading(true);
    setError("");
    void merchantDemoRequest<MerchantLedgerPage>(`${basePath}?${query}`, token, { signal })
      .then(page => { if (!signal.aborted) setResult(page); })
      .catch(error => {
        if (!signal.aborted) {
          setResult(null);
          setError(managementError(error));
        }
      }).finally(() => { if (!signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [applied, basePath, page, refreshKey, token]);

  useEffect(() => {
    setDetail(null);
    setDetailError("");
    setUpdated(false);
    setDetailLoading(false);
    if (!initialTransaction) return;
    const controller = new AbortController();
    const { signal } = controller;
    setDetailLoading(true);
    void merchantDemoRequest<{ transaction: DemoTransaction }>(`${basePath}/${encodeURIComponent(initialTransaction)}`, token, { signal })
      .then(value => { if (!signal.aborted) setDetail(value.transaction); })
      .catch(error => { if (!signal.aborted) setDetailError(managementError(error)); })
      .finally(() => { if (!signal.aborted) setDetailLoading(false); });
    return () => controller.abort();
  }, [basePath, initialTransaction, token]);

  function apply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFilterError("");
    if (filters.from && filters.to && filters.from > filters.to) {
      setFilterError("The start date must be before or equal to the end date.");
      return;
    }
    setPage(1);
    setApplied({ ...filters, q: filters.q.trim() });
  }

  async function decide(row: DemoTransaction, decision: "cleared" | "rejected") {
    if (!canDecide || saving) return;
    setSaving(true);
    setDetailError("");
    setUpdated(false);
    try {
      const result = await merchantDemoRequest<{ transaction: DemoTransaction }>(`${basePath}/${row.id}/decision`, token, {
        method: "POST", body: JSON.stringify({ decision }),
      });
      setDetail(current => current?.id === result.transaction.id ? result.transaction : current);
      setUpdated(true);
      refresh(key => key + 1);
    } catch (error) {
      setDetailError(managementError(error));
    } finally {
      setSaving(false);
    }
  }

  const transactions = result?.transactions ?? [];
  const pageCount = result ? Math.max(1, Math.ceil(result.total / result.limit)) : 1;
  const filtered = Object.values(applied).some(Boolean);
  const selected = detail?.id === initialTransaction ? detail : null;

  return <section className="min-w-0 space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby="merchant-transactions">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h2 id="merchant-transactions" className="text-lg font-semibold">Transactions</h2>
      <button className={button} type="button" disabled={loading || saving} onClick={() => refresh(key => key + 1)}>Refresh</button>
    </div>
    <form className="grid grid-cols-1 items-end gap-2 sm:grid-cols-2 xl:grid-cols-3" aria-label="Transaction filters" onSubmit={apply}>
      <label className="min-w-0 text-xs font-semibold">Transaction ID
        <input className="control mt-1" type="search" value={filters.q} maxLength={36} placeholder="ID or prefix"
          onChange={event => setFilters({ ...filters, q: event.target.value })} />
      </label>
      <label className="min-w-0 text-xs font-semibold">Outcome
        <select className="control mt-1" value={filters.outcome} onChange={event => setFilters({ ...filters, outcome: event.target.value })}>
          <option value="">All outcomes</option>
          {Object.entries(finalFilters).map(([id, label]) => <option key={id} value={id}>{label}</option>)}
        </select>
      </label>
      <label className="min-w-0 text-xs font-semibold">Review state
        <select className="control mt-1" value={filters.reviewState} onChange={event => setFilters({ ...filters, reviewState: event.target.value })}>
          <option value="">All states</option>
          {Object.entries(reviewStates).map(([id, label]) => <option key={id} value={id}>{label}</option>)}
        </select>
      </label>
      <label className="min-w-0 text-xs font-semibold">From date (UTC)
        <input className="control mt-1 min-w-0" type="date" value={filters.from} onChange={event => setFilters({ ...filters, from: event.target.value })} />
      </label>
      <label className="min-w-0 text-xs font-semibold">To date (UTC)
        <input className="control mt-1 min-w-0" type="date" value={filters.to} onChange={event => setFilters({ ...filters, to: event.target.value })} />
      </label>
      <div className="flex flex-wrap gap-2">
        <button className={button} type="submit">Apply filters</button>
        {filtered && <button className={button} type="button" onClick={() => {
          setFilters(emptyFilters); setApplied(emptyFilters); setPage(1); setFilterError("");
        }}>Clear filters</button>}
      </div>
      {filterError && <p role="alert" className="text-sm text-red-800">{filterError}</p>}
    </form>
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
    {loading && <p role="status" className="text-sm text-muted">Loading transactions…</p>}
    {!loading && !error && !transactions.length && <p className="text-sm text-muted">
      {filtered ? "No transactions match these filters." : canDecide ? "No transactions yet. Submit a test in Mock Checkout or Simulation Lab." : "No transactions yet."}
    </p>}
    {!!transactions.length && <ul className="divide-y divide-line rounded-lg border border-line">
      {transactions.map(item => <li key={item.id}>
        <Link href={merchantDetailsHref(merchant.id, "transactions", item.id)} aria-current={item.id === initialTransaction ? "page" : undefined}
          className="grid w-full min-w-0 gap-1 p-3 text-left hover:bg-paper aria-[current=page]:bg-soft sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-center sm:gap-4">
          <span className="min-w-0">
            <span className="block text-sm font-semibold wrap-anywhere">{item.product ? products[item.product] ?? item.product : "Transaction"}</span>
            <span className="block font-mono text-[11px] text-muted wrap-anywhere">{item.id}</span>
            <span className="block text-xs text-muted">{item.source === "checkout" ? "Mock Checkout" : "Simulation Lab"} · {merchantJevStatus(item)} · {merchantDate(item.createdAt)}</span>
          </span>
          <span className="text-sm font-medium">{decisionLabel(item)}</span>
          <span className="text-xs text-muted">{reviewStates[item.reviewState]}</span>
        </Link>
      </li>)}
    </ul>}
    {result && result.total > limit && <nav aria-label="Transaction pages" className="flex flex-wrap items-center justify-between gap-2">
      <p className="text-sm text-muted">{count.format(result.total)} transactions · Page {result.page} of {pageCount}</p>
      <div className="flex gap-2">
        <button className={button} type="button" disabled={loading || page <= 1} onClick={() => setPage(value => value - 1)}>Previous</button>
        <button className={button} type="button" disabled={loading || page >= pageCount} onClick={() => setPage(value => value + 1)}>Next</button>
      </div>
    </nav>}
    {detailLoading && <p role="status" className="text-sm text-muted">Opening transaction details…</p>}
    {detailError && <p role="alert" className="text-sm text-red-800">{detailError}</p>}
    {selected && <TransactionDetail selected={selected} canDecide={canDecide} saving={saving} updated={updated} onDecide={decide} />}
  </section>;
}

function TransactionDetail({ selected, canDecide, saving, updated, onDecide }: {
  selected: DemoTransaction; canDecide: boolean; saving: boolean; updated: boolean;
  onDecide: (transaction: DemoTransaction, decision: "cleared" | "rejected") => Promise<void>;
}) {
  return <section className="min-w-0 space-y-3 rounded-lg border border-line bg-paper p-3 sm:p-4" aria-labelledby="merchant-transaction-detail">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0">
        <h3 id="merchant-transaction-detail" className="font-semibold">Transaction result</h3>
        <p className="mt-1 font-mono text-xs text-muted wrap-anywhere">{selected.id}</p>
      </div>
      <span className="rounded-full border border-line bg-white px-2.5 py-1 text-xs font-semibold">{decisionLabel(selected)}</span>
    </div>
    <dl className="grid gap-3 text-sm sm:grid-cols-2">
      <div><dt className="font-medium">Merchant ID</dt><dd className="text-muted wrap-anywhere">{selected.merchantId}</dd></div>
      <div><dt className="font-medium">Time and source</dt><dd className="text-muted">{merchantDate(selected.createdAt)} · {selected.source === "checkout" ? "Mock Checkout" : "Simulation Lab"}</dd></div>
      <div><dt className="font-medium">Product</dt><dd className="text-muted">{selected.product ? products[selected.product] ?? selected.product : "Unavailable"}</dd></div>
      <div><dt className="font-medium">Booking amount</dt><dd className="text-muted">{selected.amountMinor == null ? "Unavailable" : `${count.format(selected.amountMinor)} minor units`}</dd></div>
      {selected.scenarioId && selected.scenarioName
        ? <div><dt className="font-medium">Saved scenario</dt><dd className="text-muted wrap-anywhere">
          {selected.scenarioName}{selected.scenarioVersion && ` v${selected.scenarioVersion}`} · {selected.ruleVersion}
        </dd></div>
        : <div><dt className="font-medium">Platform ruleset</dt><dd className="text-muted wrap-anywhere">{selected.ruleVersion}</dd></div>}
      <div><dt className="font-medium">Review state</dt><dd className="text-muted">{reviewStates[selected.reviewState]}</dd></div>
      <div><dt className="font-medium">Risk result</dt><dd className="text-muted">{merchantOutcomeLabels[selected.outcome]}</dd></div>
      <MerchantJevEvidence transaction={selected} />
      {selected.jevRuleVersion && <div><dt className="font-medium">Merchant Jev rules</dt><dd className="text-muted">{selected.jevRuleVersion}</dd></div>}
      {selected.thresholdMinor != null && <div><dt className="font-medium">Saved review threshold</dt><dd className="text-muted">{count.format(selected.thresholdMinor)} minor units</dd></div>}
    </dl>
    {selected.policyDecision && <PolicyDecisionDetails decision={selected.policyDecision} />}
    <p className="text-sm wrap-anywhere">{selected.reason}</p>
    {selected.jevReason && selected.jevReason !== selected.reason && <p className="text-sm wrap-anywhere"><strong>Jev assessment</strong> {selected.jevReason}</p>}
    <p className="text-sm wrap-anywhere"><strong>Rule hits</strong> {selected.ruleHits.length ? selected.ruleHits.join(", ") : "None"}</p>
    {selected.disposition && <p className="text-sm wrap-anywhere">
      <strong>Manual disposition</strong> {selected.disposition === "cleared" ? "Cleared" : "Rejected"}
      {selected.decidedBy && ` · ${selected.decidedBy}`}
      {selected.decidedAt && ` · ${merchantDate(selected.decidedAt)}`}
    </p>}
    {canDecide && selected.reviewState === "NEEDS_DECISION" && <div className="flex flex-wrap gap-2">
      <button className={button} type="button" disabled={saving} onClick={() => void onDecide(selected, "cleared")}>Clear transaction</button>
      <button className={button} type="button" disabled={saving} onClick={() => void onDecide(selected, "rejected")}>Reject transaction</button>
    </div>}
    {updated && <p role="status" className="text-sm text-emerald-800">Manual decision recorded.</p>}
  </section>;
}
