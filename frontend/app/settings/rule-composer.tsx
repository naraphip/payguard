"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState, useSyncExternalStore } from "react";
import type { FormEvent } from "react";
import { getMerchantDemoRole, getMerchantDemoToken, merchantDemoRequest, merchantPolicyProducts, policyActionLabels, setMerchantDemoToken, subscribeMerchantDemoSession } from "../merchant-demo";
import type { DemoDirectory } from "../merchant-demo";
import { useWorkspaceView } from "../workspace-view";
import type { ReactNode } from "react";
import { outlineButton, primaryButton, ruleRequest, rulesEndpoint, stages } from "./system-rules/contracts";
import type { Preview, PreviewRun, Rule, RuleCatalogue, RuleInput } from "./system-rules/contracts";

const composer = "/api/v1/booking/rule-composer";
const policies = "/api/v1/booking/demo/merchant-policy";
const panel = "min-w-0 space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5";

type Scope = "MERCHANT" | "PLATFORM";
type PolicyAction = "BLOCK" | "REVIEW" | "CHALLENGE";
type PolicyCondition = { field: string; operator: string; value: string | number | boolean };
type MerchantProposal = {
  scope: "MERCHANT";
  name: string;
  product_types: string[];
  stages: string[];
  conditions: { logic: "AND" | "OR"; rules: PolicyCondition[] };
  action: PolicyAction;
};
type PlatformProposal = { scope: "PLATFORM"; rule: RuleInput };
type Proposal = MerchantProposal | PlatformProposal;
type Interpretation = {
  status: "READY" | "CLARIFICATION";
  proposal?: Proposal;
  message?: string;
  validation: { valid: boolean; errors?: string[]; warnings?: string[] };
};

type PolicyPreview = { evaluated: number; matched: number; missing: number; conflictRuleIds?: string[]; approvalReason?: string };
type PolicyRule = {
  id: string;
  version: number;
  activeVersion?: number;
  status: "ACTIVE" | "PENDING_APPROVAL" | "DISABLED";
  proposal: MerchantProposal;
  preview: PolicyPreview;
};
type PolicySave = { rule: PolicyRule; status: PolicyRule["status"] };
export type MerchantPolicyVersion = PolicyRule & { createdAt?: string; createdBy?: string; actor?: string; changeReason?: string };
type PlatformSave = { rule: Rule; status: "DRAFT"; preview?: Preview };
type BusinessProfile = {
  businessName: string;
  products: string[];
  stages: string[];
  signals: string[];
  scoringProfileVersion: string;
};

const scopeLabels = { MERCHANT: "Merchant policy", PLATFORM: "Platform fraud rule" };
const products: Record<string, string> = merchantPolicyProducts;
const fields: Record<string, string> = {
  "product.type": "Product",
  "customer.email_domain": "Customer email domain",
  "order.amount_minor": "Order amount in minor units",
  account_age_days: "Account age in days",
  device_orders: "Orders from this device in 60 seconds",
  ip_orders: "Orders from this IP in 60 seconds",
  payment_token_orders: "Orders using this payment token in 60 seconds",
  payment_token_missing: "Payment token is missing",
  shared_device_or_token_orders: "Maximum orders using the device or token in 60 seconds",
};
const operatorLabels: Record<string, string> = {
  eq: "equals", ne: "does not equal", gt: "is greater than", gte: "is at least", lt: "is less than", lte: "is at most",
};
const statusLabels = {
  ACTIVE: "Active", PENDING_APPROVAL: "Pending approval", DISABLED: "Disabled", DRAFT: "Draft",
};
const prompts: Record<Scope, string> = {
  MERCHANT: "If a customer books a flight ticket using an email address from skycannner.com, block the booking.",
  PLATFORM: "Send a booking for review if the device placed at least four orders within 60 seconds.",
};

const emptyToken = () => "";
const noRole = () => null;

export function useRuleCredential(scope: Scope) {
  const session = useSyncExternalStore(subscribeMerchantDemoSession, getMerchantDemoToken, emptyToken);
  const role = useSyncExternalStore(subscribeMerchantDemoSession, getMerchantDemoRole, noRole);
  return role === (scope === "MERCHANT" ? "merchant" : "admin") ? session : "";
}

function errorText(cause: unknown) {
  return cause instanceof Error ? cause.message : "The rule service is unavailable. Try again.";
}

function ProfileDetails({ profile }: { profile: BusinessProfile }) {
  return <dl className="mt-2 grid gap-2 text-sm text-muted sm:grid-cols-3">
    <div><dt className="font-medium text-ink">Products</dt><dd>{profile.products?.map(product => products[product] ?? product).join(", ") || "Unavailable"}</dd></div>
    <div><dt className="font-medium text-ink">Available signals</dt><dd>{profile.signals?.map(field => fields[field] ?? field).join(", ") || "Unavailable"}</dd></div>
    <div><dt className="font-medium text-ink">Scoring profile</dt><dd>{profile.scoringProfileVersion || "Unavailable"}</dd></div>
  </dl>;
}

async function authenticatedRequest<T>(path: string, token: string, init?: RequestInit) {
  const headers = new Headers(init?.headers);
  headers.set("Authorization", `Bearer ${token}`);
  if (init?.body) headers.set("Content-Type", "application/json");
  return ruleRequest<T>(path, { ...init, headers, cache: "no-store" });
}

export function RuleAccess({ scope }: { scope: Scope }) {
  const token = useRuleCredential(scope);
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<AbortController>(null);
  const id = useId();
  const role = scope === "MERCHANT" ? "merchant" : "admin";
  const roleLabel = scope === "MERCHANT" ? "Merchant" : "Platform administrator";

  useEffect(() => () => request.current?.abort(), []);

  async function connect(event: FormEvent) {
    event.preventDefault();
    const candidate = value.trim();
    if (!candidate || request.current) return;
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError("");
    try {
      const directory = await merchantDemoRequest<DemoDirectory>("", candidate, { signal: controller.signal });
      if (controller.signal.aborted) return;
      if (directory.role !== role) throw new Error(`Use a valid ${roleLabel.toLowerCase()} token for this workspace.`);
      setValue("");
      setMerchantDemoToken(candidate, role);
    } catch (cause) {
      if (!controller.signal.aborted) setError(errorText(cause));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
      request.current = null;
    }
  }

  if (token) return <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
    <p className="font-semibold text-emerald-800">{roleLabel} connected</p>
    <button type="button" className={outlineButton} onClick={() => setMerchantDemoToken("")}>Disconnect</button>
  </div>;

  return <form className={panel} aria-label={`Connect as ${roleLabel.toLowerCase()}`} onSubmit={connect}>
    <div>
      <label htmlFor={id} className="text-sm font-semibold">{roleLabel} token</label>
      <div className="mt-1.5 flex flex-wrap items-center gap-2">
        <input id={id} type="password" value={value} autoComplete="off" className="control max-w-sm"
          disabled={busy} onChange={event => setValue(event.currentTarget.value)} />
        <button type="submit" className={primaryButton} disabled={busy || !value.trim()}>{busy ? "Connecting…" : "Connect"}</button>
      </div>
    </div>
    {scope === "MERCHANT" && <p className="text-xs text-muted">Use the access token issued for your merchant.</p>}
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
  </form>;
}

export function RulePreview({ proposal }: { proposal: Proposal }) {
  const merchant = proposal.scope === "MERCHANT";
  const name = merchant ? proposal.name : proposal.rule.name;
  const action = merchant ? proposal.action : proposal.rule.action;
  const ruleStages = merchant ? proposal.stages : [proposal.rule.stage];
  const conditions = merchant ? proposal.conditions.rules : proposal.rule.conditions.map(({ signal, operator, value }) => ({ field: signal, operator, value }));
  const logic = merchant && proposal.conditions.logic === "OR" ? "Any condition" : "All conditions";
  return <div className="space-y-4 rounded-lg border border-line bg-paper p-4">
    <dl className="grid gap-x-5 gap-y-3 text-sm sm:grid-cols-2">
      <div><dt className="text-muted">Rule name</dt><dd className="font-semibold wrap-anywhere">{name}</dd></div>
      <div><dt className="text-muted">Policy source</dt><dd className="font-semibold">{scopeLabels[proposal.scope]}</dd></div>
      {merchant && <div><dt className="text-muted">Products</dt><dd>{proposal.product_types.map(product => products[product] ?? product).join(", ")}</dd></div>}
      <div><dt className="text-muted">When</dt><dd>{ruleStages.map(stage => stages[stage as keyof typeof stages] ?? stage).join(", ")}</dd></div>
      <div><dt className="text-muted">Action</dt><dd className="font-semibold">{policyActionLabels[action] ?? action}</dd></div>
    </dl>
    <div>
      <p className="text-sm font-semibold">{logic}</p>
      <ul className="mt-1 list-disc space-y-1 pl-5 text-sm">
        {conditions.map(({ field, operator, value }, index) => <li key={index} className="wrap-anywhere">
          {fields[field] ?? field} {operatorLabels[operator] ?? operator} {field === "product.type" ? products[String(value)] ?? value : String(value)}
        </li>)}
      </ul>
    </div>
    <details>
      <summary className="cursor-pointer text-xs font-semibold text-accent">Advanced rule details</summary>
      <pre className="mt-2 max-h-64 overflow-auto rounded-md border border-line bg-white p-3 text-xs">{JSON.stringify(proposal, null, 2)}</pre>
    </details>
  </div>;
}

export function RuleComposer({ scope, onSaved, edit, onCancel }: {
  scope: Scope;
  onSaved?: () => void;
  edit?: PolicyRule;
  onCancel?: () => void;
}) {
  const token = useRuleCredential(scope);
  return token && <ConnectedComposer key={`${token}:${edit?.id ?? ""}:${edit?.version ?? ""}`}
    scope={scope} token={token} onSaved={onSaved} edit={edit} onCancel={onCancel} />;
}

function ConnectedComposer({ scope, token, onSaved, edit, onCancel }: {
  scope: Scope; token: string; onSaved?: () => void; edit?: PolicyRule; onCancel?: () => void;
}) {
  const [instruction, setInstruction] = useState("");
  const [draft, setDraft] = useState<Interpretation | null>(null);
  const [pending, setPending] = useState<"interpret" | "save" | null>(null);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState<PolicySave | PlatformSave | null>(null);
  const [runId, setRunId] = useState("");
  const [runs, setRuns] = useState<PreviewRun[]>([]);
  const [runError, setRunError] = useState("");
  const [profile, setProfile] = useState<BusinessProfile | null>(null);
  const [profileError, setProfileError] = useState("");
  const request = useRef<AbortController>(null);
  const id = useId();

  useEffect(() => {
    const controller = new AbortController();
    const init = { signal: controller.signal };
    if (scope === "MERCHANT") {
      void authenticatedRequest<BusinessProfile>(`${policies}/profile`, token, init)
        .then(setProfile).catch(cause => {
          if (!controller.signal.aborted) setProfileError(errorText(cause));
        });
    } else {
      void authenticatedRequest<RuleCatalogue>(rulesEndpoint, token, init).then(result => setRuns(result.runs ?? [])).catch(cause => {
        if (!controller.signal.aborted) setRunError(errorText(cause));
      });
    }
    return () => {
      controller.abort();
      request.current?.abort();
    };
  }, [scope, token]);

  async function interpret(event: FormEvent) {
    event.preventDefault();
    if (request.current || !instruction.trim()) return;
    const controller = new AbortController();
    request.current = controller;
    setPending("interpret");
    setDraft(null);
    setSaved(null);
    setError("");
    try {
      const result = await authenticatedRequest<Interpretation>(`${composer}/interpret`, token, {
        method: "POST", body: JSON.stringify({ scope, instruction: instruction.trim() }), signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      if (result.proposal && result.proposal.scope !== scope) throw new Error("The proposal belongs to another rule system.");
      setDraft(result);
    } catch (cause) {
      if (!controller.signal.aborted) setError(errorText(cause));
    } finally {
      if (!controller.signal.aborted) setPending(null);
      request.current = null;
    }
  }

  async function save() {
    const proposal = draft?.proposal;
    if (request.current || !proposal || draft?.status !== "READY" || !draft.validation.valid) return;
    const controller = new AbortController();
    request.current = controller;
    setPending("save");
    setError("");
    try {
      const path = proposal.scope === "MERCHANT"
        ? `${policies}/rules${edit ? `/${encodeURIComponent(edit.id)}` : ""}`
        : `${composer}/platform/save`;
      const payload = edit ? { proposal, expectedVersion: edit.version }
        : proposal.scope === "PLATFORM" && runId ? { proposal, runId } : { proposal };
      const result = await authenticatedRequest<PolicySave | PlatformSave>(path, token, {
        method: edit ? "PUT" : "POST", body: JSON.stringify(payload), signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      setSaved(result);
      onSaved?.();
    } catch (cause) {
      if (!controller.signal.aborted) setError(errorText(cause));
    } finally {
      if (!controller.signal.aborted) setPending(null);
      request.current = null;
    }
  }

  const ready = draft?.status === "READY" && draft.validation.valid && Boolean(draft.proposal);
  const reason = saved?.status === "DRAFT"
    ? "Platform proposals are saved as drafts for review."
    : saved?.status === "PENDING_APPROVAL" ? "Review the validation evidence before activating this version." : "";
  const preview = saved && "preview" in saved.rule ? saved.rule.preview : saved && "preview" in saved ? saved.preview : undefined;
  const messages = [...(draft?.validation.errors ?? []), ...(draft?.validation.warnings ?? [])];

  return <section className={panel} aria-labelledby={`${id}-title`}>
    <div>
      <h2 id={`${id}-title`} className="text-lg font-semibold">AI rule composer</h2>
      <p className="mt-1 text-sm text-muted">Describe the rule in Thai or English, then review the proposal before saving.</p>
    </div>
    {profileError && <p role="alert" className="text-sm text-red-800">{profileError}</p>}
    {profile && <details className="text-sm text-muted">
      <summary className="cursor-pointer font-semibold text-ink">{profile.businessName} business profile</summary>
      <ProfileDetails profile={profile} />
    </details>}
    {edit && <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-paper p-3 text-sm">
      <p>New version of <strong>{edit.proposal.name}</strong></p>
      <button type="button" className={outlineButton} disabled={pending !== null} onClick={onCancel}>{saved ? "Done" : "Cancel edit"}</button>
    </div>}
    <form className="space-y-2.5" onSubmit={interpret}>
      <label htmlFor={`${id}-instruction`} className="block text-sm font-semibold">Describe your rule</label>
      <textarea id={`${id}-instruction`} rows={3} maxLength={1000} className="control min-h-24 resize-y"
        aria-describedby={`${id}-instruction-help`} placeholder={prompts[scope]} value={instruction} disabled={pending !== null} onChange={event => {
          setInstruction(event.currentTarget.value);
          setDraft(null);
          setSaved(null);
          setError("");
        }} />
      <p id={`${id}-instruction-help`} className="text-xs text-muted">Use policy conditions and domains. Leave out customer identities and card details.</p>
      <button type="submit" className={ready ? outlineButton : primaryButton} disabled={!instruction.trim() || pending !== null}>
        {pending === "interpret" ? "Generating preview…" : "Generate preview"}
      </button>
    </form>
    {error && <p role="alert" className="rounded-lg bg-red-50 p-3 text-sm text-red-800">{error}</p>}
    {draft && <div className="space-y-3">
      {draft.message && <p role="status" className="text-sm text-muted">{draft.message}</p>}
      {messages.length > 0 && <ul className="list-disc space-y-1 pl-5 text-sm text-amber-900">
        {messages.map((message, index) => <li key={index}>{message}</li>)}
      </ul>}
      {draft.proposal && <RulePreview proposal={draft.proposal} />}
      {ready && !saved && <>
        {scope === "PLATFORM" && runs.length > 0 && <label className="block text-sm font-semibold">Simulation run for replay (optional)
          <select className="control mt-1 max-w-md" value={runId} onChange={event => setRunId(event.currentTarget.value)} disabled={pending !== null}>
            <option value="">Save as draft without a replay</option>
            {runs.map(run => <option key={run.id} value={run.id}>{products[run.product] ?? run.product} · Seed {run.seed} · {run.id.slice(0, 8)}</option>)}
          </select>
        </label>}
        {scope === "PLATFORM" && runError && <p className="text-sm text-muted">Simulation runs could not be loaded. {runError}</p>}
        <button type="button" className={primaryButton} disabled={pending !== null} onClick={() => void save()}>
          {pending === "save" ? "Saving…" : "Save Rule"}
        </button>
      </>}
    </div>}
    {saved && <div role="status" className="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900">
      <p className="font-semibold">Rule saved · {statusLabels[saved.status]}</p>
      {reason && <p className="mt-1">{reason}</p>}
      {preview && ("evaluated" in preview ? <PolicySample preview={preview} />
        : <p className="mt-1">{preview.matched} matched out of {preview.eligible} eligible events · {preview.skipped} with missing signals</p>)}
    </div>}
  </section>;
}

type RuleHistory = { rule: PolicyRule; versions: PolicyRule[] };

function ConnectedPolicyRules({ token, refreshKey, onEdit }: {
  token: string; refreshKey: number; onEdit: (rule: PolicyRule) => void;
}) {
  const [rules, setRules] = useState<PolicyRule[]>([]);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [history, setHistory] = useState<RuleHistory | null>(null);
  const [simulation, setSimulation] = useState<{ id: string; preview: PolicyPreview } | null>(null);
  const lifecycle = useRef<AbortController>(null);

  useEffect(() => () => lifecycle.current?.abort(), []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void authenticatedRequest<{ rules: PolicyRule[] }>(`${policies}/rules`, token, { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) setRules(result.rules ?? []); })
      .catch(cause => { if (!controller.signal.aborted) setError(errorText(cause)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [token, refreshKey, revision]);

  async function change(rule: PolicyRule, operation: "disable" | "activate" | "rollback" | "simulate" | "history", version?: number) {
    if (lifecycle.current) return;
    const controller = new AbortController();
    lifecycle.current = controller;
    setBusy(rule.id);
    setError("");
    setNotice("");
    const path = `${policies}/rules/${encodeURIComponent(rule.id)}`;
    const { signal } = controller;
    try {
      if (operation === "history") {
        const result = await authenticatedRequest<RuleHistory>(path, token, { signal });
        if (!signal.aborted) setHistory(result);
      } else if (operation === "simulate") {
        const result = await authenticatedRequest<{ preview: PolicyPreview }>(`${path}/simulate`, token, { method: "POST", signal });
        if (!signal.aborted) setSimulation({ id: rule.id, preview: result.preview });
      } else {
        const result = await authenticatedRequest<PolicySave>(`${path}/${operation}`, token, {
          method: "POST", body: JSON.stringify({ version, expectedVersion: rule.version }), signal,
        });
        if (signal.aborted) return;
        setHistory(null);
        setSimulation(null);
        setNotice(`${rule.proposal.name} · ${statusLabels[result.status]}`);
        setRevision(value => value + 1);
      }
    } catch (cause) {
      if (!signal.aborted) setError(errorText(cause));
    } finally {
      if (!signal.aborted) setBusy("");
      lifecycle.current = null;
    }
  }

  return <section className={panel} aria-labelledby="merchant-policy-list">
    <h2 id="merchant-policy-list" className="text-lg font-semibold">Saved policies</h2>
    {loading && <p role="status" className="text-sm text-muted">Loading policies…</p>}
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
    {notice && <p role="status" className="text-sm text-emerald-900">{notice}</p>}
    {!loading && !error && rules.length === 0 && <p className="text-sm text-muted">No merchant policies saved.</p>}
    {rules.length > 0 && <ul className="divide-y divide-line">
      {rules.map(rule => <li className="space-y-3 py-3" key={rule.id}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold wrap-anywhere">{rule.proposal.name}</p>
            <p className="mt-1 text-xs text-muted">{statusLabels[rule.status]} · Version {rule.version} · {policyActionLabels[rule.proposal.action]}
              {rule.activeVersion && rule.activeVersion !== rule.version ? ` · Version ${rule.activeVersion} remains active` : ""}
            </p>
            {rule.preview && <PolicySample preview={rule.preview} />}
          </div>
          <div className="flex flex-wrap gap-2">
            <button type="button" className={outlineButton} disabled={Boolean(busy)} onClick={() => {
              onEdit(rule);
              document.getElementById("merchant-policy-editor")?.focus();
            }}>Edit</button>
            <button type="button" className={outlineButton} disabled={Boolean(busy)} onClick={() => void change(rule, "simulate")}>Test rule</button>
            <button type="button" className={outlineButton} disabled={Boolean(busy)} onClick={() => {
              if (history?.rule.id === rule.id) setHistory(null);
              else void change(rule, "history");
            }}>{history?.rule.id === rule.id ? "Hide versions" : "Versions"}</button>
            {(rule.status === "ACTIVE" || rule.activeVersion) && <button type="button" className={outlineButton} disabled={Boolean(busy)}
              onClick={() => void change(rule, "disable")}>Disable</button>}
            {rule.status === "PENDING_APPROVAL" && <button type="button" className={outlineButton} disabled={Boolean(busy)}
              onClick={() => void change(rule, "activate")}>Activate</button>}
          </div>
        </div>
        {busy === rule.id && <p role="status" className="text-xs text-muted">Working…</p>}
        {simulation?.id === rule.id && <div role="status" className="rounded-md bg-paper p-3">
          <p className="text-sm font-semibold">Test result</p>
          <PolicySample preview={simulation.preview} />
        </div>}
        {history?.rule.id === rule.id && <ul className="space-y-2 rounded-md border border-line bg-paper p-3">
          {history.versions.map(version => <li key={version.version} className="min-w-0 rounded-md border border-line bg-white p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <details className="min-w-0">
                <summary className="cursor-pointer text-sm font-semibold wrap-anywhere">
                  Version {version.version} · {version.proposal.name}
                </summary>
                <div className="mt-2"><RulePreview proposal={version.proposal} /></div>
              </details>
              {version.version !== rule.version && <button type="button" className={outlineButton}
                disabled={Boolean(busy)} onClick={() => void change(rule, "rollback", version.version)}>Restore version {version.version}</button>}
            </div>
          </li>)}
        </ul>}
      </li>)}
    </ul>}
  </section>;
}

function PolicySample({ preview }: { preview: PolicyPreview }) {
  return <div className="mt-1 text-xs text-muted">
    <p>
      {preview.matched} of {preview.evaluated} matched · {preview.missing} with missing signals
      {!!preview.conflictRuleIds?.length && ` · Conflicts with ${preview.conflictRuleIds.length} active policies`}
    </p>
    {preview.approvalReason && <p className="mt-1">{preview.approvalReason}</p>}
  </div>;
}

function MerchantPolicyWorkspace({ token }: { token: string }) {
  const [refreshKey, setRefreshKey] = useState(0);
  const [edit, setEdit] = useState<PolicyRule>();
  return <>
    <div id="merchant-policy-editor" tabIndex={-1}>
      <RuleComposer scope="MERCHANT" edit={edit} onCancel={() => setEdit(undefined)} onSaved={() => setRefreshKey(value => value + 1)} />
    </div>
    <ConnectedPolicyRules token={token} refreshKey={refreshKey} onEdit={setEdit} />
  </>;
}

export function MerchantRulesWorkspace() {
  const token = useRuleCredential("MERCHANT");
  return <main className="mx-auto max-w-[1050px] space-y-5 px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header>
      <h1 className="text-[30px] font-semibold tracking-tight sm:text-[36px]">Merchant AI Rules</h1>
      <p className="mt-2 text-sm text-muted">Create policies for your merchant’s products and transaction signals.</p>
    </header>
    <RuleAccess scope="MERCHANT" />
    {token && <MerchantPolicyWorkspace key={token} token={token} />}
    <div className="flex flex-wrap gap-4 text-sm font-semibold text-accent">
      <Link href="/settings/merchants?view=transactions" className="hover:underline">Open transactions</Link>
      <Link href="/settings/merchant-rules?tab=review" className="hover:underline">Legacy booking review rules</Link>
    </div>
  </main>;
}

export function SettingsWorkspace({ children }: { children: ReactNode }) {
  const merchant = useWorkspaceView() === "merchant";
  const token = useRuleCredential("MERCHANT");
  if (!merchant) return children;
  return <div className="mt-6 space-y-4">
    <RuleAccess scope="MERCHANT" />
    {token && <MerchantRiskProtection key={token} token={token} />}
  </div>;
}

function MerchantRiskProtection({ token }: { token: string }) {
  const [profile, setProfile] = useState<BusinessProfile | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void authenticatedRequest<BusinessProfile>(`${policies}/profile`, token, { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) setProfile(result); })
      .catch(cause => { if (!controller.signal.aborted) setError(errorText(cause)); });
    return () => controller.abort();
  }, [token]);
  return <section className={panel} aria-labelledby="merchant-business-profile">
    <h2 id="merchant-business-profile" className="text-lg font-semibold">Business Profile</h2>
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
    {!error && !profile && <p role="status" className="text-sm text-muted">Loading your profile…</p>}
    {profile && <>
      <p className="font-semibold">{profile.businessName}</p>
      <div>
        <h3 className="text-sm font-semibold">Risk Protection</h3>
        <p className="mt-1 text-sm text-muted">{profile.scoringProfileVersion ? "Scoring profile active" : "No active scoring profile is recorded."}</p>
        <ProfileDetails profile={profile} />
      </div>
    </>}
  </section>;
}
