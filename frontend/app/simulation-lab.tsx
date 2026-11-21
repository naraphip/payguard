"use client";

import { useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Activity, ArrowRight, ChevronDown, FlaskConical, GitFork, ListChecks, LoaderCircle, Play, ReceiptText, ShieldCheck, SlidersHorizontal } from "lucide-react";
import { Network } from "./network";
import { DemoScenarioSimulation } from "./demo-scenario-runner";
import { CartProduct, catalog, defaultPrices, Storefront, thb, travelProduct } from "./storefront";
import { TestPayment } from "./test-payment";
import type { CheckoutConfig } from "./test-payment";
import "./storefront.css";
import type { Config, Decision, Report, Run, Scenario, SimulationProduct } from "./types";

const defaults: Config = {
  seed: 42, product: "flight_ticket", scenario: "mixed", accounts: 12, rate: 30,
  quantity: 1, date: "2026-12-04", delivery: "", rotateIp: true, sharedIp: true,
  sharedDevice: true, sharedToken: true, newAccount: false, retry: true,
  missingToken: false, jevTimeout: false, merchantPolicy: false,
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

const panel = "shop-panel";
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

function checkoutInput(config: Config, email: string): Config | null {
  const at = email.lastIndexOf("@");
  const domain = email.slice(at + 1);
  if (email && (at < 1 || email.indexOf("@") !== at || domain.length > 253 || !contactDomainPattern.test(domain))) {
    return null;
  }
  return {
    ...config,
    merchantPolicy: config.product === "flight_ticket" && config.merchantPolicy,
    contactEmailDomain: email ? domain.toLowerCase() : undefined,
  };
}

function sameConfig(saved: Config, current: Config) {
  return (Object.keys(current) as (keyof Config)[]).every(key => saved[key] === current[key]);
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
          ["Batch test policy", d.merchantPolicyDecision],
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
  const [tenantToken, setTenantToken] = useState("");
  const [run, setRun] = useState<Run | null>(null);
  const [runContact, setRunContact] = useState<{ orderId: string; contact: Contact } | null>(null);
  const [selectedID, select] = useState("");
  const [report, setReport] = useState<Report | null>(null);
  const [busy, setBusy] = useState(false);
  const [reportBusy, setReportBusy] = useState(false);
  const [error, setError] = useState("");
  const [reportError, setReportError] = useState("");
  const [checkout, setCheckout] = useState<CheckoutConfig | null>(null);
  const [checkoutError, setCheckoutError] = useState(false);
  const [paymentBusy, setPaymentBusy] = useState(false);
  const lastRequest = useRef<{ id: string; config: Config } | null>(null);
  const scoped = tenantToken.trim();
  const runEndpoint = scoped ? "/api/v1/booking/network/runs" : endpoint;
  const requestHeaders = scoped ? { Authorization: `Bearer ${scoped}` } : undefined;
  const ota = travelProduct(config.product);
  const cartProduct = config.product === "payment_gateway" ? null : config.product;
  const item = cartProduct ? catalog[cartProduct] : null;
  const prices = checkout?.prices ?? defaultPrices;
  const quantity = Number.isFinite(config.quantity) ? config.quantity : 0;
  const subtotal = cartProduct ? prices[cartProduct] * quantity : 0;
  const controlsLocked = busy || paymentBusy;
  const selected = run?.decisions.find(d => d.event.id === selectedID);
  const current = checkoutInput(config, contact.email.trim());
  const firstOrderId = run?.decisions[0]?.event.orderId;
  const firstPayment = run?.decisions.find(({ event }) =>
    event.orderId === firstOrderId && (event.stage === "PRE_PAYMENT" || event.stage === "PAYMENT"),
  );
  const runMatchesCart = Boolean(run && current && sameConfig(run.config, current));
  const paymentDisabled = checkoutError ? "Test payment settings could not be loaded. Reload to try again."
    : !checkout ? "Loading test payment settings…"
    : !checkout.enabled || !checkout.publicKey?.startsWith("pkey_test_") ? "Omise test payments are not configured."
    : busy ? "Waiting for the risk decision."
    : !run ? "Run the simulation to check payment eligibility."
    : !runMatchesCart ? "Checkout changed. Run the simulation again."
    : firstPayment?.action !== "ALLOW" ? "Test payment requires an allowed decision on the first checkout."
    : "";

  useEffect(() => {
    const controller = new AbortController();
    api<CheckoutConfig>("/api/v1/simulation/checkout/config", { signal: controller.signal, cache: "no-store" })
      .then(data => { if (!controller.signal.aborted) setCheckout(data); })
      .catch(() => { if (!controller.signal.aborted) setCheckoutError(true); });
    return () => controller.abort();
  }, []);

  function update<K extends keyof Config>(key: K, value: Config[K]) {
    setConfig(current => ({ ...current, [key]: value }));
  }

  function product(value: SimulationProduct) {
    setConfig(current => ({
      ...current,
      product: value,
      delivery: value === "physical" ? "standard" : value === "digital" ? "instant" : "",
      date: travelProduct(value) ? current.date || defaults.date : "",
    }));
  }

  async function start(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (controlsLocked || reportBusy) return;
    setError("");
    const submittedContact = { name: contact.name.trim(), email: contact.email.trim(), phone: contact.phone.trim() };
    const current = checkoutInput(config, submittedContact.email);
    if (!current) {
      setContactEmailError(true);
      return;
    }
    setContactEmailError(false);
    setBusy(true);
    if (!lastRequest.current || JSON.stringify(lastRequest.current.config) !== JSON.stringify(current)) {
      lastRequest.current = { id: crypto.randomUUID(), config: current };
    }
    try {
      const result = await api<Run>(runEndpoint, {
        method: "POST", headers: { "Content-Type": "application/json", ...requestHeaders }, body: JSON.stringify(lastRequest.current),
      });
      lastRequest.current = null;
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
      setReport(await api<Report>(`${runEndpoint}/${run.id}/report`, { headers: requestHeaders }));
    } catch (failure) {
      setReportError((failure as Error).message);
    } finally {
      setReportBusy(false);
    }
  }

  return (
    <div className="storefront-shell min-h-screen">
      <main className="mx-auto max-w-[1328px] space-y-8 px-4 pt-7 pb-12 sm:px-8 sm:pt-10">
        <header>
          <h1 className="text-[34px] leading-tight font-bold tracking-[-0.035em] text-[#2d1e38] sm:text-[44px]">Simulation Lab</h1>
          <p className="mt-2 max-w-xl text-sm leading-relaxed text-[#5f516d] sm:text-[15px]">Test the selected merchant’s active rules or generate a batch of synthetic transactions.</p>
          <nav aria-label="Simulation Lab sections" className="mt-4 flex flex-wrap gap-2">
            <a className={outline} href="#merchant-rules">Test merchant rules</a>
            <a className={outline} href="#batch-simulation">Run a batch simulation</a>
          </nav>
        </header>

        <div id="merchant-rules" className="scroll-mt-24"><DemoScenarioSimulation /></div>
        <div id="batch-simulation" className="scroll-mt-24 border-t border-line pt-6">
          <h2 className="text-2xl font-semibold tracking-tight" id="batch-simulation-title">Batch simulation</h2>
          <p className="mt-1 text-sm text-[#5f516d]">Generate traffic to test platform rules and batch settings.</p>
        </div>

        <form aria-labelledby="batch-simulation-title" className="grid min-w-0 items-start gap-5 lg:grid-cols-[minmax(0,1fr)_340px] xl:grid-cols-[minmax(0,1fr)_368px] xl:gap-6"
          onSubmit={start} onInvalidCapture={event => {
            const details = (event.target as HTMLElement).closest("details");
            if (details) details.open = true;
          }}>
          <fieldset disabled={controlsLocked} className="min-w-0 space-y-4">
            <legend className="sr-only">Checkout and simulation settings</legend>
            <Storefront selected={config.product} prices={prices} choose={product} />

            <section className="shop-panel p-4 sm:p-5" aria-labelledby="checkout-heading">
              <h2 id="checkout-heading" className="text-[17px] font-semibold tracking-tight">Checkout details</h2>
              <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                {ota ? <Field label="Booking date"><input className="control min-w-0" type="date" required value={config.date} onChange={e => update("date", e.target.value)} /></Field>
                  : cartProduct && <Field label="Delivery"><select className="control" value={config.delivery} onChange={e => update("delivery", e.target.value)}>
                    {config.product === "digital" ? <option value="instant">Instant email</option> : <><option value="standard">Standard</option><option value="express">Express</option></>}
                  </select></Field>}
                <Field label={item?.quantity ?? "Payment quantity"}><input className="control" type="number" min={1} max={20} required value={config.quantity || ""} onChange={e => update("quantity", e.target.valueAsNumber)} /></Field>
              </div>

              <section className="mt-4 border-t border-[#eee7f4] pt-4" aria-labelledby="contact">
                <h3 id="contact" className="text-sm font-semibold">Contact <span className="font-normal text-[#5f516d]">(optional)</span></h3>
                <p id="contact-hint" className="mt-1 text-xs text-[#5f516d]">Use test contact details for the first checkout. Only the .test email domain is sent for evaluation.</p>
                <div className="mt-3 grid gap-3 sm:grid-cols-3">
                  <Field label="Name"><input className="control" type="text" autoComplete="off" maxLength={120} value={contact.name} onChange={e => setContact(previous => ({ ...previous, name: e.target.value }))} /></Field>
                  <Field label="Email"><input className="control" type="email" autoComplete="off" maxLength={254} placeholder="buyer@example.test" aria-invalid={contactEmailError} aria-describedby={contactEmailError ? "contact-hint contact-email-error" : "contact-hint"} value={contact.email} onChange={e => {
                    setContact(previous => ({ ...previous, email: e.target.value }));
                    setContactEmailError(false);
                  }} /></Field>
                  <Field label="Phone"><input className="control" type="tel" autoComplete="off" maxLength={40} value={contact.phone} onChange={e => setContact(previous => ({ ...previous, phone: e.target.value }))} /></Field>
                </div>
                {contactEmailError && <p id="contact-email-error" role="alert" className="mt-2 text-xs text-red-800">Use a valid .test email address, such as buyer@example.test.</p>}
                {config.product === "flight_ticket" && <label className="mt-3 flex items-center gap-2 text-xs text-[#4e3c60]"><input type="checkbox" className="size-4 accent-[#613283]" checked={config.merchantPolicy} onChange={e => update("merchantPolicy", e.target.checked)} />Block flights for restricted.test</label>}
              </section>
            </section>

            <details className="shop-panel group p-4 sm:p-5">
              <summary className="flex min-w-0 cursor-pointer list-none items-center justify-between gap-3 text-sm font-semibold [&::-webkit-details-marker]:hidden">
                <span className="flex min-w-0 items-center gap-2.5"><SlidersHorizontal size={17} className="shrink-0 text-[#603681]" aria-hidden="true" />Simulation settings</span>
                <span className="flex shrink-0 items-center gap-2 text-xs font-medium text-[#5f516d]">
                  {({ mixed: "Mixed traffic", normal: "Normal traffic", fraud: "Fraud ring" })[config.scenario]}
                  <ChevronDown size={16} className="transition group-open:rotate-180" aria-hidden="true" />
                </span>
              </summary>
              <div className="mt-4 space-y-4 border-t border-[#eee7f4] pt-4">
                <Field label="Demo merchant token (optional)">
                  <input className="control" type="password" autoComplete="off" disabled={reportBusy} value={tenantToken}
                    onChange={event => {
                      setTenantToken(event.currentTarget.value);
                      lastRequest.current = null;
                      setRun(null);
                      setRunContact(null);
                      select("");
                      setReport(null);
                      setError("");
                      setReportError("");
                    }} />
                  <span className="text-xs font-normal text-muted">Enter a token to scope linked entities and saved runs to your demo merchant.</span>
                </Field>
                <Field label="Traffic pattern"><select className="control" value={config.scenario} onChange={e => update("scenario", e.target.value as Scenario)}>
                  <option value="mixed">Mixed traffic</option><option value="normal">Normal traffic</option><option value="fraud">Fraud ring</option>
                </select></Field>
                <div className="grid grid-cols-3 gap-2 sm:gap-3">
                  <Field label="Seed"><input className="control" type="number" min={0} max={Number.MAX_SAFE_INTEGER} required value={Number.isNaN(config.seed) ? "" : config.seed} onChange={e => update("seed", e.target.valueAsNumber)} /></Field>
                  <Field label="Accounts"><input className="control" type="number" min={1} max={100} required value={config.accounts || ""} onChange={e => update("accounts", e.target.valueAsNumber)} /></Field>
                  <Field label="Orders/min"><input className="control" type="number" min={1} max={60} required value={config.rate || ""} onChange={e => update("rate", e.target.valueAsNumber)} /></Field>
                </div>
                <div>
                  <h3 className="text-xs font-semibold">Traffic signals and edge cases</h3>
                  <div className="mt-2.5 grid gap-2.5 sm:grid-cols-2">
                    {controls.map(([key, label]) => <label className="flex items-center gap-2 text-xs text-[#4e3c60]" key={key}><input type="checkbox" className="size-4 accent-[#613283]" checked={config[key]} onChange={e => update(key, e.target.checked)} />{label}</label>)}
                  </div>
                </div>
              </div>
            </details>
          </fieldset>

          <aside className="shop-panel space-y-4 p-4 sm:p-5 lg:mt-[54px]" aria-labelledby="cart-heading">
            <div className="flex items-center gap-2.5">
              <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-[#efe4f7] text-[#653588]"><ReceiptText size={18} aria-hidden="true" /></span>
              <h2 id="cart-heading" className="text-lg font-semibold tracking-tight">{ota ? "Trip summary" : cartProduct ? "Your cart" : "Gateway summary"}</h2>
            </div>

            {cartProduct ? <CartProduct product={cartProduct} date={config.date} delivery={config.delivery} /> : <div>
              <p className="text-sm font-semibold">Gateway payment</p>
              <p className="mt-1 text-xs text-[#5f516d]">Synthetic payment authorization</p>
            </div>}

            <dl className="space-y-2.5 border-y border-[#eee7f4] py-3 text-sm">
              <div className="flex justify-between gap-3 text-[#5f516d]"><dt>{item?.quantity ?? "Payment quantity"}</dt><dd>{quantity || "—"}</dd></div>
              {cartProduct && item && <div className="flex justify-between gap-3 text-[#5f516d]"><dt>Price per {item.unit}</dt><dd>{thb(prices[cartProduct])}</dd></div>}
            </dl>

            {cartProduct && <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-semibold">Total <span className="font-normal text-[#5f516d]">(THB)</span></span>
              <strong className="text-[25px] leading-tight font-semibold tracking-tight text-[#3b2049]">{thb(subtotal)}</strong>
            </div>}

            {runMatchesCart && firstPayment && <div role="status" className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-[#e8ddef] bg-[#f8f3fb] px-3 py-2">
              <span className="text-xs font-medium text-[#4e3c60]">{cartProduct ? "First checkout decision" : "First gateway decision"}</span><Badge action={firstPayment.action} />
            </div>}
            {error && <p role="alert" className="rounded-lg bg-red-50 p-3 text-sm text-red-800">{error}</p>}

            <div className="space-y-2.5">
              <button type="submit" disabled={controlsLocked || reportBusy} className="shop-primary">
                {busy ? <LoaderCircle size={16} className="animate-spin" aria-hidden="true" /> : <Play size={16} aria-hidden="true" />}
                {busy ? "Running batch simulation…" : "Run batch simulation"}
              </button>
              {cartProduct && !scoped ? <TestPayment config={checkout} runId={runMatchesCart && run ? run.id : null} amount={subtotal}
                disabledReason={paymentDisabled} onBusyChange={setPaymentBusy} /> : <p className="text-xs text-[#5f516d]">
                  {cartProduct ? "Omise Test Mode is available for anonymous storefront runs." : "Gateway runs simulate authorization. Omise Test Mode is available for anonymous storefront runs."}
                </p>}
            </div>
          </aside>
        </form>

        <section className="space-y-4" aria-labelledby="results-title">
          <div>
            <h2 id="results-title" className="text-2xl font-semibold tracking-tight">Batch simulation results</h2>
            <p className="mt-1 text-sm text-[#5f516d]">Inspect checkout decisions and score the run against synthetic labels.</p>
          </div>
          <Summary run={run} />
          <div className="min-w-0 space-y-5" aria-busy={busy}>
            <section id="fraud-network" className={`${panel} scroll-mt-24`} aria-labelledby="events">
              <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line p-4 sm:p-5">
                <h2 id="events" className="font-semibold">Event decisions</h2>
                {run && <span className="rounded-md bg-paper px-2 py-1 text-xs tabular-nums text-muted">Seed {run.config.seed}</span>}
              </div>
              {!run ? <div className="grid min-h-64 place-items-center px-5 py-8 text-center">
                <div><div className="mx-auto mb-3 grid size-12 place-items-center rounded-xl bg-soft text-accent"><FlaskConical size={23} aria-hidden /></div>
                  <p className="text-sm font-semibold">Ready for a simulation</p><p className="mt-1 text-sm text-muted">Run the simulation to see decisions and linked evidence.</p>
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
        </section>
      </main>
    </div>
  );
}
