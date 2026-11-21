"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { JevRulesPanel } from "../jev-rules/panel";
import { SystemRulesDashboard } from "../system-rules/dashboard";
import type { FormEvent, ReactNode } from "react";
import { getMerchantDemoToken, loadLocalMerchantDirectory, merchantDemoRequest, setMerchantDemoToken } from "../../merchant-demo";
import type { DemoDirectory, DemoMerchant, DemoScenario } from "../../merchant-demo";
import { MerchantTransactions } from "./transactions";
import { isBusinessModel } from "../merchant-setup/contracts";
import type { BusinessModel, MerchantSetup } from "../merchant-setup/contracts";

const action = "rounded-lg bg-accent px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-60";
const secondary = "rounded-lg border border-line px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-60";
const input = "control mt-1 min-w-0";
const card = "min-w-0 rounded-xl border border-line bg-white p-4 sm:p-5";
const count = new Intl.NumberFormat("en-US");
const modelLabels = { ota: "Online travel agency (OTA)", ecommerce: "E-commerce" };

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "The merchant request failed. Try again.";
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block min-w-0 text-sm font-medium">{label}{children}</label>;
}

const workspaceViews = {
  workspace: "Overview",
  transactions: "Transactions",
  jev: "Jev Rules",
  platform: "Platform Rules",
} as const;
type WorkspaceView = keyof typeof workspaceViews;

export function MerchantDirectory() {
  const [credential, setCredential] = useState("");
  const [token, setToken] = useState("");
  const [directory, setDirectory] = useState<DemoDirectory | null>(null);
  const [selectedId, selectMerchant] = useState("");
  const [view, setView] = useState<WorkspaceView>("workspace");
  const [createMode, setCreateMode] = useState(false);
  const [issuedToken, setIssuedToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [requiresToken, setRequiresToken] = useState(false);
  const [previewAttempt, setPreviewAttempt] = useState(0);
  const [error, setError] = useState("");

  useEffect(() => {
    let ignore = false;
    setLoading(true);
    setError("");
    async function load() {
      try {
        const local = await loadLocalMerchantDirectory();
        if (ignore) return;
        const saved = local ? "" : getMerchantDemoToken();
        setRequiresToken(!local);
        if (!local) setCredential(saved);
        const result = local ?? (saved ? await merchantDemoRequest<DemoDirectory>("", saved) : null);
        if (ignore || !result) return;
        setToken(saved);
        setDirectory(result);
        selectMerchant(new URLSearchParams(window.location.search).get("merchant") ?? result.merchants[0]?.id ?? "");
      } catch (failure) {
        if (!ignore) setError(errorMessage(failure));
      } finally {
        if (!ignore) setLoading(false);
      }
    }
    void load();
    return () => { ignore = true; };
  }, [previewAttempt]);

  async function signIn(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextToken = credential.trim();
    if (!nextToken) return;
    setLoading(true);
    setError("");
    try {
      const result = await merchantDemoRequest<DemoDirectory>("", nextToken);
      setMerchantDemoToken(nextToken);
      setToken(nextToken);
      setDirectory(result);
      selectMerchant(new URLSearchParams(window.location.search).get("merchant") ?? result.merchants[0]?.id ?? "");
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setLoading(false);
    }
  }

  function upsert(merchant: DemoMerchant) {
    setDirectory((current) => current && ({
      ...current,
      merchants: current.merchants.some(({ id }) => id === merchant.id)
        ? current.merchants.map((item) => item.id === merchant.id ? merchant : item)
        : [...current.merchants, merchant],
    }));
  }

  function signOut() {
    setMerchantDemoToken("");
    setToken("");
    setCredential("");
    setDirectory(null);
    selectMerchant("");
    setCreateMode(false);
    setIssuedToken("");
    setError("");
  }

  function removeMerchant(merchantId: string) {
    setDirectory(current => current && ({
      ...current, merchants: current.merchants.filter(merchant => merchant.id !== merchantId),
    }));
    selectMerchant(current => current === merchantId ? "" : current);
  }

  const selected = directory?.merchants.find(({ id }) => id === selectedId) ?? directory?.merchants[0];
  const admin = directory?.role === "admin";

  useEffect(() => {
    const search = new URLSearchParams(window.location.search);
    const requested = search.get("view");
    if (requested === "transactions" || requested === "jev" || requested === "platform") setView(requested);
  }, []);

  const requestPlatformRules = useCallback(<T,>(suffix: string, init?: RequestInit) => {
    if (!selected) return Promise.reject(new Error("Choose a merchant to load its platform rules."));
    return merchantDemoRequest<T>(`/${selected.id}/platform/rules${suffix}`, token, init);
  }, [selected?.id, token]);

  if (!directory && !requiresToken) {
    return <div className={`${card} max-w-lg space-y-3`}>
      {loading ? <p role="status" className="text-sm text-slate-600">Loading merchants…</p> : <>
        <p role="alert" className="text-sm text-red-700">{error}</p>
        <button type="button" className={secondary} onClick={() => setPreviewAttempt(attempt => attempt + 1)}>Retry</button>
      </>}
    </div>;
  }

  if (!directory) {
    return (
      <form onSubmit={signIn} className={`${card} max-w-lg space-y-3`}>
        <Field label="Demo access token">
          <input className={input} type="password" autoComplete="off" value={credential} onChange={(event) => setCredential(event.target.value)} required />
        </Field>
        <p className="text-sm text-slate-600">Use the configured administrator token or a merchant access token.</p>
        {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
        <button className={action} disabled={loading || !credential.trim()}>{loading ? "Connecting…" : "Connect"}</button>
      </form>
    );
  }

  return (
    <div className="grid items-start gap-5 lg:grid-cols-[16rem_minmax(0,1fr)]">
      <aside className={`${card} space-y-4`} aria-label="Merchant directory">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm text-slate-600">{!token ? "Local demo" : admin ? "Administrator" : "Merchant access"}</span>
          {token && <button className={secondary} onClick={signOut}>Sign out</button>}
        </div>
        <div className="space-y-2">
          {directory.merchants.map((merchant) => (
            <button
              key={merchant.id}
              type="button"
              aria-pressed={!createMode && selected?.id === merchant.id}
              className="block w-full break-words rounded-lg border border-line px-3 py-2 text-left text-sm font-medium hover:bg-paper aria-pressed:border-accent aria-pressed:bg-soft aria-pressed:text-accent"
              onClick={() => {
                selectMerchant(merchant.id);
                setCreateMode(false);
                setIssuedToken("");
              }}
            >
              {merchant.name}
            </button>
          ))}
          {!directory.merchants.length && <p className="text-sm text-slate-600">No merchants yet.</p>}
        </div>
        {admin && <button className={action} onClick={() => { setCreateMode(true); setIssuedToken(""); }}>Create merchant</button>}
      </aside>
      <div className="min-w-0 space-y-5">
        {admin && (createMode || !selected) ? (
          <MerchantNameForm token={token} onSave={(merchant, newToken) => {
            upsert(merchant);
            selectMerchant(merchant.id);
            setCreateMode(false);
            setIssuedToken(newToken ?? "");
          }} />
        ) : selected && (
          <>
            {token && issuedToken && (
              <div role="status" className={`${card} space-y-2`}>
                <h2 className="font-semibold">Merchant access token</h2>
                <p className="text-sm text-slate-600">Copy this token to sign in as {selected.name}. It is shown once.</p>
                <code className="block break-all rounded-md bg-paper p-2 text-sm">{issuedToken}</code>
                <button className={secondary} onClick={() => setIssuedToken("")}>Hide token</button>
              </div>
            )}
            <nav aria-label="Merchant workspace" className="flex flex-wrap gap-2">
              {(Object.entries(workspaceViews) as [WorkspaceView, string][]).map(([id, label]) => (
                <button key={id} type="button" aria-pressed={view === id} className={view === id ? action : secondary} onClick={() => setView(id)}>{label}</button>
              ))}
            </nav>
            {view === "workspace" && <MerchantWorkspace key={`workspace-${selected.id}`} token={token} merchant={selected} admin={admin} onSave={upsert} onDelete={removeMerchant} />}
            {(view === "workspace" || view === "transactions") && <MerchantTransactions key={`transactions-${selected.id}`} token={token} merchant={selected} />}
            {view === "jev" && <JevRulesPanel key={`jev-${selected.id}`} merchant={{ id: selected.id, name: selected.name, token, canEdit: admin }} />}
            {view === "platform" && <SystemRulesDashboard key={`platform-${selected.id}`} request={requestPlatformRules} merchantName={selected.name} canEdit={admin} />}
          </>
        )}
      </div>
    </div>
  );
}

function MerchantNameForm({ token, merchant, onSave, onDelete }: {
  token: string;
  merchant?: DemoMerchant;
  onSave: (merchant: DemoMerchant, newToken?: string) => void;
  onDelete?: (merchantId: string) => void;
}) {
  const [name, setName] = useState(merchant?.name ?? "");
  const [businessModel, setBusinessModel] = useState<BusinessModel | "">(merchant?.setup?.businessModel ?? "");
  const [includesAmount, setIncludesAmount] = useState(merchant?.setup?.availableSignals.includes("amountMinor") ?? false);
  const [saving, setSaving] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const isUnchanged = name.trim() === merchant?.name && businessModel === merchant.setup?.businessModel &&
    includesAmount === (merchant.setup?.availableSignals.includes("amountMinor") ?? false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!name.trim() || !businessModel || confirmDelete) return;
    setSaving(true);
    setError("");
    setSaved(false);
    const setup: MerchantSetup = {
      schemaVersion: 1, businessModel, stage: "booking", availableSignals: includesAmount ? ["occurredAt", "amountMinor"] : ["occurredAt"],
    };
    try {
      const result = await merchantDemoRequest<{ merchant: DemoMerchant; token?: string }>(merchant ? `/${merchant.id}` : "", token, {
        method: merchant ? "PUT" : "POST",
        body: JSON.stringify({ name: name.trim(), setup }),
      });
      onSave(result.merchant, result.token);
      setName(result.merchant.name);
      setSaved(true);
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setSaving(false);
    }
  }

  async function deleteMerchant() {
    if (!merchant || !onDelete || deleting) return;
    setDeleting(true);
    setError("");
    try {
      await merchantDemoRequest<{ deleted: boolean }>(`/${merchant.id}`, token, { method: "DELETE" });
      onDelete(merchant.id);
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setDeleting(false);
    }
  }

  return (
    <form onSubmit={submit} className={`${card} space-y-4`}>
      <h2 className="text-lg font-semibold">{merchant ? "Merchant details" : "New merchant"}</h2>
      <fieldset disabled={saving || deleting || confirmDelete} className="grid gap-4 sm:grid-cols-2">
        <legend className="sr-only">Merchant name and setup</legend>
        <Field label="Merchant name">
          <input className={input} value={name} onChange={event => { setName(event.target.value); setSaved(false); }} maxLength={80} required />
        </Field>
        <Field label="Business model">
          <select className={input} value={businessModel} required onChange={event => {
            const value = event.target.value;
            setBusinessModel(isBusinessModel(value) ? value : "");
            setSaved(false);
          }}>
            <option value="">Choose a business model</option>
            <option value="ota">{modelLabels.ota}</option>
            <option value="ecommerce">{modelLabels.ecommerce}</option>
          </select>
        </Field>
        <Field label="Journey stage">
          <select className={input} defaultValue="booking"><option value="booking">Booking</option></select>
        </Field>
        <fieldset className="space-y-2">
          <legend className="mb-1 text-sm font-medium">Available signals</legend>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked disabled />Booking timestamp (required)</label>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={includesAmount} onChange={event => { setIncludesAmount(event.target.checked); setSaved(false); }} />
            Booking amount
          </label>
        </fieldset>
      </fieldset>
      {merchant && !merchant.setup && <p className="text-sm text-slate-600">Setup incomplete. Choose a business model to finish this merchant’s setup.</p>}
      <div className="flex flex-wrap items-center gap-3">
        <button className={action} disabled={saving || deleting || confirmDelete || !name.trim() || !businessModel || isUnchanged}>
          {saving ? "Saving…" : "Save merchant"}
        </button>
        {merchant && onDelete && !confirmDelete && <button type="button" className="text-sm font-semibold text-red-700 hover:underline" disabled={saving} onClick={() => {
          setConfirmDelete(true);
          setSaved(false);
          setError("");
        }}>Delete merchant</button>}
      </div>
      {confirmDelete && merchant && <section className="space-y-3 rounded-lg border border-red-200 bg-red-50 p-3" aria-label="Confirm merchant deletion">
        <p className="text-sm text-red-900">Delete <strong>{merchant.name}</strong>? It will be removed from Merchants and the test pickers. Saved transaction history is retained.</p>
        <div className="flex flex-wrap gap-2">
          <button type="button" className="rounded-lg bg-red-700 px-3 py-2 text-sm font-semibold text-white disabled:opacity-60" disabled={deleting} onClick={() => void deleteMerchant()}>
            {deleting ? "Deleting…" : "Confirm deletion"}
          </button>
          <button type="button" className={secondary} disabled={deleting} onClick={() => setConfirmDelete(false)}>Cancel</button>
        </div>
      </section>}
      {saved && <p role="status" className="text-sm text-emerald-700">Merchant saved.</p>}
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
    </form>
  );
}

function MerchantWorkspace({ merchant, token, admin, onSave, onDelete }: {
  merchant: DemoMerchant;
  token: string;
  admin: boolean;
  onSave: (merchant: DemoMerchant) => void;
  onDelete: (merchantId: string) => void;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const scenario = merchant.scenarios.find(({ id }) => id === editing);

  return (
    <>
      {admin ? <MerchantNameForm token={token} merchant={merchant} onSave={onSave} onDelete={onDelete} /> : <section className={card}>
        <h2 className="text-xl font-semibold">{merchant.name}</h2>
        {merchant.setup ? <p className="mt-2 text-sm text-slate-600">
          {modelLabels[merchant.setup.businessModel]} · Booking · {merchant.setup.availableSignals.includes("amountMinor") ? "Timestamp and amount" : "Timestamp"}
        </p> : <p className="mt-2 text-sm text-slate-600">Setup incomplete.</p>}
      </section>}
      <section className={`${card} space-y-4`} aria-labelledby="merchant-scenarios">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 id="merchant-scenarios" className="text-lg font-semibold">Scenarios and rules</h2>
          {admin && <button className={secondary} onClick={() => setEditing("new")}>Add scenario</button>}
        </div>
        {!merchant.scenarios.length && <p className="text-sm text-slate-600">No scenarios yet. {admin && "Add a scenario to test its mock Jev rules."}</p>}
        {merchant.scenarios.map((item) => {
          const params = new URLSearchParams({ merchant: merchant.id, scenario: item.id });
          return (
            <div key={item.id} className="space-y-2 rounded-lg border border-line p-3">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <h3 className="break-words font-semibold">{item.name}</h3>
                  <p className="mt-1 text-sm text-slate-600">
                    Scenario v{item.scenarioVersion} · {item.ruleVersion}
                  </p>
                </div>
                {admin && <button className={secondary} aria-label={`Edit ${item.name}`} onClick={() => setEditing(item.id)}>Edit</button>}
              </div>
              <p className="text-sm">Send to review at {count.format(item.thresholdMinor)} minor units or above.</p>
              <div className="flex flex-wrap gap-2">
                <Link className={secondary} href={`/settings/mock-checkout?${params}`}>Mock checkout</Link>
                <Link className={secondary} href={`/?${params}`}>Simulation Lab</Link>
              </div>
            </div>
          );
        })}
        {admin && editing && <ScenarioForm key={editing} token={token} merchantId={merchant.id} scenario={scenario} onCancel={() => setEditing(null)} onSave={(saved) => {
          onSave({
            ...merchant,
            scenarios: scenario ? merchant.scenarios.map((item) => item.id === saved.id ? saved : item) : [...merchant.scenarios, saved],
          });
          setEditing(null);
        }} />}
      </section>
    </>
  );
}

function ScenarioForm({ token, merchantId, scenario, onSave, onCancel }: {
  token: string;
  merchantId: string;
  scenario?: DemoScenario;
  onSave: (scenario: DemoScenario) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(scenario?.name ?? "");
  const [threshold, setThreshold] = useState(String(scenario?.thresholdMinor ?? 50000));
  const [draft, setDraft] = useState<{ name: string; thresholdMinor: number } | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  function generate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const amount = threshold.trim();
    if (!name.trim() || !/^\d+$/.test(amount) || !Number.isSafeInteger(Number(amount))) {
      setError("Enter a name and a whole, nonnegative amount in minor units.");
      return;
    }
    setError("");
    setDraft({ name: name.trim(), thresholdMinor: Number(amount) });
  }

  async function save() {
    if (!draft) return;
    setSaving(true);
    setError("");
    try {
      const result = await merchantDemoRequest<{ scenario: DemoScenario }>(
        `/${merchantId}/scenarios${scenario ? `/${scenario.id}` : ""}`,
        token,
        { method: scenario ? "PUT" : "POST", body: JSON.stringify(draft) },
      );
      onSave(result.scenario);
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={generate} className="space-y-3 rounded-lg border border-line bg-paper p-3">
      <h3 className="font-semibold">{scenario ? "Edit scenario" : "New scenario"}</h3>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Scenario name">
          <input className={input} value={name} maxLength={80} required disabled={saving} onChange={(event) => {
            setName(event.target.value);
            setDraft(null);
            setError("");
          }} />
        </Field>
        <Field label="Review threshold in minor units">
          <input className={input} inputMode="numeric" value={threshold} required disabled={saving} onChange={(event) => {
            setThreshold(event.target.value);
            setDraft(null);
            setError("");
          }} />
        </Field>
      </div>
      {draft && <p role="status" className="rounded-md border border-line bg-white p-3 text-sm">
        <strong>Mock rule preview.</strong> Send to review when the observed booking amount is at least {count.format(draft.thresholdMinor)} minor units.
      </p>}
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <div className="flex flex-wrap gap-2">
        <button className={secondary} disabled={saving}>Generate rule preview</button>
        <button className={action} type="button" disabled={!draft || saving} onClick={() => void save()}>{saving ? "Saving…" : "Save scenario and rules"}</button>
        <button className={secondary} type="button" disabled={saving} onClick={onCancel}>Cancel</button>
      </div>
    </form>
  );
}
