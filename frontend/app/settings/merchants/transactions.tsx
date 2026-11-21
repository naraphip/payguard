"use client";

import { useEffect, useState } from "react";
import { merchantDemoRequest } from "../../merchant-demo";
import type { DemoMerchant, DemoTransaction } from "../../merchant-demo";

const button = "rounded-lg border border-line bg-white px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-60 aria-pressed:border-accent aria-pressed:bg-soft aria-pressed:text-accent";
const count = new Intl.NumberFormat("en-US");
const time = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" });
const outcomes: Record<DemoTransaction["outcome"], string> = {
  LOW_RISK: "Low risk",
  REVIEW: "Review",
  UNRESOLVED: "Unresolved",
};
const reviewStates: Record<DemoTransaction["reviewState"], string> = {
  NONE: "No review",
  NEEDS_DECISION: "Needs decision",
  RESOLVED: "Resolved",
};

function formatTime(value: string) {
  return time.format(new Date(value));
}

export function MerchantTransactions({ token, merchant, canDecide }: { token: string; merchant: DemoMerchant; canDecide: boolean }) {
  const [transactions, setTransactions] = useState<DemoTransaction[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [needsDecision, setNeedsDecision] = useState(false);
  const [refreshKey, refresh] = useState(0);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [updated, setUpdated] = useState(false);
  const basePath = `/${merchant.id}/transactions`;

  useEffect(() => {
    let ignore = false;
    setLoading(true);
    setError("");
    merchantDemoRequest<{ transactions: DemoTransaction[] }>(basePath, token).then((result) => {
      if (ignore) return;
      setTransactions(result.transactions);
      const requestedId = new URLSearchParams(window.location.search).get("transaction");
      setSelectedId((current) => current || requestedId || "");
    }).catch((failure: unknown) => {
      if (!ignore) setError(failure instanceof Error ? failure.message : "Could not load transactions.");
    }).finally(() => {
      if (!ignore) setLoading(false);
    });
    return () => { ignore = true; };
  }, [basePath, token, refreshKey]);

  const visible = transactions.filter((item) => !needsDecision || item.reviewState === "NEEDS_DECISION");
  const selected = transactions.find(({ id }) => id === selectedId);

  async function decide(transaction: DemoTransaction, decision: "cleared" | "rejected") {
    if (!canDecide) return;
    setSaving(true);
    setError("");
    setUpdated(false);
    try {
      const result = await merchantDemoRequest<{ transaction: DemoTransaction }>(`${basePath}/${transaction.id}/decision`, token, {
        method: "POST",
        body: JSON.stringify({ decision }),
      });
      setTransactions((current) => current.map((item) => item.id === result.transaction.id ? result.transaction : item));
      setUpdated(true);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Could not record the decision.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="min-w-0 space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby="merchant-transactions">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="merchant-transactions" className="text-lg font-semibold">Transactions</h2>
        <div className="flex flex-wrap gap-2">
          <button className={button} type="button" aria-pressed={!needsDecision} onClick={() => setNeedsDecision(false)}>All transactions</button>
          <button className={button} type="button" aria-pressed={needsDecision} onClick={() => setNeedsDecision(true)}>Needs decision</button>
          <button className={button} type="button" disabled={loading || saving} onClick={() => refresh((key) => key + 1)}>Refresh</button>
        </div>
      </div>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      {loading && <p role="status" className="text-sm text-slate-600">Loading transactions…</p>}
      {!loading && !visible.length && <p className="text-sm text-slate-600">{needsDecision
        ? "No transactions need a decision."
        : canDecide ? "No transactions yet. Submit a mock checkout or simulation with one of this merchant’s scenarios." : "No transactions yet."}</p>}
      {!!visible.length && (
        <ul className="divide-y divide-line rounded-lg border border-line">
          {visible.map((item) => (
            <li key={item.id}>
              <button
                className="grid w-full gap-1 p-3 text-left hover:bg-paper aria-pressed:bg-soft sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-center sm:gap-4"
                type="button"
                aria-pressed={item.id === selectedId}
                onClick={() => { setSelectedId(item.id); setUpdated(false); }}
              >
                <span className="min-w-0">
                  <span className="block break-words text-sm font-semibold">{item.scenarioName}</span>
                  <span className="block text-xs text-slate-600">{item.source === "checkout" ? "Mock checkout" : "Simulation Lab"} · {formatTime(item.createdAt)}</span>
                </span>
                <span className="text-sm font-medium">{outcomes[item.outcome]}</span>
                <span className="text-xs text-slate-600">{reviewStates[item.reviewState]}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {selected && (
        <section className="space-y-3 rounded-lg border border-line bg-paper p-3 sm:p-4" aria-labelledby="merchant-transaction-detail">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <h3 id="merchant-transaction-detail" className="font-semibold">Transaction result</h3>
              <p className="mt-1 break-all font-mono text-xs text-slate-600">{selected.id}</p>
            </div>
            <span className="rounded-full border border-line bg-white px-2.5 py-1 text-xs font-semibold">Mock Jev</span>
          </div>
          <dl className="grid gap-3 text-sm sm:grid-cols-2">
            <div><dt className="font-medium">Scenario and rules</dt><dd className="break-words text-slate-600">{selected.scenarioName} v{selected.scenarioVersion} · {selected.ruleVersion}</dd></div>
            <div><dt className="font-medium">Booking amount</dt><dd className="text-slate-600">{selected.amountMinor == null ? "Unavailable" : `${count.format(selected.amountMinor)} minor units`}</dd></div>
            <div><dt className="font-medium">Automated result</dt><dd className="text-slate-600">{outcomes[selected.outcome]}</dd></div>
            <div><dt className="font-medium">Review state</dt><dd className="text-slate-600">{reviewStates[selected.reviewState]}</dd></div>
            <div><dt className="font-medium">Jev response</dt><dd className="text-slate-600">{selected.jevStatus === "TIMEOUT" ? "Simulated timeout" : "Mock response"} · {selected.jevVersion}</dd></div>
            {selected.jevRuleVersion && <div><dt className="font-medium">Merchant Jev rules</dt><dd className="text-slate-600">{selected.jevRuleVersion}</dd></div>}
            <div><dt className="font-medium">Review threshold</dt><dd className="text-slate-600">{count.format(selected.thresholdMinor)} minor units</dd></div>
          </dl>
          <p className="break-words text-sm">{selected.reason}</p>
          <p className="break-words text-sm"><strong>Rule hits</strong> {selected.ruleHits.length ? selected.ruleHits.join(", ") : "None"}</p>
          {selected.disposition && <p className="break-words text-sm">
            <strong>Manual disposition</strong> {selected.disposition === "cleared" ? "Cleared" : "Rejected"}
            {selected.decidedBy && ` · ${selected.decidedBy}`}
            {selected.decidedAt && ` · ${formatTime(selected.decidedAt)}`}
          </p>}
          {canDecide && selected.reviewState === "NEEDS_DECISION" && (
            <div className="flex flex-wrap gap-2">
              <button className={button} type="button" disabled={saving} onClick={() => void decide(selected, "cleared")}>Clear transaction</button>
              <button className={button} type="button" disabled={saving} onClick={() => void decide(selected, "rejected")}>Reject transaction</button>
            </div>
          )}
          {updated && <p role="status" className="text-sm text-emerald-700">Manual decision recorded.</p>}
        </section>
      )}
    </section>
  );
}
