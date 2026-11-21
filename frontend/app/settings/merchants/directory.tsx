"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { getMerchantDemoToken, merchantDemoRequest, setMerchantDemoToken } from "../../merchant-demo";
import type { DemoDirectory, DemoMerchant, DemoScenario } from "../../merchant-demo";
import { MerchantTransactions } from "./transactions";

const action = "rounded-lg bg-accent px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-60";
const secondary = "rounded-lg border border-line px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-60";
const input = "control mt-1 min-w-0";
const card = "min-w-0 rounded-xl border border-line bg-white p-4 sm:p-5";
const count = new Intl.NumberFormat("en-US");

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "The merchant request failed. Try again.";
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block min-w-0 text-sm font-medium">{label}{children}</label>;
}

export function MerchantDirectory() {
  const [credential, setCredential] = useState("");
  const [token, setToken] = useState("");
  const [directory, setDirectory] = useState<DemoDirectory | null>(null);
  const [selectedId, selectMerchant] = useState("");
  const [createMode, setCreateMode] = useState(false);
  const [issuedToken, setIssuedToken] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const saved = getMerchantDemoToken();
    if (!saved) return;
    let ignore = false;
    setCredential(saved);
    setLoading(true);
    merchantDemoRequest<DemoDirectory>("", saved).then((result) => {
      if (ignore) return;
      setToken(saved);
      setDirectory(result);
      selectMerchant(new URLSearchParams(window.location.search).get("merchant") ?? result.merchants[0]?.id ?? "");
    }).catch((failure: unknown) => {
      if (!ignore) setError(errorMessage(failure));
    }).finally(() => {
      if (!ignore) setLoading(false);
    });
    return () => { ignore = true; };
  }, []);

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

  const selected = directory?.merchants.find(({ id }) => id === selectedId) ?? directory?.merchants[0];
  const admin = directory?.role === "admin";

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
          <span className="text-sm text-slate-600">{admin ? "Administrator" : "Merchant access"}</span>
          <button className={secondary} onClick={signOut}>Sign out</button>
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
            {issuedToken && (
              <div role="status" className={`${card} space-y-2`}>
                <h2 className="font-semibold">Merchant access token</h2>
                <p className="text-sm text-slate-600">Copy this token to sign in as {selected.name}. It is shown once.</p>
                <code className="block break-all rounded-md bg-paper p-2 text-sm">{issuedToken}</code>
                <button className={secondary} onClick={() => setIssuedToken("")}>Hide token</button>
              </div>
            )}
            <MerchantWorkspace key={`workspace-${selected.id}`} token={token} merchant={selected} admin={admin} onSave={upsert} />
            <MerchantTransactions key={`transactions-${selected.id}`} token={token} merchant={selected} />
          </>
        )}
      </div>
    </div>
  );
}

function MerchantNameForm({ token, merchant, onSave }: {
  token: string;
  merchant?: DemoMerchant;
  onSave: (merchant: DemoMerchant, newToken?: string) => void;
}) {
  const [name, setName] = useState(merchant?.name ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!name.trim()) return;
    setSaving(true);
    setError("");
    setSaved(false);
    try {
      const result = await merchantDemoRequest<{ merchant: DemoMerchant; token?: string }>(merchant ? `/${merchant.id}` : "", token, {
        method: merchant ? "PUT" : "POST",
        body: JSON.stringify({ name: name.trim() }),
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

  return (
    <form onSubmit={submit} className={`${card} space-y-3`}>
      <h2 className="text-lg font-semibold">{merchant ? "Merchant details" : "New merchant"}</h2>
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-0 flex-[1_1_16rem]">
          <Field label="Merchant name">
            <input className={input} value={name} onChange={(event) => { setName(event.target.value); setSaved(false); }} maxLength={80} required />
          </Field>
        </div>
        <button className={action} disabled={saving || !name.trim() || name.trim() === merchant?.name}>{saving ? "Saving…" : "Save merchant"}</button>
      </div>
      {saved && <p role="status" className="text-sm text-emerald-700">Merchant saved.</p>}
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
    </form>
  );
}

function MerchantWorkspace({ merchant, token, admin, onSave }: {
  merchant: DemoMerchant;
  token: string;
  admin: boolean;
  onSave: (merchant: DemoMerchant) => void;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const scenario = merchant.scenarios.find(({ id }) => id === editing);

  return (
    <>
      {admin ? <MerchantNameForm token={token} merchant={merchant} onSave={onSave} /> : <h2 className="text-xl font-semibold">{merchant.name}</h2>}
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
