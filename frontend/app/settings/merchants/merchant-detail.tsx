"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { MerchantDemoError, merchantDemoRequest, merchantJevStatus, merchantOutcomeLabels, merchantPolicyProducts, policyActionLabels } from "../../merchant-demo";
import type { DemoMerchant, DemoTransaction } from "../../merchant-demo";
import { useWorkspaceView } from "../../workspace-view";
import { JevRulesPanel } from "../jev-rules/panel";
import { RuleAccess, RulePreview, useRuleCredential } from "../rule-composer";
import type { MerchantPolicyVersion } from "../rule-composer";
import { outlineButton } from "../system-rules/contracts";
import { SystemRulesDashboard } from "../system-rules/dashboard";
import { MerchantTransactions } from "./transactions";
import { MerchantWorkspace } from "./directory";
import { MerchantManagementAccess, managementError } from "./management-access";
import { merchantDate, merchantDetailsHref, merchantTabs } from "./management-contracts";
import type { MerchantAuditEvent, MerchantDetails, MerchantPolicyHistory, MerchantTab } from "./management-contracts";

const panel = "min-w-0 space-y-3 rounded-xl border border-line bg-white p-4 sm:p-5";
const link = "font-semibold text-accent hover:underline";
const products: Record<string, string> = merchantPolicyProducts;
const amountFormat = new Intl.NumberFormat("en");

function label(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

function transactionAction(transaction: DemoTransaction) {
  const action = transaction.policyDecision?.finalDecision.action;
  return action ? policyActionLabels[action] ?? action : merchantOutcomeLabels[transaction.outcome];
}

function useMerchantResource<T>(path: string, token: string, revision = 0) {
  const [response, setResponse] = useState<T | null>(null);
  const [failure, setFailure] = useState<{ error: string; status?: number }>();
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setResponse(null);
    setFailure(undefined);
    void merchantDemoRequest<T>(path, token, { signal: controller.signal }).then(value => {
      if (!controller.signal.aborted) setResponse(value);
    }).catch(error => {
      if (!controller.signal.aborted) setFailure({ error: managementError(error), status: error instanceof MerchantDemoError ? error.status : undefined });
    });
    return () => controller.abort();
  }, [path, token, retry, revision]);

  return { response, failure, reload: () => setRetry(value => value + 1) };
}

export function MerchantDetail({ merchantId, tab, transaction }: { merchantId: string; tab: MerchantTab; transaction?: string }) {
  const merchantView = useWorkspaceView() === "merchant";
  return <MerchantManagementAccess merchant={merchantView}>
    {token => <ConnectedDetail key={`${token}:${merchantId}`} token={token} merchantId={merchantId}
      tab={merchantView ? "transactions" : tab} merchantView={merchantView} transaction={transaction} />}
  </MerchantManagementAccess>;
}

function ConnectedDetail({ token, merchantId, tab, merchantView, transaction }: {
  token: string; merchantId: string; tab: MerchantTab; merchantView: boolean; transaction?: string;
}) {
  const path = `/${encodeURIComponent(merchantId)}`;
  const { response, failure, reload } = useMerchantResource<MerchantDetails>(path, token);
  const [updated, setUpdated] = useState<DemoMerchant>();
  const router = useRouter();
  if (failure) return <section className={panel}>
    <h1 className="text-2xl font-semibold">
      {failure.status === 404 ? "Merchant not found" : failure.status === 403 || failure.status === 401 ? "Access denied" : "Could not load merchant"}
    </h1>
    <p role="alert" className="text-sm text-muted">{failure.error}</p>
    <div className="flex flex-wrap gap-3 text-sm">
      {!failure.status && <button type="button" className={outlineButton} onClick={reload}>Retry</button>}
      <Link className={link} href={merchantView ? "/settings/merchants?view=transactions" : "/settings/merchants"}>Back to {merchantView ? "transactions" : "merchants"}</Link>
    </div>
  </section>;
  if (!response) return <p role="status" className="text-sm text-muted">Loading merchant details…</p>;
  const merchant = updated ?? response.merchant;
  const goBack = merchantView ? "/settings/merchants?view=transactions" : "/settings/merchants";
  const scenario = merchant.scenarios?.[0];
  const sandboxQuery = new URLSearchParams({ merchant: merchant.id });
  if (scenario && merchant.scenarios.length === 1) sandboxQuery.set("scenario", scenario.id);

  return <div className="space-y-5">
    <header className="space-y-2">
      <Link href={goBack} className={`text-sm ${link}`}>← {merchantView ? "Transactions" : "Merchants"}</Link>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h1 className="text-3xl font-semibold tracking-tight wrap-anywhere">{merchant.name}</h1>
          <p className="font-mono text-xs text-muted wrap-anywhere">{merchant.id}</p>
          {merchant.description && <p className="max-w-3xl text-sm text-muted wrap-anywhere">{merchant.description}</p>}
          {merchant.createdAt && <p className="text-xs text-muted">Created {merchantDate(merchant.createdAt)}</p>}
        </div>
        {!merchantView && <div className="flex flex-wrap items-center gap-2">
          <Link className={outlineButton} href={`/settings/mock-checkout?${sandboxQuery}`}>Mock Checkout</Link>
          <Link className={outlineButton} href={`/simulation-lab?${sandboxQuery}`}>Simulation Lab</Link>
        </div>}
      </div>
    </header>
    {!merchantView && <nav className="flex flex-wrap gap-1 rounded-lg border border-line bg-white p-1.5 text-sm" aria-label="Merchant sections">
      {Object.entries(merchantTabs).map(([id, name]) => <Link key={id} href={merchantDetailsHref(merchant.id, id as MerchantTab)}
        aria-current={id === tab ? "page" : undefined}
        className="rounded-md px-2.5 py-2 font-semibold text-muted hover:bg-paper aria-[current=page]:bg-soft aria-[current=page]:text-accent">{name}</Link>)}
    </nav>}
    {tab === "overview" && <MerchantOverview detail={response} merchant={merchant} token={token} onSave={setUpdated}
      onDelete={() => router.push("/settings/merchants")} />}
    {tab === "transactions" && <MerchantTransactions token={token} merchant={merchant} canDecide={!merchantView} initialTransaction={transaction} />}
    {tab === "rules" && <>
      <PolicyInspection key={merchant.id} path={path} token={token} />
      <section id="booking-review-rules" className={`${panel} scroll-mt-28`} aria-label="Booking review rules">
        <MerchantJevRules merchant={merchant} />
        <h3 className="font-semibold">Recent Jev assessments</h3>
        {!response.recentTransactions.length && <p className="text-sm text-muted">No recorded assessments.</p>}
        <ul className="divide-y divide-line">
          {response.recentTransactions.map(item => <li key={item.id} className="flex flex-wrap justify-between gap-2 py-2 text-sm">
            <Link href={merchantDetailsHref(merchant.id, "transactions", item.id)} className={link}>{item.scenarioName}</Link>
            <span className="wrap-anywhere">{merchantJevStatus(item)}{item.jevVersion && ` · ${item.jevVersion}`}{item.jevRuleVersion && ` · Rules ${item.jevRuleVersion}`}{item.jevAnswer && ` · ${merchantOutcomeLabels[item.jevAnswer]}`}</span>
          </li>)}
        </ul>
      </section>
    </>}
    {tab === "profiles" && <Profiles detail={response} />}
    {tab === "integrations" && <Integrations detail={response} merchant={merchant} />}
    {tab === "audit" && <MerchantAudit path={path} token={token} />}
    {tab === "platform-drafts" && <SandboxPlatformDrafts merchant={merchant} />}
  </div>;
}

function MerchantOverview({ detail, merchant, token, onSave, onDelete }: {
  detail: MerchantDetails; merchant: DemoMerchant; token: string;
  onSave: (merchant: DemoMerchant) => void; onDelete: () => void;
}) {
  const { metrics, profile, recentTransactions } = detail;
  const counts = [
    ["Transactions", metrics.totalTransactions],
    ["Allowed", metrics.allowed],
    ["Challenged", metrics.challenged],
    ["Under review", metrics.underReview],
    ["Blocked", metrics.blocked],
    ["Active merchant policies", metrics.activePolicies],
  ] as const;
  return <div className="space-y-5">
    <section aria-label="Merchant activity" className="space-y-2">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-lg font-semibold">Merchant activity</h2>
        <p className="text-xs text-muted">Transaction activity over the last {metrics.windowDays} days</p>
      </div>
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6">
        {counts.map(([name, value]) => <div className="min-w-0 rounded-xl border border-line bg-white p-3" key={name}>
          <dt className="text-xs font-medium text-muted">{name}</dt><dd className="mt-1 text-xl font-semibold tabular-nums">{value}</dd>
        </div>)}
      </dl>
    </section>
    <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
      <section className={panel} aria-labelledby="merchant-recent-transactions">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="text-lg font-semibold" id="merchant-recent-transactions">Recent Transactions</h2>
          <Link className={`text-sm ${link}`} href={merchantDetailsHref(merchant.id, "transactions")}>Open ledger</Link>
        </div>
        {!recentTransactions.length && <p className="text-sm text-muted">No transactions yet. Run a Mock Checkout or Simulation Lab scenario.</p>}
        <ul className="divide-y divide-line">
          {recentTransactions.map(item => <li key={item.id} className="flex flex-wrap items-start justify-between gap-2 py-2">
            <div className="min-w-0 text-sm">
              <Link href={merchantDetailsHref(merchant.id, "transactions", item.id)} className={`${link} wrap-anywhere`}>{item.scenarioName}</Link>
              <p className="font-mono text-[11px] text-muted wrap-anywhere">{item.id}</p>
              <p className="text-xs text-muted">
                {item.product ? products[item.product] ?? item.product : "Product unavailable"}
                {" · "}{item.amountMinor == null ? "Amount unavailable" : `${amountFormat.format(item.amountMinor)} minor units`}
              </p>
              <p className="text-xs text-muted">{merchantDate(item.createdAt)} · {item.source === "checkout" ? "Mock Checkout" : "Simulation Lab"}</p>
            </div>
            <div><p className="text-sm font-semibold">{transactionAction(item)}</p><p className="text-xs text-muted">{merchantJevStatus(item)}</p></div>
          </li>)}
        </ul>
      </section>
      <section className={panel} aria-label="Current configuration">
        <h2 className="text-lg font-semibold">Current configuration</h2>
        <dl className="space-y-2 text-sm">
          <div><dt className="font-medium">Scoring profile</dt><dd className="text-muted">{profile.scoringProfileVersion || "No active profile recorded"}</dd></div>
          <div><dt className="font-medium">Available signals</dt><dd className="text-muted wrap-anywhere">{profile.signals?.join(", ") || "Unavailable"}</dd></div>
          <div><dt className="font-medium">Sandbox scenarios</dt><dd className="text-muted">{merchant.scenarios.length}</dd></div>
        </dl>
        <Link href="/settings/system-rules" className={`inline-block text-sm ${link}`}>Open global Platform Rules</Link>
      </section>
    </div>
    <MerchantWorkspace key={merchant.id} merchant={merchant} token={token} admin onSave={onSave} onDelete={onDelete} />
  </div>;
}

function Profiles({ detail }: { detail: MerchantDetails }) {
  const { profile } = detail;
  return <section className={panel}>
    <h2 className="text-lg font-semibold">Advanced Risk Configuration</h2>
    <dl className="grid grid-cols-2 gap-3 text-sm">
      <div><dt className="font-medium">Active version</dt><dd className="text-muted">{profile.scoringProfileVersion || "No active profile recorded"}</dd></div>
      <div><dt className="font-medium">Scope</dt><dd className="text-muted">{detail.merchant.name}</dd></div>
      <div className="col-span-2"><dt className="font-medium">Available signals</dt><dd className="text-muted wrap-anywhere">{profile.signals?.join(", ") || "Unavailable"}</dd></div>
    </dl>
    <Link href="/settings/booking-assessment" className={`text-sm ${link}`}>Open legacy booking profile tools</Link>
  </section>;
}

function Integrations({ detail, merchant }: { detail: MerchantDetails; merchant: DemoMerchant }) {
  const { profile } = detail;
  return <section className={panel}>
    <h2 className="text-lg font-semibold">Integrations</h2>
    <dl className="grid gap-3 text-sm sm:grid-cols-2">
      <div><dt className="font-medium">Environment</dt><dd className="text-muted">Sandbox</dd></div>
      <div><dt className="font-medium">Scenario testing</dt><dd className="text-muted">{merchant.scenarios.length ? `${merchant.scenarios.length} saved scenarios` : "No saved scenarios"}</dd></div>
      <div><dt className="font-medium">Products</dt><dd className="text-muted wrap-anywhere">{profile.products?.map(product => products[product] ?? product).join(", ") || "No recorded products"}</dd></div>
      <div><dt className="font-medium">Transaction stages</dt><dd className="text-muted">{profile.stages?.map(label).join(", ") || "Unavailable"}</dd></div>
      <div className="sm:col-span-2"><dt className="font-medium">Available signals</dt><dd className="text-muted wrap-anywhere">{profile.signals?.join(", ") || "Unavailable"}</dd></div>
    </dl>
  </section>;
}

function PolicyInspection({ path, token }: { path: string; token: string }) {
  const { response, failure, reload } = useMerchantResource<{ rules: MerchantPolicyVersion[] }>(`${path}/policies`, token);
  const [selected, setSelected] = useState("");
  return <section className={panel}>
    <h2 className="text-lg font-semibold">Business policies</h2>
    <p className="text-sm text-muted">Inspect this merchant’s business policies and saved versions.</p>
    {failure && <p role="alert" className="text-sm text-red-800">{failure.error} <button className={link} onClick={reload}>Retry</button></p>}
    {!failure && !response && <p role="status" className="text-sm text-muted">Loading policies…</p>}
    {response && !response.rules.length && <p className="text-sm text-muted">No business policies have been saved.</p>}
    {!!response?.rules.length && <ul className="divide-y divide-line">
      {response.rules.map(rule => <li key={rule.id} className="space-y-2 py-3">
        <div className="flex flex-wrap justify-between gap-2">
          <div className="min-w-0">
            <p className="text-sm font-semibold wrap-anywhere">{rule.proposal.name}</p>
            <p className="text-xs text-muted">{label(rule.status)} · {policyActionLabels[rule.proposal.action]} · Version {rule.version}</p>
            <p className="text-xs text-muted wrap-anywhere">{rule.proposal.product_types.map(product => products[product] ?? product).join(", ")}</p>
            {rule.createdAt && <p className="text-xs text-muted">{merchantDate(rule.createdAt)}</p>}
            {(rule.createdBy || rule.actor) && <p className="text-xs text-muted">Created by {rule.createdBy ?? rule.actor}</p>}
          </div>
          <button type="button" className={outlineButton} aria-expanded={selected === rule.id} onClick={() => setSelected(value => value === rule.id ? "" : rule.id)}>Versions</button>
        </div>
        {selected === rule.id && <PolicyVersions path={`${path}/policies/${encodeURIComponent(rule.id)}`} token={token} />}
      </li>)}
    </ul>}
  </section>;
}

function PolicyVersions({ path, token }: { path: string; token: string }) {
  const { response, failure } = useMerchantResource<MerchantPolicyHistory>(path, token);
  if (failure) return <p role="alert" className="text-sm text-red-800">{failure.error}</p>;
  if (!response) return <p role="status" className="text-sm text-muted">Loading versions…</p>;
  return <ul className="space-y-2">
    {response.versions.map(version => <li key={version.version}>
      <details className="rounded-lg border border-line bg-white p-3">
        <summary className="cursor-pointer text-sm font-semibold">Version {version.version} · {label(version.status)}</summary>
        <p className="mt-2 text-xs text-muted">{merchantDate(version.createdAt)}{(version.createdBy || version.actor) && ` · ${version.createdBy ?? version.actor}`}{version.changeReason && ` · ${version.changeReason}`}</p>
        <div className="mt-2"><RulePreview proposal={version.proposal} /></div>
      </details>
    </li>)}
  </ul>;
}

function MerchantAudit({ path, token }: { path: string; token: string }) {
  const { response, failure, reload } = useMerchantResource<{ events: MerchantAuditEvent[] }>(`${path}/audit`, token);
  return <section className={panel}>
    <h2 className="text-lg font-semibold">Audit Logs</h2>
    {failure && <p role="alert" className="text-sm text-red-800">{failure.error} <button className={link} onClick={reload}>Retry</button></p>}
    {!failure && !response && <p role="status" className="text-sm text-muted">Loading audit history…</p>}
    {response && !response.events.length && <p className="text-sm text-muted">No audit events are recorded for this merchant.</p>}
    {!!response?.events.length && <ul className="divide-y divide-line">
      {response.events.map(item => <li key={item.id} className="space-y-1 py-3 text-sm">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <p className="font-semibold first-letter:uppercase">{label(item.action)}</p>
          <time className="text-xs text-muted" dateTime={item.occurredAt}>{merchantDate(item.occurredAt)}</time>
        </div>
        <p className="text-xs text-muted wrap-anywhere">{item.actor} · {item.resourceId}{item.previousVersion != null && ` · Previous version ${item.previousVersion}`}{item.version != null && ` · Version ${item.version}`}</p>
        {item.reason && <p className="text-sm text-muted wrap-anywhere">{item.reason}</p>}
      </li>)}
    </ul>}
  </section>;
}

function SandboxPlatformDrafts({ merchant }: { merchant: DemoMerchant }) {
  const token = useRuleCredential("PLATFORM");
  const request = useCallback(<T,>(suffix: string, init?: RequestInit) =>
    merchantDemoRequest<T>(`/${encodeURIComponent(merchant.id)}/platform/rules${suffix}`, token, init), [merchant.id, token]);
  return <section className={panel}>
    <h2 className="text-lg font-semibold">Sandbox Platform drafts</h2>
    <p className="text-sm text-muted">Review legacy Platform drafts for saved Sandbox runs. Global Platform Rules are managed in the Platform module.</p>
    <Link href="/settings/system-rules" className={`inline-block text-sm ${link}`}>Open global Platform Rules</Link>
    {!token && <RuleAccess scope="PLATFORM" />}
    <SystemRulesDashboard merchantName={merchant.name} canEdit={Boolean(token)} request={request} />
  </section>;
}

function MerchantJevRules({ merchant }: { merchant: DemoMerchant }) {
  const token = useRuleCredential("PLATFORM");
  return <>
    {!token && <RuleAccess scope="PLATFORM" />}
    <JevRulesPanel merchant={{ id: merchant.id, name: merchant.name, token, canEdit: Boolean(token) }} />
  </>;
}
