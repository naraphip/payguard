"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Building2, ClipboardList } from "lucide-react";
import { getMerchantDemoToken, loadLocalMerchantDirectory, merchantDemoRequest } from "../merchant-demo";
import type { DemoDirectory, DemoTransaction } from "../merchant-demo";

type DashboardData = { merchantCount: number; transactions: (DemoTransaction & { merchantName: string })[] };
const outcomeLabels = { LOW_RISK: "Low risk", REVIEW: "Review", UNRESOLVED: "Unresolved" };

export function DashboardOverview() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const token = getMerchantDemoToken();
        const localDirectory = await loadLocalMerchantDirectory();
        const directory = localDirectory ?? (token ? await merchantDemoRequest<DemoDirectory>("", token) : null);
        if (directory?.role !== "admin") return;
        const grouped = await Promise.all(directory.merchants.map(async merchant => {
          const response = await merchantDemoRequest<{ transactions: DemoTransaction[] }>(
            `/${merchant.id}/transactions`, localDirectory ? "" : token,
          );
          return response.transactions.map(item => ({ ...item, merchantName: merchant.name }));
        }));
        const transactions = grouped.flat().sort((a, b) => b.createdAt.localeCompare(a.createdAt));
        if (active) setData({ merchantCount: directory.merchants.length, transactions });
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : "Dashboard could not be loaded.");
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    return () => { active = false; };
  }, [attempt]);

  const pending = data?.transactions.filter(transaction => transaction.reviewState === "NEEDS_DECISION").length;
  return <main className="mx-auto max-w-[1440px] space-y-6 px-4 py-7 sm:px-7 lg:px-9">
    <header className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Fraud Overview</h1>
        <p className="mt-1.5 text-sm text-muted">Merchant transactions and review activity</p>
      </div>
      <Link href="/settings/merchants" className="inline-flex items-center gap-2 rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white">
        <Building2 size={16} aria-hidden="true" /> Merchants
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
    {!loading && !error && !data && <p className="text-sm text-muted">Open Merchants to connect as an administrator.</p>}
    {data && <>
      <dl id="risk-trends" className="grid gap-3 sm:grid-cols-3">
        {[
          ["Merchants", data.merchantCount],
          ["Recent transactions", data.transactions.length],
          ["Needs decision", pending ?? 0],
        ].map(([label, value]) => <div key={label} className="rounded-xl border border-line bg-white p-4">
          <dt className="text-sm text-muted">{label}</dt>
          <dd className="mt-2 text-2xl font-semibold tabular-nums">{value}</dd>
        </div>)}
      </dl>
      <section className="rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby="recent-transactions">
        <h2 id="recent-transactions" className="flex items-center gap-2 text-lg font-semibold"><ClipboardList size={18} aria-hidden="true" />Recent transactions</h2>
        {data.transactions.length === 0 ? <p className="mt-3 text-sm text-muted">No transactions yet.</p> :
          <ul className="mt-3 divide-y divide-line">
          {data.transactions.slice(0, 10).map(item => <li key={`${item.merchantId}:${item.id}`}>
            <Link href={`/settings/merchants?${new URLSearchParams({ merchant: item.merchantId, transaction: item.id, view: "transactions" })}`}
              className="flex flex-wrap items-center justify-between gap-2 py-3 hover:text-accent">
              <span className="text-sm"><strong>{item.merchantName}</strong> · {item.scenarioName}</span>
              <span className="text-sm text-muted">{outcomeLabels[item.outcome]}</span>
            </Link>
          </li>)}
          </ul>}
      </section>
    </>}
  </main>;
}
