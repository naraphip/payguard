"use client";

import Link from "next/link";
import { Check, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { merchantDemoRequest, merchantPolicyProducts } from "../../merchant-demo";
import type { DemoMerchant, DemoTransaction } from "../../merchant-demo";
import { MerchantDecisionEvidence } from "../../merchant-decision-evidence";
import { BookingComparisonDetails } from "../../merchant-booking-comparison";
import { RiskScoreDetails, riskScoreLabel } from "../../merchant-risk-score";
import { TransactionDisposition, TransactionStatusBadge, transactionReviewCause, transactionStatus, transactionStatusLabel, transactionStatuses } from "../../merchant-transaction-status";
import { managementError } from "./management-access";
import { merchantDate, merchantDetailsHref } from "./management-contracts";
import type { MerchantLedgerPage } from "./management-contracts";

const button = "rounded-lg border border-line bg-white px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-60";
const decisionButton = "inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold text-white disabled:opacity-60";
const count = new Intl.NumberFormat("en-US");
const limit = 20;
const products: Record<string, string> = merchantPolicyProducts;
const emptyFilters = { q: "", status: "", from: "", to: "" };
type Filters = typeof emptyFilters;

export function MerchantTransactions({ token, merchant, canDecide, initialTransaction, onDecision }: {
  token: string; merchant: DemoMerchant; canDecide: boolean; initialTransaction?: string; onDecision?: () => void;
}) {
  const [result, setResult] = useState<MerchantLedgerPage | null>(null);
  const [detail, setDetail] = useState<DemoTransaction | null>(null);
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [applied, setApplied] = useState<Filters>(emptyFilters);
  const [page, setPage] = useState(1);
  const [refreshKey, refresh] = useState(0);
  const [detailRefreshKey, refreshDetail] = useState(0);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [detailError, setDetailError] = useState("");
  const [filterError, setFilterError] = useState("");
  const [updatedTransactionId, setUpdatedTransactionId] = useState<string>();
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
    setUpdatedTransactionId(undefined);
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
  }, [basePath, detailRefreshKey, initialTransaction, token]);

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
    if (!canDecide || saving || transactionStatus(row) !== "REVIEW") return;
    setSaving(true);
    setDetailError("");
    setUpdatedTransactionId(undefined);
    try {
      const { transaction } = await merchantDemoRequest<{ transaction: DemoTransaction }>(`${basePath}/${row.id}/decision`, token, {
        method: "POST", body: JSON.stringify({ decision }),
      });
      setDetail(current => current?.id === transaction.id ? transaction : current);
      setResult(current => current && { ...current, transactions: current.transactions.map(item => item.id === transaction.id ? transaction : item) });
      setUpdatedTransactionId(transaction.id);
      onDecision?.();
      refresh(key => key + 1);
    } catch (error) {
      setDetailError(managementError(error));
    } finally {
      setSaving(false);
    }
  }

  const transactions = result?.transactions.filter(item => !applied.status || transactionStatus(item) === applied.status) ?? [];
  const pageCount = result ? Math.max(1, Math.ceil(result.total / result.limit)) : 1;
  const filtered = Object.values(applied).some(Boolean);
  const selected = detail?.id === initialTransaction ? detail : null;

  return <section className="min-w-0 space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby="merchant-transactions">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h2 id="merchant-transactions" className="text-lg font-semibold">Transactions</h2>
      <button className={button} type="button" disabled={loading || detailLoading || saving} onClick={() => {
        refresh(key => key + 1);
        refreshDetail(key => key + 1);
      }}>Refresh</button>
    </div>
    <form className="grid grid-cols-1 items-end gap-2 sm:grid-cols-2 xl:grid-cols-3" aria-label="Transaction filters" onSubmit={apply}>
      <label className="min-w-0 text-xs font-semibold">Transaction ID
        <input className="control mt-1" type="search" value={filters.q} maxLength={36} placeholder="ID or prefix"
          onChange={event => setFilters({ ...filters, q: event.target.value })} />
      </label>
      <label className="min-w-0 text-xs font-semibold">Status
        <select className="control mt-1" value={filters.status} onChange={event => setFilters({ ...filters, status: event.target.value })}>
          <option value="">All statuses</option>
          {transactionStatuses.map(status => <option key={status} value={status}>{transactionStatusLabel(status)}</option>)}
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
          className="grid w-full min-w-0 gap-1 p-3 text-left hover:bg-paper aria-[current=page]:bg-soft sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-4">
          <span className="min-w-0">
            <span className="block text-sm font-semibold wrap-anywhere">{item.product ? products[item.product] ?? item.product : "Transaction"}</span>
            <span className="block font-mono text-[11px] text-muted wrap-anywhere">{item.id}</span>
            <span className="block text-xs text-muted">{item.source === "checkout" ? "Mock Checkout" : "Simulation Lab"} · {merchantDate(item.createdAt)}</span>
          </span>
          <span className="space-y-1 sm:text-right">
            <TransactionStatusBadge status={transactionStatus(item)} />
            <span className="block text-xs text-muted tabular-nums">Risk score · {riskScoreLabel(item.policyDecision?.riskScore)}</span>
          </span>
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
    {selected && <TransactionDetail selected={selected} canDecide={canDecide} saving={saving}
      updated={updatedTransactionId === selected.id} onDecide={decide} />}
  </section>;
}

function TransactionDetail({ selected, canDecide, saving, updated, onDecide }: {
  selected: DemoTransaction; canDecide: boolean; saving: boolean; updated: boolean;
  onDecide: (transaction: DemoTransaction, decision: "cleared" | "rejected") => Promise<void>;
}) {
  const status = transactionStatus(selected);
  const resultHeading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    if (updated) resultHeading.current?.focus();
  }, [updated]);

  return <section className="min-w-0 space-y-3 rounded-lg border border-line bg-paper p-3 sm:p-4" aria-labelledby="merchant-transaction-detail">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0">
        <h3 id="merchant-transaction-detail" tabIndex={-1} ref={resultHeading} className="font-semibold">Transaction result</h3>
        <p className="mt-1 font-mono text-xs text-muted wrap-anywhere">{selected.id}</p>
      </div>
      <div role="status" aria-atomic="true" className="space-y-1 sm:text-right">
        <TransactionStatusBadge status={status} />
        <TransactionDisposition transaction={selected} />
      </div>
    </div>
    {status === "REVIEW" && <div className="space-y-2.5 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950">
      <p>{transactionReviewCause(selected)} A manual decision is required.</p>
      {canDecide && <div className="flex flex-wrap gap-2">
        <button className={`${decisionButton} bg-emerald-700 hover:bg-emerald-800`} type="button" disabled={saving}
          onClick={() => void onDecide(selected, "cleared")}><Check size={16} aria-hidden="true" />Approve</button>
        <button className={`${decisionButton} bg-red-700 hover:bg-red-800`} type="button" disabled={saving}
          onClick={() => void onDecide(selected, "rejected")}><X size={16} aria-hidden="true" />Reject</button>
      </div>}
    </div>}
    <dl className="grid gap-3 text-sm sm:grid-cols-2">
      <div><dt className="font-medium">Merchant ID</dt><dd className="text-muted wrap-anywhere">{selected.merchantId}</dd></div>
      <div><dt className="font-medium">Time and source</dt><dd className="text-muted">{merchantDate(selected.createdAt)} · {selected.source === "checkout" ? "Mock Checkout" : "Simulation Lab"}</dd></div>
      <div><dt className="font-medium">Product</dt><dd className="text-muted">{selected.product ? products[selected.product] ?? selected.product : "Unavailable"}</dd></div>
      <div><dt className="font-medium">Booking amount</dt><dd className="text-muted">{selected.amountMinor == null ? "Unavailable" : `${count.format(selected.amountMinor)} minor units`}</dd></div>
    </dl>
    <RiskScoreDetails score={selected.policyDecision?.riskScore} />
    {selected.policyDecision?.bookingComparison && <BookingComparisonDetails comparison={selected.policyDecision.bookingComparison} />}
    <MerchantDecisionEvidence transaction={selected} />
  </section>;
}
