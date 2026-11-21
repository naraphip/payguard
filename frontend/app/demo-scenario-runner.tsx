"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState } from "react";
import type { FormEvent } from "react";
import { merchantMailboxDigest, parseScenarioEmail } from "./merchant-mailbox";
import {
  getMerchantDemoToken, loadLocalMerchantDirectory, merchantDemoRequest, merchantOutcomeLabels, merchantPolicyProducts, policyActionLabels, setMerchantDemoToken,
} from "./merchant-demo";
import { MerchantJevEvidence } from "./merchant-jev-evidence";
import { PolicyDecisionDetails } from "./merchant-policy-decision";
import { merchantDetailsHref } from "./settings/merchants/management-contracts";
import type { DemoDirectory, DemoTransaction } from "./merchant-demo";
import type { SimulationProduct } from "./types";

type ScenarioInputs = {
  source: "checkout" | "simulation";
  product?: SimulationProduct;
  amountMinor?: number;
  customerEmail?: string;
  simulateTimeout: boolean;
};

type ScenarioAttempt = Omit<ScenarioInputs, "customerEmail"> & {
  id: string;
  scenarioId: string;
  customerEmailDomain?: string;
  customerMailboxDigest?: string;
};

const action = "rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50";
const secondary = "rounded-lg border border-line bg-white px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-50";

export function readDemoAmount(value: string) {
  const entered = value.trim();
  if (entered === "") return undefined;
  const amount = Number(entered);
  return /^\d+$/.test(entered) && Number.isSafeInteger(amount) ? amount : NaN;
}

export const invalidDemoAmount = "Enter a whole number from 0 to 9,007,199,254,740,991 or leave it empty.";

export function useDemoScenario() {
  const [credential, setCredential] = useState("");
  const [directory, setDirectory] = useState<DemoDirectory | null>(null);
  const [accessToken, setAccessToken] = useState("");
  const [merchantId, setMerchantId] = useState("");
  const [scenarioId, setScenarioId] = useState("");
  const [connecting, setConnecting] = useState(true);
  const [requiresToken, setRequiresToken] = useState(false);
  const [connectionError, setConnectionError] = useState("");
  const [selectionError, setSelectionError] = useState("");
  const [requestedSelection, setRequestedSelection] = useState(false);
  const [pending, setPending] = useState<ScenarioAttempt | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<DemoTransaction | null>(null);
  const [submissionError, setSubmissionError] = useState("");
  const requestGeneration = useRef(0);
  const connectionGeneration = useRef(0);
  const submittingRef = useRef(false);
  const merchant = directory?.merchants.find(option => option.id === merchantId);
  const scenario = merchant?.scenarios.find(option => option.id === scenarioId);

  function clearAttempt() {
    requestGeneration.current++;
    submittingRef.current = false;
    setPending(null);
    setSubmitting(false);
    setResult(null);
    setSubmissionError("");
  }

  async function connect(token?: string) {
    const automatic = token === undefined;
    let currentToken = token?.trim() ?? "";
    if (!automatic && !currentToken) return;
    const generation = ++connectionGeneration.current;
    setConnecting(true);
    setConnectionError("");
    setSelectionError("");
    clearAttempt();
    setDirectory(null);
    setAccessToken("");
    setMerchantId("");
    setScenarioId("");

    try {
      const local = automatic ? await loadLocalMerchantDirectory() : null;
      if (connectionGeneration.current !== generation) return;
      setRequiresToken(!local);
      if (automatic && !local) currentToken = getMerchantDemoToken();
      if (!local && !currentToken) return;
      const connected = local ?? await merchantDemoRequest<DemoDirectory>("", currentToken);
      if (connectionGeneration.current !== generation) return;
      const search = new URLSearchParams(window.location.search);
      const targetMerchant = search.get("merchant");
      const targetScenario = search.get("scenario");
      const selected = targetMerchant
        ? connected.merchants.find(option => option.id === targetMerchant)
        : connected.merchants.length === 1 ? connected.merchants[0] : undefined;
      const selectedScenario = targetScenario
        ? selected?.scenarios.find(option => option.id === targetScenario)
        : selected?.scenarios.length === 1 ? selected.scenarios[0] : undefined;
      setDirectory(connected);
      setAccessToken(currentToken);
      if (currentToken) setMerchantDemoToken(currentToken, connected.role);
      setCredential("");
      setMerchantId(selected?.id ?? "");
      setScenarioId(selectedScenario?.id ?? "");
      if (targetMerchant && !selected) setSelectionError("The linked merchant is unavailable with this token. Choose an accessible merchant.");
      else if (targetScenario && !selectedScenario) setSelectionError("The linked scenario is unavailable. Choose a scenario to continue.");
    } catch (cause) {
      if (connectionGeneration.current === generation) {
        setConnectionError(cause instanceof Error ? cause.message : "The merchant directory is unavailable.");
      }
    } finally {
      if (connectionGeneration.current === generation) setConnecting(false);
    }
  }

  useEffect(() => {
    const query = new URLSearchParams(window.location.search);
    setRequestedSelection(query.has("merchant") || query.has("scenario"));
    void connect();

    return () => {
      requestGeneration.current++;
      connectionGeneration.current++;
    };
  }, []);

  function disconnect() {
    connectionGeneration.current++;
    clearAttempt();
    setMerchantDemoToken("");
    setDirectory(null);
    setAccessToken("");
    setMerchantId("");
    setScenarioId("");
    setConnecting(false);
    setRequestedSelection(false);
    setConnectionError("");
    setSelectionError("");
  }

  function selectMerchant(id: string) {
    clearAttempt();
    setSelectionError("");
    setMerchantId(id);
    const scenarios = directory?.merchants.find(option => option.id === id)?.scenarios;
    setScenarioId(scenarios?.length === 1 ? scenarios[0].id : "");
  }

  function selectScenario(id: string) {
    clearAttempt();
    setSelectionError("");
    setScenarioId(id);
  }

  async function submit({ customerEmail, ...inputs }: ScenarioInputs) {
    if (!directory || !merchant || !scenario || submittingRef.current) return;
    const email = parseScenarioEmail(customerEmail);
    if (!email) {
      setSubmissionError("Enter a synthetic email or its domain, such as buyer@example.test.");
      return;
    }
    const request = ++requestGeneration.current;
    submittingRef.current = true;
    setSubmitting(true);
    setResult(null);
    setSubmissionError("");
    try {
      const attempt = pending ?? {
        ...inputs,
        id: crypto.randomUUID(),
        scenarioId: scenario.id,
        customerEmailDomain: email.domain,
        customerMailboxDigest: email.mailbox ? await merchantMailboxDigest(merchant.id, email.mailbox) : undefined,
      };
      if (requestGeneration.current !== request) return;
      setPending(attempt);
      const { transaction } = await merchantDemoRequest<{ transaction: DemoTransaction }>(
        `/${encodeURIComponent(merchant.id)}/transactions`, accessToken,
        { method: "POST", body: JSON.stringify(attempt) },
      );
      if (requestGeneration.current !== request) return;
      setResult(transaction);
      setPending(null);
    } catch (cause) {
      if (requestGeneration.current === request) {
        setSubmissionError(cause instanceof Error ? cause.message : "The Jev assessment could not be saved.");
      }
    } finally {
      if (requestGeneration.current === request) {
        submittingRef.current = false;
        setSubmitting(false);
      }
    }
  }

  return {
    credential, setCredential, directory, merchantId, scenarioId, merchant, scenario,
    connecting, connectionError, selectionError, requestedSelection, requiresToken,
    connect, disconnect, selectMerchant, selectScenario,
    pending, submitting, result, submissionError, submit, clearAttempt,
  };
}

type DemoScenarioState = ReturnType<typeof useDemoScenario>;

export function DemoScenarioPicker({ flow, allowLegacy = false }: { flow: DemoScenarioState; allowLegacy?: boolean }) {
  const tokenId = useId();
  return <div className="space-y-3">
    {!flow.directory && !flow.requiresToken && (flow.connecting
      ? <p role="status" className="text-xs text-muted">Loading merchants…</p>
      : <button type="button" className={secondary} onClick={() => void flow.connect()}>
        {flow.connectionError ? "Retry merchant directory" : "Use a merchant scenario"}
      </button>)}
    {!flow.directory && flow.requiresToken && <>
      <p className="text-xs leading-relaxed text-muted">
        Connect with an administrator or merchant token to select a saved scenario.
        <Link href="/settings/merchants" className="ml-1 font-semibold text-accent underline underline-offset-2">Open Merchants</Link>
      </p>
      <label htmlFor={tokenId} className="block text-sm font-semibold">Merchant directory token</label>
      <input id={tokenId} className="control" type="password" autoComplete="off" value={flow.credential}
        onChange={event => flow.setCredential(event.currentTarget.value)} disabled={flow.connecting}
        onKeyDown={event => {
          if (event.key === "Enter") {
            event.preventDefault();
            void flow.connect(flow.credential);
          }
        }} />
      <div className="flex flex-wrap items-center gap-2">
        <button type="button" className={secondary} onClick={() => void flow.connect(flow.credential)}
          disabled={!flow.credential.trim() || flow.connecting}>
          {flow.connecting ? "Connecting…" : "Connect merchant directory"}
        </button>
      </div>
    </>}
    {!flow.directory && allowLegacy && flow.requestedSelection && !flow.connecting &&
      <button type="button" className={secondary} onClick={flow.disconnect}>Use standalone checkout</button>}
    {flow.connectionError && <p role="alert" className="text-sm text-red-800">{flow.connectionError}</p>}
    {flow.directory && <>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="min-w-0 text-sm font-semibold">Merchant
          <select className="control mt-1.5" value={flow.merchantId} onChange={event => flow.selectMerchant(event.currentTarget.value)}>
            <option value="">Select merchant</option>
            {flow.directory.merchants.map(merchant => <option key={merchant.id} value={merchant.id}>{merchant.name}</option>)}
          </select>
        </label>
        <label className="min-w-0 text-sm font-semibold">Scenario
          <select className="control mt-1.5" value={flow.scenarioId}
            onChange={event => flow.selectScenario(event.currentTarget.value)} disabled={!flow.merchant?.scenarios.length}>
            <option value="">Select scenario</option>
            {flow.merchant?.scenarios.map(scenario => <option key={scenario.id} value={scenario.id}>{scenario.name}</option>)}
          </select>
        </label>
      </div>
      {flow.selectionError && <p role="alert" className="text-sm text-red-800">{flow.selectionError}</p>}
      {flow.scenario && <p className="text-xs text-muted">Scenario v{flow.scenario.scenarioVersion} · {flow.scenario.ruleVersion} · Review at {flow.scenario.thresholdMinor.toLocaleString()} minor units</p>}
      {(!flow.directory.merchants.length || (flow.merchant && !flow.merchant.scenarios.length)) &&
        <p className="text-sm text-muted">
          {flow.merchant ? "This merchant has no saved scenarios." : "No merchants are available."} <Link href="/settings/merchants" className="font-semibold text-accent underline underline-offset-2">Open Merchants</Link>
        </p>}
      {(allowLegacy || flow.requiresToken) && <button type="button" className={secondary} onClick={flow.disconnect}>
        {allowLegacy ? "Use standalone checkout" : "Disconnect merchant directory"}
      </button>}
    </>}
  </div>;
}

const reviewLabels = { NONE: "No manual review", NEEDS_DECISION: "Needs decision", RESOLVED: "Resolved" } as const;

export function DemoScenarioResult({ transaction, merchantName }: { transaction: DemoTransaction; merchantName: string }) {
  const final = transaction.policyDecision?.finalDecision;
  const result = final ? policyActionLabels[final.action] ?? final.action : merchantOutcomeLabels[transaction.outcome];
  const allowed = final ? final.action === "ALLOW" : transaction.outcome === "LOW_RISK";
  const title = final ? "Transaction decision" : "Jev result";
  return <section className="rounded-xl border border-line bg-white p-4 sm:p-5" aria-label={title}>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h3 className="font-semibold">{title}</h3>
      <p role="status" className={`rounded-full px-3 py-1 text-xs font-semibold ${allowed
        ? "bg-emerald-50 text-emerald-800" : "bg-amber-50 text-amber-900"}`}>{result}</p>
    </div>
    <p className="mt-2 text-sm text-muted">{transaction.reason}</p>
    {transaction.policyDecision && <div className="mt-3"><PolicyDecisionDetails decision={transaction.policyDecision} /></div>}
    <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
      <div className="min-w-0"><dt className="text-xs text-muted">Merchant</dt><dd className="break-words font-semibold">{merchantName}</dd></div>
      <div className="min-w-0"><dt className="text-xs text-muted">Scenario</dt><dd className="break-words font-semibold">{transaction.scenarioName} · v{transaction.scenarioVersion}</dd></div>
      <div><dt className="text-xs text-muted">Rule set</dt><dd className="font-semibold">{transaction.ruleVersion}</dd></div>
      {transaction.jevRuleVersion && <div><dt className="text-xs text-muted">Merchant Jev rules</dt><dd className="font-semibold">{transaction.jevRuleVersion}</dd></div>}
      <MerchantJevEvidence transaction={transaction} />
      <div><dt className="text-xs text-muted">Booking amount</dt><dd className="font-semibold">{transaction.amountMinor === null ? "Unavailable" : `${transaction.amountMinor.toLocaleString()} minor units`}</dd></div>
      <div className="min-w-0"><dt className="text-xs text-muted">Rule hits</dt><dd className="break-words font-semibold">{transaction.ruleHits.join(", ") || "None"}</dd></div>
      <div><dt className="text-xs text-muted">Transaction state</dt><dd className="font-semibold">{reviewLabels[transaction.reviewState]}</dd></div>
    </dl>
    <Link href={merchantDetailsHref(transaction.merchantId, "transactions", transaction.id)} className="mt-4 inline-block text-sm font-semibold text-accent underline underline-offset-2">View merchant transaction</Link>
  </section>;
}

export function DemoScenarioSimulation() {
  const flow = useDemoScenario();
  const [amount, setAmount] = useState("50000");
  const [product, setProduct] = useState<SimulationProduct>("flight_ticket");
  const [email, setEmail] = useState("");
  const [simulateTimeout, setSimulateTimeout] = useState(false);
  const [amountError, setAmountError] = useState("");
  const amountId = useId();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const amountMinor = readDemoAmount(amount);
    if (Number.isNaN(amountMinor)) {
      setAmountError(invalidDemoAmount);
      return;
    }
    void flow.submit({ source: "simulation", product, amountMinor, customerEmail: email, simulateTimeout });
  }

  return <section className="space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby="scenario-simulation">
    <div>
      <h2 id="scenario-simulation" className="font-semibold">Merchant scenario simulation</h2>
      <p className="mt-1 text-sm text-muted">Test one synthetic booking against a saved scenario and record the result in the merchant's transactions.</p>
    </div>
    <DemoScenarioPicker flow={flow} />
    {flow.directory && <form onSubmit={submit} noValidate className="space-y-3">
      <fieldset className="grid gap-3 sm:grid-cols-2" disabled={flow.submitting || flow.pending !== null || !flow.scenario}>
        <label className="text-sm font-semibold">Product
          <select className="control mt-1.5" value={product} onChange={event => setProduct(event.currentTarget.value as SimulationProduct)}>
            {Object.entries(merchantPolicyProducts).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
          </select>
        </label>
        <label className="text-sm font-semibold">Synthetic email or domain
          <input className="control mt-1.5" value={email} maxLength={320} placeholder="buyer@example.test (optional)"
            autoComplete="off" autoCapitalize="none" spellCheck={false} onChange={event => setEmail(event.currentTarget.value)} />
        </label>
        <label htmlFor={amountId} className="text-sm font-semibold">Booking amount in minor units
          <input id={amountId} className="control mt-1.5" inputMode="numeric" value={amount} placeholder="Optional"
            aria-invalid={!!amountError} aria-describedby={amountError ? `${amountId}-error` : undefined} onChange={event => {
              setAmount(event.currentTarget.value);
              setAmountError("");
            }} />
          {amountError && <span role="alert" id={`${amountId}-error`} className="mt-1 block text-xs text-red-800">{amountError}</span>}
        </label>
        <label className="text-sm font-semibold">Jev response
          <select className="control mt-1.5" value={simulateTimeout ? "timeout" : "mock"}
            onChange={event => setSimulateTimeout(event.currentTarget.value === "timeout")}>
            <option value="mock">Use configured Jev</option>
            <option value="timeout">Simulate timeout</option>
          </select>
        </label>
      </fieldset>
      {flow.submissionError && <p role="alert" className="text-sm text-red-800">{flow.submissionError}</p>}
      <div className="flex flex-wrap gap-2">
        <button className={action} type="submit" disabled={!flow.scenario || flow.submitting}>
          {flow.submitting ? "Assessing…" : flow.pending ? "Retry scenario simulation" : "Run scenario simulation"}
        </button>
        {flow.pending && !flow.submitting && <button type="button" className={secondary} onClick={flow.clearAttempt}>Start a new test</button>}
      </div>
    </form>}
    {flow.merchant && flow.result && <DemoScenarioResult transaction={flow.result} merchantName={flow.merchant.name} />}
  </section>;
}
