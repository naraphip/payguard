"use client";

import Link from "next/link";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import type { FormEvent } from "react";
import { merchantMailboxDigest, parseScenarioEmail } from "./merchant-mailbox";
import {
  getMerchantDemoRole, getMerchantDemoToken, loadLocalMerchantDirectory, merchantDemoRequest, merchantOutcomeLabels, merchantPolicyProducts, policyActionLabels, setMerchantDemoToken,
} from "./merchant-demo";
import { MerchantJevEvidence } from "./merchant-jev-evidence";
import { PolicyDecisionDetails } from "./merchant-policy-decision";
import { RiskScoreDetails } from "./merchant-risk-score";
import { merchantDate, merchantDetailsHref } from "./settings/merchants/management-contracts";
import { merchantFieldLabels, merchantOperatorLabels, policyFieldLabel } from "./settings/structured-rule-builder";
import { useWorkspaceView } from "./workspace-view";
import type { DemoDirectory, DemoTransaction } from "./merchant-demo";
import type { PolicyField } from "./settings/rule-composer";
import type { SimulationProduct } from "./types";

type ScenarioSource = "checkout" | "simulation";

type ScenarioInputs = {
  source: ScenarioSource;
  product?: SimulationProduct;
  amountMinor?: number;
  customerEmail?: string;
  accountAgeDays?: string;
  customSignals?: DemoCustomSignalValues;
  simulateTimeout: boolean;
};

type TestPolicyCondition = { field: string; operator: string; value?: string | number | boolean };

export type DemoCustomSignalValues = Record<string, string>;

export type DemoScenarioContext = {
  products: string[];
  fieldRegistry?: PolicyField[];
  platformRuleVersion?: string;
  unavailablePlatformSignals?: string[];
  jevRules: {
    version: string;
    rules: {
      id: string;
      name: string;
      action: string;
      conditions: { signal: string; operator: string; value: number | null }[];
    }[];
  };
  policies: {
    id: string;
    name: string;
    action: string;
    version: number;
    products: string[];
    conditions: { logic: "AND" | "OR"; rules: TestPolicyCondition[] };
    expiresAt?: string;
  }[];
};

type ScenarioAttempt = Omit<ScenarioInputs, "customerEmail" | "accountAgeDays" | "customSignals"> & {
  id: string;
  customerEmailDomain?: string;
  customerMailboxDigest?: string;
  accountAgeDays?: number;
  customSignals?: Record<string, string | number | boolean>;
};

const action = "rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50";
const secondary = "rounded-lg border border-line bg-white px-3 py-2 text-sm font-semibold hover:bg-paper disabled:opacity-50";
const catalog = Object.keys(merchantPolicyProducts) as SimulationProduct[];
const emptyProducts: SimulationProduct[] = [];

function sourceProducts(source: ScenarioSource, declared: string[]) {
  return catalog.filter(product => (source === "simulation" || product !== "payment_gateway") &&
    declared.includes(product));
}

function customFields(context?: DemoScenarioContext, product?: SimulationProduct) {
  if (!context || !product) return [];
  const usesDescription = context.policies.some(policy => policy.products.includes(product) &&
    policy.conditions.rules.some(condition => condition.field === "transaction.description"));
  return context.fieldRegistry?.filter(field => field.supportedProductTypes.includes(product) &&
    (field.fieldName.startsWith("custom_") || (usesDescription && field.fieldName === "transaction.description"))) ?? [];
}

function readAccountAge(input?: string) {
  const value = readDemoAmount(input ?? "");
  if (value !== undefined && (Number.isNaN(value) || value > 36500)) {
    throw new Error("Enter a whole account age from 0 to 36,500 days or leave it empty.");
  }
  return value;
}

function readCustomSignals(context: DemoScenarioContext | undefined, product: SimulationProduct, entries: DemoCustomSignalValues = {}) {
  const values: Record<string, string | number | boolean> = {};
  for (const field of customFields(context, product)) {
    const value = entries[field.fieldName]?.trim();
    if (!value) continue;
    const label = policyFieldLabel(field);
    switch (field.dataType) {
      case "number": {
        const number = Number(value);
        if (!/^-?\d+$/.test(value) || !Number.isSafeInteger(number)) throw new Error(`Enter a whole number for ${label}.`);
        values[field.fieldName] = number;
        break;
      }
      case "boolean":
        if (value !== "true" && value !== "false") throw new Error(`Choose Yes or No for ${label}.`);
        values[field.fieldName] = value === "true";
        break;
      case "country":
        if (!/^[A-Za-z]{2}$/.test(value)) throw new Error(`Enter a two-letter country code for ${label}.`);
        values[field.fieldName] = value.toUpperCase();
        break;
      default:
        values[field.fieldName] = value;
    }
  }
  return Object.keys(values).length ? values : undefined;
}

export function readDemoAmount(value: string) {
  const entered = value.trim();
  if (entered === "") return undefined;
  const amount = Number(entered);
  return /^\d+$/.test(entered) && Number.isSafeInteger(amount) ? amount : NaN;
}

export const invalidDemoAmount = "Enter a whole number from 0 to 9,007,199,254,740,991 or leave it empty.";

export function useDemoScenario(source: ScenarioSource) {
  const workspace = useWorkspaceView();
  const [credential, setCredential] = useState("");
  const [directory, setDirectory] = useState<DemoDirectory | null>(null);
  const [accessToken, setAccessToken] = useState("");
  const [merchantId, setMerchantId] = useState("");
  const [selectedProduct, setSelectedProduct] = useState<SimulationProduct>();
  const [loadedContext, setLoadedContext] = useState<{ merchantId: string; token: string; revision: number; data: DemoScenarioContext }>();
  const [contextError, setContextError] = useState("");
  const [contextRevision, setContextRevision] = useState(0);
  const [connecting, setConnecting] = useState(true);
  const [requiresToken, setRequiresToken] = useState(false);
  const [connectionError, setConnectionError] = useState("");
  const [selectionError, setSelectionError] = useState("");
  const [pending, setPending] = useState<ScenarioAttempt | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<DemoTransaction | null>(null);
  const [submissionError, setSubmissionError] = useState("");
  const requestGeneration = useRef(0);
  const connectionGeneration = useRef(0);
  const contextGeneration = useRef(0);
  const submittingRef = useRef(false);
  const merchantRestricted = workspace === "merchant";
  const admin = !merchantRestricted && directory?.role === "admin";
  const merchant = admin ? directory.merchants.find(option => option.id === merchantId) : undefined;
  const context = admin && loadedContext?.merchantId === merchantId && loadedContext.token === accessToken
    ? loadedContext.data : undefined;
  const products = context ? sourceProducts(source, context.products) : emptyProducts;
  const product = selectedProduct && products.includes(selectedProduct) ? selectedProduct : products[0];
  const contextLoading = Boolean(merchant && !contextError && (!context || loadedContext?.revision !== contextRevision));
  const ready = Boolean(context && product && !contextLoading && !contextError);

  const clearAttempt = useCallback(() => {
    requestGeneration.current++;
    submittingRef.current = false;
    setPending(null);
    setSubmitting(false);
    setResult(null);
    setSubmissionError("");
  }, []);

  const connect = useCallback(async (token?: string) => {
    const automatic = token === undefined;
    const currentToken = token?.trim() ?? getMerchantDemoToken();
    if (!automatic && !currentToken) return;
    const generation = ++connectionGeneration.current;
    contextGeneration.current++;
    setConnecting(true);
    setConnectionError("");
    setSelectionError("");
    clearAttempt();
    setDirectory(null);
    setAccessToken("");
    setMerchantId("");
    setSelectedProduct(undefined);
    setLoadedContext(undefined);
    setContextError("");

    try {
      if (merchantRestricted || getMerchantDemoRole() === "merchant") return;
      const local = automatic && !currentToken ? await loadLocalMerchantDirectory() : null;
      if (connectionGeneration.current !== generation) return;
      setRequiresToken(!local);
      if (!local && !currentToken) return;
      const connected = local ?? await merchantDemoRequest<DemoDirectory>("", currentToken);
      if (connectionGeneration.current !== generation) return;
      if (connected.role !== "admin") {
        setConnectionError("Use a Platform Admin token to test merchant rules.");
        return;
      }
      const search = new URLSearchParams(window.location.search);
      const targetMerchant = search.get("merchant");
      const selected = targetMerchant
        ? connected.merchants.find(option => option.id === targetMerchant)
        : connected.merchants.length === 1 ? connected.merchants[0] : undefined;
      setDirectory(connected);
      setAccessToken(currentToken);
      if (currentToken) setMerchantDemoToken(currentToken, connected.role);
      setCredential("");
      setMerchantId(selected?.id ?? "");
      if (targetMerchant && !selected) setSelectionError("The linked merchant is unavailable with this token. Choose an accessible merchant.");
    } catch (cause) {
      if (connectionGeneration.current === generation) {
        setConnectionError(cause instanceof Error ? cause.message : "The merchant directory is unavailable.");
      }
    } finally {
      if (connectionGeneration.current === generation) setConnecting(false);
    }
  }, [clearAttempt, merchantRestricted]);

  useEffect(() => {
    if (!workspace) return;
    void connect();

    return () => {
      requestGeneration.current++;
      connectionGeneration.current++;
      contextGeneration.current++;
    };
  }, [connect, workspace]);

  useEffect(() => {
    if (!admin || !merchantId) return;
    const controller = new AbortController();
    const generation = contextGeneration.current;
    void merchantDemoRequest<DemoScenarioContext>(`/${encodeURIComponent(merchantId)}/test-context`, accessToken, {
      signal: controller.signal,
    }).then(data => {
      if (controller.signal.aborted || contextGeneration.current !== generation) return;
      setLoadedContext({ merchantId, token: accessToken, revision: contextRevision, data });
    }).catch(cause => {
      if (!controller.signal.aborted && contextGeneration.current === generation) {
        setContextError(cause instanceof Error ? cause.message : "Could not load the merchant's test setup.");
      }
    });
    return () => controller.abort();
  }, [accessToken, admin, contextRevision, merchantId]);

  function disconnect() {
    connectionGeneration.current++;
    contextGeneration.current++;
    clearAttempt();
    setMerchantDemoToken("");
    setDirectory(null);
    setAccessToken("");
    setMerchantId("");
    setSelectedProduct(undefined);
    setLoadedContext(undefined);
    setContextError("");
    setConnecting(false);
    setConnectionError("");
    setSelectionError("");
  }

  function selectMerchant(id: string) {
    if (merchantId === id) return;
    contextGeneration.current++;
    clearAttempt();
    setSelectionError("");
    setMerchantId(id);
    setSelectedProduct(undefined);
    setLoadedContext(undefined);
    setContextError("");
  }

  function selectProduct(id: SimulationProduct) {
    if (!products.includes(id) || id === product) return;
    clearAttempt();
    setSelectedProduct(id);
  }

  function refreshContext() {
    contextGeneration.current++;
    setContextError("");
    setContextRevision(value => value + 1);
  }

  async function submit({ customerEmail, accountAgeDays, customSignals, ...inputs }: ScenarioInputs) {
    if (!ready || !merchant || !product || inputs.source !== source ||
        (inputs.product && inputs.product !== product) || submittingRef.current || getMerchantDemoRole() === "merchant") return;
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
        product,
        id: crypto.randomUUID(),
        customerEmailDomain: email.domain,
        accountAgeDays: readAccountAge(accountAgeDays),
        customSignals: readCustomSignals(context, product, customSignals),
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
    credential, setCredential, directory, admin, merchantRestricted, merchantId, merchant,
    connecting, connectionError, selectionError, requiresToken,
    context, contextLoading, contextError, refreshContext,
    product, products, selectProduct, source, ready, connect, disconnect, selectMerchant,
    pending, submitting, result, submissionError, submit, clearAttempt,
  };
}

type DemoScenarioState = ReturnType<typeof useDemoScenario>;

export function DemoScenarioPicker({ flow }: { flow: DemoScenarioState }) {
  const tokenId = useId();
  if (flow.merchantRestricted) return <p className="text-sm text-muted">
    Platform Admin can test merchant rules. <Link href="/settings/merchants?view=transactions" className="font-semibold text-accent underline underline-offset-2">Open transactions</Link>
  </p>;
  return <div className="space-y-3">
    {!flow.directory && !flow.requiresToken && (flow.connecting
      ? <p role="status" className="text-xs text-muted">Loading merchants…</p>
      : <button type="button" className={secondary} onClick={() => void flow.connect()}>
        {flow.connectionError ? "Retry merchant directory" : "Use merchant rules"}
      </button>)}
    {!flow.directory && flow.requiresToken && <>
      <p className="text-xs leading-relaxed text-muted">
        Connect with a Platform Admin token to select a merchant.
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
    {flow.connectionError && <p role="alert" className="text-sm text-red-800">{flow.connectionError}</p>}
    {flow.admin && flow.directory && <>
      <label className="block max-w-md text-sm font-semibold">Merchant
        <select className="control mt-1.5" value={flow.merchantId} onChange={event => flow.selectMerchant(event.currentTarget.value)}>
          <option value="">Select merchant</option>
          {flow.directory.merchants.map(merchant => <option key={merchant.id} value={merchant.id}>{merchant.name}</option>)}
        </select>
      </label>
      {flow.selectionError && <p role="alert" className="text-sm text-red-800">{flow.selectionError}</p>}
      {!flow.directory.merchants.length && <p className="text-sm text-muted">
        No merchants are available. <Link href="/settings/merchants" className="font-semibold text-accent underline underline-offset-2">Open Merchants</Link>
      </p>}
      {flow.requiresToken && <button type="button" className={secondary} onClick={flow.disconnect}>Disconnect merchant directory</button>}
    </>}
  </div>;
}

export function DemoScenarioEmailField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const helpId = useId();
  return <label className="block min-w-0 text-sm font-semibold">Synthetic email or domain
    <input className="control mt-1.5" value={value} maxLength={320} placeholder="buyer@example.test (optional)"
      autoComplete="off" autoCapitalize="none" spellCheck={false} aria-describedby={helpId}
      onChange={event => onChange(event.currentTarget.value)} />
    <span id={helpId} className="mt-1.5 block text-xs font-normal text-muted">Enter the full synthetic address to test an exact-email block. A bare domain only supplies domain evidence.</span>
  </label>;
}

export function DemoScenarioAccountAgeField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return <label className="block min-w-0 text-sm font-semibold">Account age in days
    <input className="control mt-1.5" inputMode="numeric" value={value} maxLength={5} placeholder="Optional"
      autoComplete="off" onChange={event => onChange(event.currentTarget.value)} />
  </label>;
}

export function DemoScenarioCustomSignals({ flow, values, onChange }: {
  flow: DemoScenarioState;
  values: DemoCustomSignalValues;
  onChange: (values: DemoCustomSignalValues) => void;
}) {
  const fields = customFields(flow.context, flow.product);
  if (!fields.length) return null;
  return <fieldset className="grid gap-3 sm:col-span-2 sm:grid-cols-2">
    <legend className="mb-1 text-sm font-semibold">Additional rule evidence</legend>
    {fields.some(field => field.fieldName === "transaction.description") && <p className="text-xs text-muted sm:col-span-2">
      Use non-identifying test details in the transaction description. Jev may use it to assess meaning.
    </p>}
    {fields.map(field => {
      const name = field.fieldName;
      const fieldValue = values[name] ?? "";
      function update(value: string) {
        onChange({ ...values, [name]: value });
      }
      return <label key={name} className="block min-w-0 text-sm font-semibold">
        {policyFieldLabel(field)}
        {field.dataType === "boolean" ? <select className="control mt-1.5" value={fieldValue} onChange={event => update(event.currentTarget.value)}>
          <option value="">Not supplied</option>
          <option value="true">Yes</option>
          <option value="false">No</option>
        </select> : <input className="control mt-1.5" value={fieldValue}
          inputMode={field.dataType === "number" ? "numeric" : "text"} placeholder={field.dataType === "country" ? "e.g. IN (optional)" : "Optional"}
          maxLength={field.dataType === "country" ? 2 : 120} autoComplete="off" onChange={event => update(event.currentTarget.value)} />}
      </label>;
    })}
  </fieldset>;
}

const jevTestSignals: Record<string, string> = {
  amountMinor: "Amount in minor units",
};
function describePolicyCondition(condition: TestPolicyCondition, fieldRegistry: PolicyField[]) {
  if (condition.field === "customer.mailbox_ref") return "Exact email address equals the specified address";
  const value = String(condition.value ?? "");
  const productLabel = merchantPolicyProducts[value as SimulationProduct];
  const label = condition.field === "product.type" ? productLabel ?? value : value;
  const customField = fieldRegistry.find(field => field.fieldName === condition.field);
  const field = customField ? policyFieldLabel(customField) : merchantFieldLabels[condition.field] ?? condition.field;
  return `${field} ${merchantOperatorLabels[condition.operator] ?? condition.operator.replaceAll("_", " ")} ${label}${condition.operator === "semantic_match" ? " (Jev)" : ""}`;
}

export function DemoScenarioPolicyContext({ flow }: { flow: DemoScenarioState }) {
  const { merchant, context, contextLoading, contextError, product, products, source } = flow;
  if (!merchant) return null;
  if (contextLoading) return <p role="status" className="text-sm text-muted">Loading merchant policies and products…</p>;
  if (contextError) return <div className="space-y-2 text-sm">
    <p role="alert" className="text-red-800">{contextError}</p>
    <button type="button" className={secondary} onClick={flow.refreshContext}>Retry loading rule setup</button>
  </div>;
  if (!context) return null;
  const policies = context.policies.filter(policy => product && policy.products.includes(product));
  return <section className="space-y-3 rounded-lg border border-line bg-paper p-3 text-sm" aria-label="Rules for this test">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h3 className="font-semibold">Rules for this test</h3>
      <button type="button" className={secondary} onClick={flow.refreshContext} disabled={flow.pending !== null || flow.submitting}>Refresh rules</button>
    </div>
    {context.platformRuleVersion && <div className="space-y-1">
      <p className="font-semibold">Platform rules · {context.platformRuleVersion}</p>
      {!!context.unavailablePlatformSignals?.length && <p className="text-xs text-muted">
        This test can supply account age. Signals that require earlier activity or real payment data are unavailable.
      </p>}
    </div>}
    {!products.length && <p className="text-muted">
      {source === "checkout" && context.products.includes("payment_gateway") ? <>
        Gateway payments run in Simulation Lab.{" "}
        <Link href={`/simulation-lab?merchant=${encodeURIComponent(merchant.id)}#merchant-rules`}
          className="font-semibold text-accent underline underline-offset-2">Open Simulation Lab</Link>
      </> : <>
        This merchant has no supported products for this test.{" "}
        <Link href={merchantDetailsHref(merchant.id)} className="font-semibold text-accent underline underline-offset-2">Open merchant details</Link>
      </>}
    </p>}
    {product && (policies.length || context.jevRules.rules.length ? <ul className="space-y-2">
      {policies.map(policy => <li key={policy.id} className="space-y-1 rounded-md border border-line bg-white p-2.5 wrap-anywhere">
        <p className="font-semibold">{policy.name} · v{policy.version} · {policyActionLabels[policy.action] ?? policy.action}</p>
        <p className="text-xs text-muted">
          {policy.conditions.rules.map(condition => describePolicyCondition(condition, context.fieldRegistry ?? [])).join(policy.conditions.logic === "OR" ? " or " : " and ")}
        </p>
        {policy.expiresAt && <p className="text-xs text-muted">Expires <time dateTime={policy.expiresAt}>{merchantDate(policy.expiresAt)}</time></p>}
      </li>)}
      {context.jevRules.rules.map(rule => <li key={rule.id} className="space-y-1 rounded-md border border-line bg-white p-2.5 wrap-anywhere">
        <p className="font-semibold">{rule.name} · {context.jevRules.version} · {policyActionLabels[rule.action] ?? rule.action}</p>
        <p className="text-xs text-muted">{rule.conditions.map(({ signal, operator, value }) =>
          `${jevTestSignals[signal] ?? signal} ${merchantOperatorLabels[operator] ?? operator} ${value ?? ""}`).join(" and ")}</p>
      </li>)}
    </ul> : <p className="text-muted">No active merchant rules apply to {merchantPolicyProducts[product]}.</p>)}
  </section>;
}

const reviewLabels = { NONE: "No manual review", NEEDS_DECISION: "Needs decision", RESOLVED: "Resolved" } as const;

export function DemoScenarioResult({ transaction, merchantName }: { transaction: DemoTransaction; merchantName: string }) {
  const final = transaction.policyDecision?.finalDecision;
  const result = final ? policyActionLabels[final.action] ?? final.action : merchantOutcomeLabels[transaction.outcome];
  const allowed = final ? final.action === "ALLOW" : transaction.outcome === "LOW_RISK";
  const title = final ? "Transaction decision" : "Jev result";
  const merchantReview = final?.source === "MERCHANT_POLICY" && final.action === "REVIEW" &&
    transaction.policyDecision?.platformDecision.action === "ALLOW";
  return <section className="rounded-xl border border-line bg-white p-4 sm:p-5" aria-label={title}>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h3 className="font-semibold">{title}</h3>
      <p role="status" className={`rounded-full px-3 py-1 text-xs font-semibold ${allowed
        ? "bg-emerald-50 text-emerald-800" : "bg-amber-50 text-amber-900"}`}>{result}</p>
    </div>
    {transaction.policyDecision && <div className="mt-3"><PolicyDecisionDetails decision={transaction.policyDecision} /></div>}
    <div className="mt-3"><RiskScoreDetails score={transaction.policyDecision?.riskScore} /></div>
    <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
      {final && <div><dt className="text-xs text-muted">Risk result</dt><dd className="font-semibold">{merchantOutcomeLabels[transaction.outcome]}</dd></div>}
      <div className="min-w-0"><dt className="text-xs text-muted">Merchant</dt><dd className="break-words font-semibold">{merchantName}</dd></div>
      {transaction.scenarioName && <div className="min-w-0"><dt className="text-xs text-muted">Scenario</dt><dd className="break-words font-semibold">{transaction.scenarioName} · v{transaction.scenarioVersion}</dd></div>}
      {transaction.product && <div><dt className="text-xs text-muted">Product</dt><dd className="font-semibold">{merchantPolicyProducts[transaction.product as SimulationProduct] ?? transaction.product}</dd></div>}
      {transaction.ruleVersion && <div><dt className="text-xs text-muted">Rule set</dt><dd className="font-semibold">{transaction.ruleVersion}</dd></div>}
      {transaction.jevRuleVersion && <div><dt className="text-xs text-muted">Merchant Jev rules</dt><dd className="font-semibold">{transaction.jevRuleVersion}</dd></div>}
      <MerchantJevEvidence transaction={transaction} />
      <div><dt className="text-xs text-muted">Booking amount</dt><dd className="font-semibold">{transaction.amountMinor === null ? "Unavailable" : `${transaction.amountMinor.toLocaleString()} minor units`}</dd></div>
      <div className="min-w-0"><dt className="text-xs text-muted">{transaction.scenarioName ? "Scenario and Jev rule hits" : "Jev rule hits"}</dt><dd className="break-words font-semibold">{transaction.ruleHits.join(", ") || "None"}</dd></div>
      <div><dt className="text-xs text-muted">Transaction state</dt><dd className="font-semibold">{reviewLabels[transaction.reviewState]}</dd></div>
      <div className="col-span-full">
        <dt className="text-xs text-muted">{merchantReview ? "Merchant review reason" : "Assessment reason"}</dt>
        <dd>{transaction.reason}</dd>
      </div>
      {transaction.jevReason && transaction.jevReason !== transaction.reason && <div className="col-span-full">
        <dt className="text-xs text-muted">Jev reason</dt>
        <dd>{transaction.jevReason}</dd>
      </div>}
    </dl>
    <Link href={merchantDetailsHref(transaction.merchantId, "transactions", transaction.id)} className="mt-4 inline-block text-sm font-semibold text-accent underline underline-offset-2">View merchant transaction</Link>
  </section>;
}

export function DemoScenarioSimulation() {
  const flow = useDemoScenario("simulation");
  return <section className="space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby="merchant-rule-simulation">
    <div>
      <h2 id="merchant-rule-simulation" className="font-semibold">Merchant rule simulation</h2>
      <p className="mt-1 text-sm text-muted">Run the selected merchant's rules on one synthetic transaction and save the decision to its ledger.</p>
    </div>
    <DemoScenarioPicker flow={flow} />
    <DemoScenarioPolicyContext flow={flow} />
    {flow.context && flow.product && <ScenarioSimulationForm key={flow.merchantId} flow={flow} />}
    {flow.merchant && flow.result && <DemoScenarioResult transaction={flow.result} merchantName={flow.merchant.name} />}
  </section>;
}

function ScenarioSimulationForm({ flow }: { flow: DemoScenarioState }) {
  const [amount, setAmount] = useState("50000");
  const [email, setEmail] = useState("");
  const [accountAgeDays, setAccountAgeDays] = useState("");
  const [customSignals, setCustomSignals] = useState<DemoCustomSignalValues>({});
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
    void flow.submit({ source: "simulation", amountMinor, customerEmail: email, accountAgeDays, customSignals, simulateTimeout });
  }

  return <form onSubmit={submit} noValidate className="space-y-3">
    <fieldset className="grid gap-3 sm:grid-cols-2" disabled={flow.submitting || flow.pending !== null || !flow.ready}>
      <label className="text-sm font-semibold">Product
        <select className="control mt-1.5" value={flow.product} onChange={event => flow.selectProduct(event.currentTarget.value as SimulationProduct)}>
          {flow.products.map(key => <option key={key} value={key}>{merchantPolicyProducts[key]}</option>)}
        </select>
      </label>
      <DemoScenarioEmailField value={email} onChange={value => {
        flow.clearAttempt();
        setEmail(value);
      }} />
      <DemoScenarioAccountAgeField value={accountAgeDays} onChange={value => {
        flow.clearAttempt();
        setAccountAgeDays(value);
      }} />
      <label htmlFor={amountId} className="text-sm font-semibold">Booking amount in minor units
        <input id={amountId} className="control mt-1.5" inputMode="numeric" value={amount} placeholder="Optional"
          aria-invalid={!!amountError} aria-describedby={amountError ? `${amountId}-error` : undefined} onChange={event => {
            flow.clearAttempt();
            setAmount(event.currentTarget.value);
            setAmountError("");
          }} />
        {amountError && <span role="alert" id={`${amountId}-error`} className="mt-1 block text-xs text-red-800">{amountError}</span>}
      </label>
      <label className="text-sm font-semibold">Jev response
        <select className="control mt-1.5" value={simulateTimeout ? "timeout" : "mock"}
          onChange={event => {
            flow.clearAttempt();
            setSimulateTimeout(event.currentTarget.value === "timeout");
          }}>
          <option value="mock">Use configured Jev</option>
          <option value="timeout">Simulate timeout</option>
        </select>
      </label>
      <DemoScenarioCustomSignals flow={flow} values={customSignals} onChange={values => {
        flow.clearAttempt();
        setCustomSignals(values);
      }} />
    </fieldset>
    {flow.submissionError && <p role="alert" className="text-sm text-red-800">{flow.submissionError}</p>}
    <div className="flex flex-wrap gap-2">
      <button className={action} type="submit" disabled={!flow.ready || flow.submitting}>
        {flow.submitting ? "Assessing…" : flow.pending ? "Retry merchant simulation" : "Run merchant simulation"}
      </button>
      {flow.pending && !flow.submitting && <button type="button" className={secondary} onClick={flow.clearAttempt}>Start a new test</button>}
    </div>
  </form>;
}
