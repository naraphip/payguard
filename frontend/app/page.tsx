"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Activity, ArrowRight, FlaskConical, GitFork, Hotel, ListChecks, Package, Plane, Play, ShieldCheck, ShoppingBag, SlidersHorizontal, TabletSmartphone } from "lucide-react";
import { Network } from "./network";
import type { Config, Decision, Product, Report, Run, Scenario } from "./types";

const defaults: Config = {
  seed: 42, product: "flight_ticket", scenario: "mixed", accounts: 12, rate: 30,
  quantity: 1, date: "2026-12-04", delivery: "", rotateIp: true, sharedIp: true,
  sharedDevice: true, sharedToken: true, newAccount: false, retry: true,
  missingToken: false, jevTimeout: false, merchantPolicy: false,
};

const products = {
  flight_ticket: { label: "Flight ticket", detail: "Bangkok to Singapore", icon: Plane },
  hotel_room: { label: "Hotel room", detail: "Singapore city stay", icon: Hotel },
  physical: { label: "Physical goods", detail: "Premium headphones", icon: Package },
  digital: { label: "Digital goods", detail: "Digital gift card", icon: TabletSmartphone },
};

const controls = [
  ["rotateIp", "Rotate ring IPs"], ["sharedIp", "Shared legitimate Wi-Fi"],
  ["sharedDevice", "Share ring devices"], ["sharedToken", "Share ring payment tokens"],
  ["newAccount", "Use new accounts"], ["retry", "Duplicate PSP retry"],
  ["missingToken", "Omit payment token"], ["jevTimeout", "Mock Jev timeout"],
] as const;

const actionStyle: Record<string, string> = {
  ALLOW: "bg-emerald-50 text-emerald-800",
  REVIEW: "bg-amber-50 text-amber-900",
  CHALLENGE: "bg-indigo-50 text-indigo-800",
  BLOCK: "bg-red-50 text-red-800",
  UNSUCCESSFUL: "bg-slate-100 text-slate-800",
};

const panel = "rounded-xl border border-line bg-white";
const button = "inline-flex items-center justify-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-white hover:bg-indigo-700 disabled:cursor-wait disabled:opacity-65";
const outline = "inline-flex items-center justify-center gap-2 rounded-lg border border-line bg-white px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-65";
const endpoint = "/api/v1/simulation/runs";
const display = (value: string) => value.replaceAll("_", " ");
const contactDomainPattern = /^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[Tt][Ee][Ss][Tt]$/;
const emptyContact = { name: "", email: "", phone: "" };
type Contact = typeof emptyContact;

async function api<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
  const body = await response.json().catch(() => null);
  if (!response.ok || !body) throw new Error(body?.error ?? "Simulation API unavailable. Check the local Go service.");
  return body as T;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="grid gap-1.5 text-xs font-semibold">{label}{children}</label>;
}

function Badge({ action }: { action: string }) {
  return <span className={`inline-block rounded-full px-2.5 py-1 text-xs font-semibold ${actionStyle[action] ?? "bg-paper"}`}>{display(action)}</span>;
}

function Summary({ run }: { run: Run | null }) {
  const count = (action: string) => run?.decisions.filter(d => d.action === action).length ?? 0;
  const cards = [
    { label: "Events evaluated", value: run?.decisions.length ?? 0, icon: Activity, color: "bg-soft text-accent" },
    { label: "Allowed", value: count("ALLOW"), icon: ShieldCheck, color: "bg-emerald-50 text-emerald-800" },
    { label: "Review or challenge", value: count("REVIEW") + count("CHALLENGE"), icon: ListChecks, color: "bg-amber-50 text-amber-800" },
    { label: "Retries deduplicated", value: run?.duplicates ?? 0, icon: GitFork, color: "bg-violet-50 text-violet-800" },
  ];
  return <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
    {cards.map(({ label, value, icon: Icon, color }) => (
      <div className={`${panel} flex items-start gap-3 p-3 sm:p-4`} key={label}>
        <span className={`rounded-lg p-2 ${color}`}><Icon size={17} aria-hidden /></span>
        <div><p className="text-xs font-medium text-muted">{label}</p><p className="mt-0.5 text-2xl font-semibold tabular-nums">{run ? value : "—"}</p></div>
      </div>
    ))}
  </div>;
}

function Evidence({ decision, run, contact }: { decision: Decision; run: Run; contact?: Contact }) {
  const d = decision;
  return (
    <section className="border-t border-line p-4 sm:p-5" aria-labelledby="evidence">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <h3 id="evidence" className="font-semibold">Decision evidence</h3>
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-xs text-muted">{d.event.id}</span>
          <Badge action={d.action} />
        </div>
      </div>
      {contact && <div className="mb-4 rounded-lg border border-line p-3">
        <h4 className="text-sm font-semibold">Checkout contact</h4>
        <dl className="mt-2 grid gap-3 text-xs sm:grid-cols-3">
          {([["Name", contact.name], ["Email", contact.email], ["Phone", contact.phone]] as const)
            .filter(([, value]) => value)
            .map(([label, value]) => <div className="min-w-0" key={label}>
              <dt className="text-muted">{label}</dt><dd className="mt-1 wrap-anywhere font-semibold">{value}</dd>
            </div>)}
        </dl>
      </div>}
      <dl className="mb-4 grid grid-cols-2 gap-3 text-xs sm:grid-cols-3">
        {[
          ["Platform action", d.platformDecision],
          ["Merchant policy", d.merchantPolicyDecision],
          ["Event email domain", d.event.domain],
          ["Final action source", d.actionSource],
          ["Jev status", d.jevStatus],
          ["Jev risk level", d.jevAnswer],
          ["Missing signals", d.missing.join(", ") || "None"],
        ].map(([label, value]) => (
          <div className="rounded-lg bg-paper p-3" key={label}>
            <dt className="text-muted">{label}</dt><dd className="mt-1 wrap-anywhere font-semibold">{value}</dd>
          </div>
        ))}
      </dl>
      <p className="text-sm"><strong>Rule hits</strong> <span className="break-words text-muted">{d.ruleHits.join(", ") || "None"}</span></p>
      <p className="mt-2 text-xs text-muted"><strong>60-second order counts</strong> {d.features.map(f => `${f.kind} ${f.orders}`).join(" · ")}</p>
      <h4 className="mb-2 mt-5 text-sm font-semibold">Entity links</h4>
      <Network selected={d} decisions={run.decisions} />
    </section>
  );
}

function Metrics({ report }: { report: Report }) {
  const percent = (n: number | null) => n === null ? "—" : new Intl.NumberFormat("en", { style: "percent", maximumFractionDigits: 1 }).format(n);
  return <>
    <div className="my-4 grid grid-cols-3 gap-2">
      {([["Precision", report.precision], ["Recall", report.recall], ["False positive rate", report.fpr]] as const).map(([name, value]) => (
        <div className="rounded-lg bg-paper p-3" key={name}><div className="text-xs text-muted">{name}</div><div className="mt-1 text-lg font-semibold">{percent(value)}</div></div>
      ))}
    </div>
    <p className="mb-2 text-xs text-muted">{report.samples} synthetic payment samples, {report.unscored} unscored</p>
    <table className="w-full text-left text-xs">
      <thead className="bg-paper"><tr><th className="px-3 py-2" scope="col">QA label</th><th scope="col">Flagged</th><th scope="col">Clear</th></tr></thead>
      <tbody>
        <tr className="border-b border-line"><th className="px-3 py-2" scope="row">Fraud</th><td>{report.tp}</td><td>{report.fn}</td></tr>
        <tr><th className="px-3 py-2" scope="row">Normal</th><td>{report.fp}</td><td>{report.tn}</td></tr>
      </tbody>
    </table>
  </>;
}

export default function Home() {
  const [config, setConfig] = useState(defaults);
  const [contact, setContact] = useState(emptyContact);
  const [contactEmailError, setContactEmailError] = useState(false);
  const [run, setRun] = useState<Run | null>(null);
  const [runContact, setRunContact] = useState<{ orderId: string; contact: Contact } | null>(null);
  const [selectedID, select] = useState("");
  const [report, setReport] = useState<Report | null>(null);
  const [busy, setBusy] = useState(false);
  const [reportBusy, setReportBusy] = useState(false);
  const [error, setError] = useState("");
  const [reportError, setReportError] = useState("");
  const lastRequest = useRef<{ id: string; config: Config } | null>(null);
  const ota = config.product === "flight_ticket" || config.product === "hotel_room";
  const choices: Product[] = ota ? ["flight_ticket", "hotel_room"] : ["physical", "digital"];
  const selected = run?.decisions.find(d => d.event.id === selectedID);

  function update<K extends keyof Config>(key: K, value: Config[K]) {
    setConfig(current => ({ ...current, [key]: value }));
  }

  function product(value: Product) {
    setConfig(current => ({
      ...current,
      product: value,
      delivery: value === "physical" ? "standard" : value === "digital" ? "instant" : "",
      date: value === "flight_ticket" || value === "hotel_room" ? current.date || defaults.date : "",
    }));
  }

  async function start(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    const submittedContact = { name: contact.name.trim(), email: contact.email.trim(), phone: contact.phone.trim() };
    const emailDomain = submittedContact.email.slice(submittedContact.email.lastIndexOf("@") + 1);
    if (submittedContact.email && (emailDomain.length > 253 || !contactDomainPattern.test(emailDomain))) {
      setContactEmailError(true);
      return;
    }
    setContactEmailError(false);
    setBusy(true);
    const current: Config = {
      ...config,
      merchantPolicy: config.product === "flight_ticket" && config.merchantPolicy,
      contactEmailDomain: emailDomain.toLowerCase() || undefined,
    };
    if (!lastRequest.current || JSON.stringify(lastRequest.current.config) !== JSON.stringify(current)) {
      lastRequest.current = { id: crypto.randomUUID(), config: current };
    }
    try {
      const result = await api<Run>(endpoint, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(lastRequest.current),
      });
      setRun(result);
      const firstOrderID = result.decisions[0]?.event.orderId;
      const savedContact = firstOrderID && Object.values(submittedContact).some(Boolean)
        ? { orderId: firstOrderID, contact: submittedContact }
        : null;
      setRunContact(savedContact);
      const contactPayment = savedContact && result.decisions.find(({ event }) =>
        event.orderId === savedContact.orderId && (event.stage === "PRE_PAYMENT" || event.stage === "PAYMENT"));
      const preferred = contactPayment || result.decisions.find(d => d.action === "REVIEW") || result.decisions[1] || result.decisions[0];
      select(preferred?.event.id ?? "");
      setReport(null);
      setReportError("");
    } catch (failure) {
      setError((failure as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function score() {
    if (!run) return;
    setReportBusy(true);
    setReportError("");
    try {
      setReport(await api<Report>(`${endpoint}/${run.id}/report`));
    } catch (failure) {
      setReportError((failure as Error).message);
    } finally {
      setReportBusy(false);
    }
  }

  return (
    <>
      <header className="border-b border-line bg-white">
        <div className="mx-auto flex min-h-16 max-w-[1440px] items-center justify-between px-4 sm:px-8">
          <div className="flex items-center gap-3">
            <span className="grid size-9 place-items-center rounded-lg bg-ink text-white"><ShieldCheck size={21} aria-hidden /></span>
            <span className="font-semibold tracking-tight">PayGuard</span>
            <span className="hidden border-l border-line pl-3 text-sm text-muted sm:inline">Simulation Lab</span>
          </div>
          <div className="flex shrink-0 items-center gap-2 sm:gap-3">
            <Link href="/settings/system-rules" className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-white px-2.5 py-1.5 text-xs font-semibold text-muted hover:bg-paper hover:text-ink">
              <SlidersHorizontal size={14} aria-hidden /><span>Platform Rules</span>
            </Link>
            <span className="hidden items-center gap-2 rounded-full border border-line bg-paper px-3 py-1 text-xs font-semibold text-muted min-[400px]:flex"><span className="size-2 rounded-full bg-teal-600" /> Sandbox</span>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-[1440px] space-y-5 px-4 py-6 sm:px-8 sm:py-8">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Simulation Lab</h1>
          <p className="mt-2 text-sm text-muted">Simulate synthetic checkout activity and inspect PayGuard risk decisions.</p>
        </div>
        <Summary run={run} />

        <div className="grid items-start gap-5 lg:grid-cols-[350px_minmax(0,1fr)] xl:grid-cols-[380px_minmax(0,1fr)]">
          <form onSubmit={start} className={`${panel} min-w-0`}>
            <fieldset disabled={busy} className="min-w-0 space-y-5 p-4 sm:p-5">
              <legend className="sr-only">Simulation setup</legend>
              <h2 className="font-semibold">Simulation setup</h2>
              <div className="grid grid-cols-2 gap-1 rounded-lg bg-paper p-1" role="group" aria-label="Storefront">
                {[{ label: "OTA", active: ota, icon: Plane, value: "flight_ticket" }, { label: "E-commerce", active: !ota, icon: ShoppingBag, value: "physical" }].map(({ label, active, icon: Icon, value }) => (
                  <button key={label} type="button" onClick={() => product(value as Product)} aria-pressed={active} className={`flex items-center justify-center gap-2 rounded-md px-2 py-2 text-xs font-semibold ${active ? "bg-white text-accent shadow-xs" : "text-muted hover:text-ink"}`}>
                    <Icon size={15} aria-hidden />{label}
                  </button>
                ))}
              </div>

              <div className="grid grid-cols-2 gap-2" role="group" aria-label="Product">
                {choices.map(key => {
                  const { label, detail, icon: Icon } = products[key];
                  const active = key === config.product;
                  return <button key={key} type="button" aria-pressed={active} onClick={() => product(key)} className={`min-w-0 rounded-lg border p-2.5 text-left ${active ? "border-accent bg-soft" : "border-line hover:bg-paper"}`}>
                    <Icon size={17} className={active ? "text-accent" : "text-muted"} aria-hidden />
                    <span className="mt-2 block text-xs font-semibold">{label}</span>
                    <span className="mt-0.5 block text-[11px] leading-tight text-muted">{detail}</span>
                  </button>;
                })}
              </div>

              <div className="grid grid-cols-2 gap-3">
                {ota ? <Field label="Booking date"><input className="control min-w-0" type="date" required value={config.date} onChange={e => update("date", e.target.value)} /></Field>
                  : <Field label="Delivery"><select className="control" value={config.delivery} onChange={e => update("delivery", e.target.value)}>
                    {config.product === "digital" ? <option value="instant">Instant email</option> : <><option value="standard">Standard</option><option value="express">Express</option></>}
                  </select></Field>}
                <Field label={ota ? "Party / rooms" : "Cart quantity"}><input className="control" type="number" min={1} max={20} required value={config.quantity || ""} onChange={e => update("quantity", e.target.valueAsNumber)} /></Field>
              </div>

              <section className="space-y-3 border-t border-line pt-4" aria-labelledby="contact">
                <div>
                  <h3 id="contact" className="text-sm font-semibold">Contact <span className="font-normal text-muted">(optional)</span></h3>
                  <p id="contact-hint" className="mt-1 text-xs text-muted">Use test contact details for the first checkout. Only the .test email domain is sent for evaluation.</p>
                </div>
                <Field label="Name"><input className="control" type="text" autoComplete="off" maxLength={120} value={contact.name} onChange={e => setContact(current => ({ ...current, name: e.target.value }))} /></Field>
                <Field label="Email"><input className="control" type="email" autoComplete="off" maxLength={254} placeholder="buyer@example.test" aria-invalid={contactEmailError} aria-describedby={contactEmailError ? "contact-hint contact-email-error" : "contact-hint"} value={contact.email} onChange={e => {
                  setContact(current => ({ ...current, email: e.target.value }));
                  setContactEmailError(false);
                }} /></Field>
                {contactEmailError && <p id="contact-email-error" role="alert" className="text-xs text-red-800">Use a valid .test email address, such as buyer@example.test.</p>}
                <Field label="Phone"><input className="control" type="tel" autoComplete="off" maxLength={40} value={contact.phone} onChange={e => setContact(current => ({ ...current, phone: e.target.value }))} /></Field>
                {config.product === "flight_ticket" && <label className="flex items-center gap-2 text-xs"><input type="checkbox" className="size-4 accent-accent" checked={config.merchantPolicy} onChange={e => update("merchantPolicy", e.target.checked)} />Block flights for restricted.test</label>}
              </section>

              <div className="space-y-3 border-t border-line pt-4">
                <Field label="Scenario"><select className="control" value={config.scenario} onChange={e => update("scenario", e.target.value as Scenario)}>
                  <option value="mixed">Mixed traffic</option><option value="normal">Normal traffic</option><option value="fraud">Fraud ring</option>
                </select></Field>
                <div className="grid grid-cols-3 gap-2">
                  <Field label="Seed"><input className="control" type="number" min={0} max={Number.MAX_SAFE_INTEGER} required value={Number.isNaN(config.seed) ? "" : config.seed} onChange={e => update("seed", e.target.valueAsNumber)} /></Field>
                  <Field label="Accounts"><input className="control" type="number" min={1} max={100} required value={config.accounts || ""} onChange={e => update("accounts", e.target.valueAsNumber)} /></Field>
                  <Field label="Orders/min"><input className="control" type="number" min={1} max={60} required value={config.rate || ""} onChange={e => update("rate", e.target.valueAsNumber)} /></Field>
                </div>
              </div>

              <details className="rounded-lg border border-line px-3 py-2.5">
                <summary className="cursor-pointer text-xs font-semibold">Traffic signals and edge cases</summary>
                <div className="mt-3 grid gap-2.5">
                  {controls.map(([key, label]) => <label className="flex items-center gap-2 text-xs" key={key}><input type="checkbox" className="size-4 accent-accent" checked={config[key]} onChange={e => update(key, e.target.checked)} />{label}</label>)}
                </div>
              </details>

              {error && <p role="alert" className="rounded-lg bg-red-50 p-3 text-sm text-red-800">{error}</p>}
              <button disabled={busy || reportBusy} type="submit" className={`${button} w-full`}><Play size={16} aria-hidden />{busy ? "Running simulation…" : "Run simulation"}</button>
            </fieldset>
          </form>

          <div className="min-w-0 space-y-5" aria-busy={busy}>
            <section className={panel} aria-labelledby="events">
              <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line p-4 sm:p-5">
                <h2 id="events" className="font-semibold">Event decisions</h2>
                {run && <span className="rounded-md bg-paper px-2 py-1 text-xs tabular-nums text-muted">Seed {run.config.seed}</span>}
              </div>
              {!run ? <div className="grid min-h-64 place-items-center px-5 py-8 text-center">
                <div><div className="mx-auto mb-3 grid size-12 place-items-center rounded-xl bg-soft text-accent"><FlaskConical size={23} aria-hidden /></div>
                  <p className="text-sm font-semibold">Ready for a simulation</p><p className="mt-1 text-sm text-muted">Run the scenario to see decisions and linked evidence.</p>
                </div>
              </div> : <>
                <div className="max-h-80 overflow-auto" tabIndex={0} aria-label="Synthetic event decisions">
                  <table className="w-full table-fixed text-left text-sm">
                    <thead className="sticky top-0 bg-paper text-xs text-muted"><tr><th scope="col" className="w-[41%] px-3 py-2.5 sm:px-4">Event</th><th scope="col" className="w-[27%] pr-1">Stage</th><th scope="col" className="pr-2">Final action</th></tr></thead>
                    <tbody>{run.decisions.map(d => (
                      <tr className={`border-b border-line ${selectedID === d.event.id ? "bg-soft" : "hover:bg-paper"}`} key={d.event.id}>
                        <td className="px-3 py-2 sm:px-4"><button className="flex items-center gap-1 rounded text-left text-xs font-semibold text-accent" aria-pressed={selectedID === d.event.id} onClick={() => select(d.event.id)}><span className="min-w-0 wrap-anywhere">{d.event.id}</span><ArrowRight size={13} className="hidden shrink-0 sm:block" aria-hidden /></button></td>
                        <td className="pr-1 text-xs text-muted">{display(d.event.stage)}</td><td className="pr-2"><Badge action={d.action} /></td>
                      </tr>
                    ))}</tbody>
                  </table>
                </div>
                {selected && <Evidence decision={selected} run={run} contact={runContact?.orderId === selected.event.orderId ? runContact.contact : undefined} />}
                <p className="border-t border-line px-4 py-3 font-mono text-xs text-muted sm:px-5">{run.generatorVersion} / {run.ruleVersion} / {run.jevVersion}</p>
              </>}
            </section>

            <section className={`${panel} p-4 sm:p-5`} aria-labelledby="qa">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div><h2 id="qa" className="font-semibold">Offline QA</h2><p className="mt-1 text-xs text-muted">Compare platform decisions with synthetic payment labels.</p></div>
                {run && <button className={outline} disabled={busy || reportBusy} onClick={score}>{reportBusy ? "Scoring…" : "Score run"}</button>}
              </div>
              {reportError && <p role="alert" className="mt-3 text-sm text-red-800">{reportError}</p>}
              {report && <Metrics report={report} />}
            </section>
          </div>
        </div>
      </main>
    </>
  );
}
