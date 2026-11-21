"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Building2, ClipboardList } from "lucide-react";
import { loadLocalMerchantDirectory, merchantDemoRequest } from "../merchant-demo";
import type { DemoDirectory, DemoTransaction } from "../merchant-demo";
import { TransactionStatusBadge, transactionStatus, transactionStatuses } from "../merchant-transaction-status";
import { useWorkspaceView } from "../workspace-view";
import { useRuleCredential } from "../settings/rule-composer";
import { merchantDetailsHref } from "../settings/merchants/management-contracts";

type DashboardData = { merchantCount: number; transactions: (DemoTransaction & { merchantName: string })[] };

export function DashboardOverview() {
  const merchantView = useWorkspaceView() === "merchant";
  const token = useRuleCredential(merchantView ? "MERCHANT" : "PLATFORM");
  return <ConnectedOverview key={token} token={token} merchantView={merchantView} />;
}

function ConnectedOverview({ token, merchantView }: { token: string; merchantView: boolean }) {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    const { signal } = controller;
    async function load() {
      try {
        const localDirectory = merchantView || token ? null : await loadLocalMerchantDirectory();
        if (signal.aborted) return;
        const directory = localDirectory ?? (token ? await merchantDemoRequest<DemoDirectory>("", token, { signal }) : null);
        if (directory?.role !== (merchantView ? "merchant" : "admin")) return;
        const grouped = await Promise.all(directory.merchants.map(async merchant => {
          const response = await merchantDemoRequest<{ transactions: DemoTransaction[] }>(
            `/${merchant.id}/transactions`, localDirectory ? "" : token, { signal },
          );
          return response.transactions.map(item => ({ ...item, merchantName: merchant.name }));
        }));
        const transactions = grouped.flat().sort((a, b) => b.createdAt.localeCompare(a.createdAt));
        if (!signal.aborted) setData({ merchantCount: directory.merchants.length, transactions });
      } catch (cause) {
        if (!signal.aborted) setError(cause instanceof Error ? cause.message : "Dashboard could not be loaded.");
      } finally {
        if (!signal.aborted) setLoading(false);
      }
    }
    void load();
    return () => controller.abort();
  }, [attempt, merchantView, token]);

  const statusCounts = { APPROVED: 0, REJECTED: 0, REVIEW: 0 };
  for (const transaction of data?.transactions ?? []) {
    statusCounts[transactionStatus(transaction)]++;
  }
  return <main className="mx-auto max-w-[1440px] space-y-6 px-4 py-7 sm:px-7 lg:px-9">
    <header className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Fraud Overview</h1>
        <p className="mt-1.5 text-sm text-muted">Merchant transactions and review activity</p>
      </div>
      <Link href={merchantView ? "/settings/merchants?view=transactions" : "/settings/merchants"} className="inline-flex items-center gap-2 rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white">
        <Building2 size={16} aria-hidden="true" /> {merchantView ? "Transactions" : "Merchants"}
      </Link>
    </header>
    {loading && <p role="status" className="text-sm text-muted">Loading merchant activity…</p>}
    {error && <div role="alert" className="rounded-lg border border-line bg-white p-4 text-sm">
      <p>{error}</p>
      <button type="button" className="mt-2 font-semibold text-accent" onClick={() => {
        setError("");
        setLoading(true);
        setAttempt(value => value + 1);
      }}>Retry</button>
    </div>}
    {!loading && !error && !data && <p className="text-sm text-muted">{merchantView ? "Open Transactions to connect with your merchant access token." : "Open Merchants to connect as an administrator."}</p>}
    {data && <>
      <dl id="risk-trends" className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
        {[
          ...(!merchantView ? [["Merchants", data.merchantCount]] : []),
          ["Transactions", data.transactions.length],
        ].map(([label, value]) => <div key={label} className="rounded-xl border border-line bg-white p-4">
          <dt className="text-sm text-muted">{label}</dt>
          <dd className="mt-2 text-2xl font-semibold tabular-nums">{value}</dd>
        </div>)}
        {transactionStatuses.map(status => <div key={status} className="rounded-xl border border-line bg-white p-4">
          <dt><TransactionStatusBadge status={status} /></dt>
          <dd className="mt-2 text-2xl font-semibold tabular-nums">{statusCounts[status]}</dd>
        </div>)}
      </dl>
      <section className="rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby="recent-transactions">
        <h2 id="recent-transactions" className="flex items-center gap-2 text-lg font-semibold"><ClipboardList size={18} aria-hidden="true" />Recent transactions</h2>
        {data.transactions.length === 0 ? <p className="mt-3 text-sm text-muted">No transactions yet.</p> :
          <ul className="mt-3 divide-y divide-line">
          {data.transactions.slice(0, 10).map(item => <li key={`${item.merchantId}:${item.id}`}>
            <Link href={merchantDetailsHref(item.merchantId, "transactions", item.id)}
              className="flex flex-wrap items-center justify-between gap-2 py-3 hover:text-accent">
              <span className="text-sm"><strong>{item.merchantName}</strong> · {item.source === "checkout" ? "Mock Checkout" : "Simulation Lab"}</span>
              <TransactionStatusBadge status={transactionStatus(item)} />
            </Link>
          </li>)}
          </ul>}
      </section>
    </>}
  </main>;
}
