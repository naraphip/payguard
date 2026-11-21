"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState, useSyncExternalStore } from "react";
import type { FormEvent } from "react";
import { getMerchantDemoRole, getMerchantDemoToken, merchantDemoRequest, merchantPolicyProducts, policyActionLabels, setMerchantDemoToken, subscribeMerchantDemoSession } from "../merchant-demo";
import type { DemoDirectory } from "../merchant-demo";
import { containsMailbox, merchantMailboxDigest, parseMailboxBlock } from "../merchant-mailbox";
import { adminRulePath, useLocalAdminPreview } from "../local-admin-preview";
import { useWorkspaceView } from "../workspace-view";
import type { ReactNode } from "react";
import { merchantDate } from "./merchants/management-contracts";
import { LegacyJevRuleItem, type LegacyJevRules } from "./jev-rules/panel";
import type { Proposal as LegacyJevProposal } from "./jev-rules/proposal";
import { merchantFieldLabels, merchantOperatorLabels, policyFieldLabel, StructuredRuleBuilder } from "./structured-rule-builder";
import { outlineButton, primaryButton, ruleRequest, rulesEndpoint, stages } from "./system-rules/contracts";
import type { Preview, PreviewRun, Rule, RuleCatalogue, RuleInput } from "./system-rules/contracts";

const composer = "/api/v1/booking/rule-composer";
const policies = "/api/v1/booking/demo/merchant-policy";
const panel = "min-w-0 space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5";

type Scope = "MERCHANT" | "PLATFORM";
export type PolicyAction = "BLOCK" | "REVIEW" | "CHALLENGE";
type PolicyCondition = { field: string; operator: string; value: string | number | boolean };
export type MerchantProposal = {
  scope: "MERCHANT";
  name: string;
  product_types: string[];
  stages: string[];
  conditions: { logic: "AND" | "OR"; rules: PolicyCondition[] };
  action: PolicyAction;
  durationHours?: number;
};
type PlatformProposal = { scope: "PLATFORM"; rule: RuleInput };
type Proposal = MerchantProposal | PlatformProposal;
type Interpretation = {
  status: "READY" | "CLARIFICATION";
  proposal?: Proposal;
  message?: string;
  validation: { valid: boolean; errors?: string[]; warnings?: string[] };
  semanticDraft?: { field: string };
};

type PolicyPreview = { evaluated: number; matched: number; missing: number; requiresJev?: number; conflictRuleIds?: string[]; approvalReason?: string };
type PolicyRule = {
  id: string;
  version: number;
  activeVersion?: number;
  status: "ACTIVE" | "PENDING_APPROVAL" | "DISABLED" | "EXPIRED";
  proposal: MerchantProposal;
  preview: PolicyPreview;
  expiresAt?: string;
};
type PolicySave = { rule: PolicyRule; status: PolicyRule["status"] };
type PlatformSave = { rule: Rule; status: "DRAFT"; preview?: Preview };
export type PolicyField = {
  fieldName: string;
  label?: string;
  dataType: string;
  allowedOperators: string[];
  supportedProductTypes: string[];
  availableStages: string[];
};
export type BusinessProfile = {
  merchantId: string;
  businessName: string;
  products: string[];
  stages: string[];
  signals: string[];
  scoringProfileVersion: string;
  fieldRegistry?: PolicyField[];
};

const scopeLabels = { MERCHANT: "Merchant AI Rule", PLATFORM: "Platform fraud rule" };
const products: Record<string, string> = merchantPolicyProducts;
const mailboxField = "customer.mailbox_ref";
const mailboxExample = "Block email buyer@example.test for 12 hours";
const fields: Record<string, string> = {
  ...merchantFieldLabels,
  account_age_days: "Account age in days",
  device_orders: "Orders from this device in 60 seconds",
  ip_orders: "Orders from this IP in 60 seconds",
  payment_token_orders: "Orders using this payment token in 60 seconds",
  payment_token_missing: "Payment token is missing",
  shared_device_or_token_orders: "Maximum orders using the device or token in 60 seconds",
};
const statusLabels = {
  ACTIVE: "Active", PENDING_APPROVAL: "Pending approval", DISABLED: "Disabled", EXPIRED: "Expired", DRAFT: "Draft",
};
const prompts: Record<Scope, string> = {
  MERCHANT: "Review all products when amount is at least 50000 minor units",
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
    <div><dt className="font-medium text-ink">Available signals</dt><dd>{profile.signals?.map(field => {
      const capability = profile.fieldRegistry?.find(entry => entry.fieldName === field);
      return capability ? policyFieldLabel(capability) : fields[field] ?? field;
    }).join(", ") || "Unavailable"}</dd></div>
    <div><dt className="font-medium text-ink">Scoring profile</dt><dd>{profile.scoringProfileVersion || "Unavailable"}</dd></div>
  </dl>;
}

async function authenticatedRequest<T>(path: string, token: string, init?: RequestInit) {
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (init?.body) headers.set("Content-Type", "application/json");
  return ruleRequest<T>(path, { ...init, headers, cache: "no-store" });
}

function merchantPolicyRequest<T>(resource: string, token: string, init?: RequestInit, adminMerchantId?: string) {
  if (adminMerchantId) {
    const path = resource === "interpret" ? "policy-interpret" : resource === "profile" ? "policy-profile" : resource.replace(/^rules/, "policies");
    return merchantDemoRequest<T>(`/${encodeURIComponent(adminMerchantId)}/${path}`, token, init);
  }
  const path = resource === "interpret" ? `${composer}/interpret` : `${policies}/${resource}`;
  return authenticatedRequest<T>(path, token, init);
}

export function RuleAccess({ scope }: { scope: Scope }) {
  const token = useRuleCredential(scope);
  const localAdmin = useLocalAdminPreview();
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

  if (scope === "PLATFORM" && localAdmin && !token) return null;

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

export function RulePreview({ proposal, mailbox, profile }: { proposal: Proposal; mailbox?: string; profile?: BusinessProfile | null }) {
  const merchant = proposal.scope === "MERCHANT";
  const name = merchant ? proposal.name : proposal.rule.name;
  const action = merchant ? proposal.action : proposal.rule.action;
  const ruleStages = merchant ? proposal.stages : [proposal.rule.stage];
  const conditions = merchant ? proposal.conditions.rules : proposal.rule.conditions.map(({ signal, operator, value }) => ({ field: signal, operator, value }));
  const logic = merchant && proposal.conditions.logic === "OR" ? "Any condition" : "All conditions";
  const hasMailbox = conditions.some(condition => condition.field === mailboxField);
  const duration = merchant ? proposal.durationHours : undefined;
  return <div className="space-y-4 rounded-lg border border-line bg-paper p-4">
    <dl className="grid gap-x-5 gap-y-3 text-sm sm:grid-cols-2">
      <div><dt className="text-muted">Rule name</dt><dd className="font-semibold wrap-anywhere">{name}</dd></div>
      <div><dt className="text-muted">Policy source</dt><dd className="font-semibold">{scopeLabels[proposal.scope]}</dd></div>
      {merchant && <div><dt className="text-muted">Products</dt><dd>{proposal.product_types.map(product => products[product] ?? product).join(", ")}</dd></div>}
      <div><dt className="text-muted">When</dt><dd>{ruleStages.map(stage => stages[stage as keyof typeof stages] ?? stage).join(", ")}</dd></div>
      <div><dt className="text-muted">Action</dt><dd className="font-semibold">{policyActionLabels[action] ?? action}</dd></div>
      {duration && <div><dt className="text-muted">Duration</dt><dd>{duration} {duration === 1 ? "hour" : "hours"} from activation</dd></div>}
    </dl>
    <div>
      <p className="text-sm font-semibold">{logic}</p>
      <ul className="mt-1 list-disc space-y-1 pl-5 text-sm">
        {conditions.map(({ field, operator, value }, index) => {
          const capability = profile?.fieldRegistry?.find(entry => entry.fieldName === field);
          const label = capability ? policyFieldLabel(capability) : fields[field] ?? field;
          const display = field === mailboxField ? mailbox ?? "the specified address" : field === "product.type" ? products[String(value)] ?? value : String(value);
          return <li key={index} className="wrap-anywhere">
            {label} {merchantOperatorLabels[operator] ?? operator} {display}
            {merchant && <span className="ml-2 rounded-md bg-white px-1.5 py-0.5 text-xs font-medium text-muted">
              {operator === "semantic_match" ? "Checked by Jev" : "Checked by PayGuard"}
            </span>}
          </li>;
        })}
      </ul>
      {merchant && conditions.some(condition => condition.operator === "semantic_match") && <p className="mt-2 text-xs text-muted">
        Jev evaluates the meaning from transaction evidence. If a required condition remains unresolved, the transaction goes to manual review.
      </p>}
    </div>
    {!hasMailbox && <details>
      <summary className="cursor-pointer text-xs font-semibold text-accent">Advanced rule details</summary>
      <pre className="mt-2 max-h-64 overflow-auto rounded-md border border-line bg-white p-3 text-xs">{JSON.stringify(proposal, null, 2)}</pre>
    </details>}
  </div>;
}

export function PolicyExpiry({ expiresAt, status }: Pick<PolicyRule, "expiresAt" | "status">) {
  if (!expiresAt) return null;
  return <p className="mt-1 text-xs text-muted">
    {status === "EXPIRED" ? "Expired" : "Expires"} <time dateTime={expiresAt}>{merchantDate(expiresAt)}</time>
  </p>;
}

export function RuleComposer({ scope, onSaved }: {
  scope: Scope;
  onSaved?: () => void;
}) {
  const token = useRuleCredential(scope);
  const localAdmin = useLocalAdminPreview();
  return (token || (scope === "PLATFORM" && localAdmin)) && <ConnectedComposer key={token}
    scope={scope} token={token} localAdmin={localAdmin} onSaved={onSaved} />;
}

function useMerchantRuleProfile(token: string, adminMerchantId?: string) {
  const [profile, setProfile] = useState<BusinessProfile | null>(null);
  const [profileError, setProfileError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setProfileError("");
    void merchantPolicyRequest<BusinessProfile>("profile", token, { signal: controller.signal }, adminMerchantId)
      .then(result => { if (!controller.signal.aborted) setProfile(result); })
      .catch(cause => { if (!controller.signal.aborted) setProfileError(errorText(cause)); });
    return () => controller.abort();
  }, [token, revision, adminMerchantId]);
  return { profile, profileError, retryProfile: () => setRevision(value => value + 1) };
}

function ConnectedComposer({ scope, token, localAdmin = false, adminMerchantId, onSaved, edit, onCancel, merchantProfile }: {
  scope: Scope; token: string; localAdmin?: boolean; adminMerchantId?: string; onSaved?: () => void; edit?: PolicyRule; onCancel?: () => void;
  merchantProfile?: ReturnType<typeof useMerchantRuleProfile>;
}) {
  const exactEmailEdit = edit?.proposal.conditions.rules.some(condition => condition.field === mailboxField);
  const canUseBuilder = scope === "MERCHANT" && !exactEmailEdit;
  const [instruction, setInstruction] = useState("");
  const [draft, setDraft] = useState<Interpretation | null>(null);
  const [pending, setPending] = useState<"interpret" | "save" | null>(null);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState<PolicySave | PlatformSave | null>(null);
  const [runId, setRunId] = useState("");
  const [runs, setRuns] = useState<PreviewRun[]>([]);
  const [runError, setRunError] = useState("");
  const [showConditions, setShowConditions] = useState(Boolean(edit) && !exactEmailEdit);
  const [builderProposal, setBuilderProposal] = useState<MerchantProposal | undefined>(edit?.proposal);
  const [builderRevision, setBuilderRevision] = useState(0);
  const [semanticField, setSemanticField] = useState<string>();
  const { profile, profileError, retryProfile } = merchantProfile ?? {};
  const request = useRef<AbortController>(null);
  const id = useId();
  const mailboxBlock = scope === "MERCHANT" ? parseMailboxBlock(instruction) : null;

  useEffect(() => {
    const controller = new AbortController();
    const init = { signal: controller.signal };
    if (scope === "PLATFORM") {
      void authenticatedRequest<RuleCatalogue>(adminRulePath(rulesEndpoint, localAdmin, token), token, init).then(result => {
        if (!controller.signal.aborted) setRuns(result.runs ?? []);
      }).catch(cause => {
        if (!controller.signal.aborted) setRunError(errorText(cause));
      });
    }
    return () => {
      controller.abort();
      request.current?.abort();
    };
  }, [scope, token, localAdmin]);

  async function interpret(event: FormEvent) {
    event.preventDefault();
    if (request.current || !instruction.trim() || (scope === "MERCHANT" && !profile)) return;
    const controller = new AbortController();
    request.current = controller;
    setPending("interpret");
    setDraft(null);
    setSaved(null);
    setError("");
    try {
      let payload: { scope: Scope; instruction: string } | { scope: Scope; mailboxBlock: { mailboxDigest: string; durationHours: number } };
      if (mailboxBlock) {
        if (!profile?.merchantId) throw new Error("The merchant profile is unavailable. Reconnect and try again.");
        const mailboxDigest = await merchantMailboxDigest(profile.merchantId, mailboxBlock.mailbox);
        payload = { scope, mailboxBlock: { mailboxDigest, durationHours: mailboxBlock.durationHours } };
      } else {
        if (scope === "MERCHANT" && containsMailbox(instruction)) {
          throw new Error(`To block one email address temporarily, enter a duration from 1 to 8,760 whole hours. For example, ${mailboxExample}.`);
        }
        payload = { scope, instruction: instruction.trim() };
      }
      if (controller.signal.aborted) return;
      const init = { method: "POST", body: JSON.stringify(payload), signal: controller.signal };
      const result = scope === "MERCHANT"
        ? await merchantPolicyRequest<Interpretation>("interpret", token, init, adminMerchantId)
        : await authenticatedRequest<Interpretation>(adminRulePath(`${composer}/interpret`, localAdmin, token), token, init);
      if (controller.signal.aborted) return;
      if (result.proposal && result.proposal.scope !== scope) throw new Error("The proposal belongs to another rule system.");
      setDraft(result);
      if (result.proposal?.scope === "MERCHANT" && !result.proposal.conditions.rules.some(condition => condition.field === mailboxField)) {
        setBuilderProposal(result.proposal);
        setBuilderRevision(value => value + 1);
        setSemanticField(undefined);
      }
    } catch (cause) {
      if (!controller.signal.aborted) {
        if (canUseBuilder && !mailboxBlock) setShowConditions(true);
        setError(errorText(cause));
      }
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
      const payload = edit ? { proposal, expectedVersion: edit.version }
        : proposal.scope === "PLATFORM" && runId ? { proposal, runId } : { proposal };
      const init = { method: edit ? "PUT" : "POST", body: JSON.stringify(payload), signal: controller.signal };
      const result = proposal.scope === "MERCHANT"
        ? await merchantPolicyRequest<PolicySave>(`rules${edit ? `/${encodeURIComponent(edit.id)}` : ""}`, token, init, adminMerchantId)
        : await authenticatedRequest<PlatformSave>(adminRulePath(`${composer}/platform/save`, localAdmin, token), token, init);
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
  const suggestedField = draft?.semanticDraft?.field;

  function resetDraft() {
    setDraft(null);
    setSaved(null);
    setError("");
  }

  function startJevCondition(field: string) {
    setBuilderProposal(undefined);
    setBuilderRevision(value => value + 1);
    setSemanticField(field);
    setShowConditions(true);
    setDraft(null);
  }

  return <section className={panel} aria-labelledby={`${id}-title`}>
    <div>
      <h2 id={`${id}-title`} className="text-lg font-semibold">{scope === "MERCHANT" ? "AI Rule" : "AI rule composer"}</h2>
      <p className="mt-1 text-sm text-muted">Describe the rule in Thai or English, then review the proposal before saving.</p>
    </div>
    {scope === "MERCHANT" && !profile && !profileError && <p role="status" className="text-sm text-muted">Loading available rule fields…</p>}
    {profileError && <div className="space-y-2">
      <p role="alert" className="text-sm text-red-800">Could not load the merchant’s rule fields. {profileError}</p>
      <button type="button" className={outlineButton} onClick={retryProfile}>Retry loading rule fields</button>
    </div>}
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
      <textarea id={`${id}-instruction`} rows={3} maxLength={2000} className="control min-h-24 resize-y"
        aria-describedby={`${id}-instruction-help`} placeholder={prompts[scope]} value={instruction} disabled={pending !== null} onChange={event => {
          setInstruction(event.currentTarget.value);
          resetDraft();
        }} />
      <p id={`${id}-instruction-help`} className="text-xs text-muted">
        {scope === "MERCHANT" ? <>For a temporary exact-address block, use <code>{mailboxExample}</code>. Use bare domains for domain rules. Leave out IP addresses, card details, and credentials.</>
          : "Use policy conditions and domains. Leave out customer identities and card details."}
      </p>
      <button type="submit" className={ready ? outlineButton : primaryButton}
        disabled={!instruction.trim() || pending !== null || (scope === "MERCHANT" && !profile)}>
        {pending === "interpret" ? "Generating preview…" : "Generate preview"}
      </button>
    </form>
    {error && <p role="alert" className="rounded-lg bg-red-50 p-3 text-sm text-red-800">{error}</p>}
    {suggestedField && canUseBuilder && profile && <div className="space-y-2 rounded-lg bg-paper p-3 text-sm">
      {draft?.message && <p role="status">{draft.message}</p>}
      <button className={outlineButton} type="button" disabled={pending !== null}
        onClick={() => startJevCondition(suggestedField)}>Set up Jev condition</button>
    </div>}
    {canUseBuilder && profile && <details className="rounded-lg border border-line p-3" open={showConditions}
      onToggle={event => setShowConditions(event.currentTarget.open)}>
      <summary className="cursor-pointer text-sm font-semibold">Rule fields</summary>
      <div className="mt-3 space-y-3">
        {semanticField && <p className="text-xs text-muted">
          Enter the meaning Jev should look for. Add any other conditions from your instruction before reviewing.
        </p>}
        <StructuredRuleBuilder key={builderRevision} profile={profile} initial={builderProposal} semanticField={semanticField}
          disabled={pending !== null} onChange={resetDraft}
          onPreview={proposal => {
            setSaved(null);
            setError("");
            setDraft({ status: "READY", proposal, validation: { valid: true } });
          }} />
      </div>
    </details>}
    {draft && (!suggestedField || !canUseBuilder) && <div className="space-y-3">
      {draft.message && <p role="status" className="text-sm text-muted">{draft.message}</p>}
      {messages.length > 0 && <ul className="list-disc space-y-1 pl-5 text-sm text-amber-900">
        {messages.map((message, index) => <li key={index}>{message}</li>)}
      </ul>}
      {draft.proposal && <RulePreview proposal={draft.proposal} mailbox={mailboxBlock?.mailbox} profile={profile} />}
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
      {"expiresAt" in saved.rule && <PolicyExpiry expiresAt={saved.rule.expiresAt} status={saved.rule.status} />}
      {preview && ("evaluated" in preview ? <PolicySample preview={preview} />
        : <p className="mt-1">{preview.matched} matched out of {preview.eligible} eligible events · {preview.skipped} with missing signals</p>)}
    </div>}
  </section>;
}

type RuleHistory = { rule: PolicyRule; versions: PolicyRule[] };

function ConnectedPolicyRules({ token, adminMerchantId, profile, refreshKey, onEdit, onDelete }: {
  token: string; adminMerchantId?: string; profile: BusinessProfile | null; refreshKey: number;
  onEdit: (rule: PolicyRule) => void; onDelete: (ruleId: string) => void;
}) {
  const [rules, setRules] = useState<PolicyRule[]>([]);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [history, setHistory] = useState<RuleHistory | null>(null);
  const [simulation, setSimulation] = useState<{ id: string; preview: PolicyPreview } | null>(null);
  const [confirmDelete, setConfirmDelete] = useState("");
  const [legacy, setLegacy] = useState<LegacyJevRules>();
  const [legacyError, setLegacyError] = useState("");
  const [legacyLoading, setLegacyLoading] = useState(false);
  const lifecycle = useRef<AbortController>(null);
  const scopedMerchantId = adminMerchantId ?? profile?.merchantId;
  const legacyPath = scopedMerchantId ? `/${encodeURIComponent(scopedMerchantId)}/jev/rules` : "";

  useEffect(() => () => lifecycle.current?.abort(), []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void merchantPolicyRequest<{ rules: PolicyRule[] }>("rules", token, { signal: controller.signal }, adminMerchantId)
      .then(result => { if (!controller.signal.aborted) setRules(result.rules ?? []); })
      .catch(cause => { if (!controller.signal.aborted) setError(errorText(cause)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [token, adminMerchantId, refreshKey, revision]);

  useEffect(() => {
    if (!legacyPath) return;
    const controller = new AbortController();
    setLegacyLoading(true);
    setLegacyError("");
    void merchantDemoRequest<LegacyJevRules>(legacyPath, token, { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) setLegacy(result); })
      .catch(cause => { if (!controller.signal.aborted) setLegacyError(errorText(cause)); })
      .finally(() => { if (!controller.signal.aborted) setLegacyLoading(false); });
    return () => controller.abort();
  }, [legacyPath, token, revision]);

  async function changeLegacy(id: string, proposal?: LegacyJevProposal) {
    if (!legacy || !legacyPath || lifecycle.current) return false;
    const controller = new AbortController();
    lifecycle.current = controller;
    setBusy(id);
    setLegacyError("");
    setNotice("");
    try {
      const result = await merchantDemoRequest<LegacyJevRules>(`${legacyPath}/${encodeURIComponent(id)}`, token, {
        method: proposal ? "PUT" : "DELETE",
        body: JSON.stringify({ expectedVersion: legacy.version, ...(proposal ? { definition: proposal } : {}) }),
        signal: controller.signal,
      });
      if (controller.signal.aborted) return false;
      setLegacy(result);
      setNotice(proposal ? "Rule updated." : "Rule deleted.");
      return true;
    } catch (cause) {
      if (!controller.signal.aborted) setLegacyError(errorText(cause));
      return false;
    } finally {
      if (!controller.signal.aborted) setBusy("");
      lifecycle.current = null;
    }
  }

  async function change(rule: PolicyRule, operation: "disable" | "activate" | "rollback" | "simulate" | "history" | "delete", version?: number) {
    if (lifecycle.current) return;
    const controller = new AbortController();
    lifecycle.current = controller;
    setBusy(rule.id);
    setError("");
    setNotice("");
    const path = `rules/${encodeURIComponent(rule.id)}`;
    const { signal } = controller;
    try {
      if (operation === "history") {
        const result = await merchantPolicyRequest<RuleHistory>(path, token, { signal }, adminMerchantId);
        if (!signal.aborted) setHistory(result);
      } else if (operation === "simulate") {
        const result = await merchantPolicyRequest<{ preview: PolicyPreview }>(`${path}/simulate`, token, { method: "POST", signal }, adminMerchantId);
        if (!signal.aborted) setSimulation({ id: rule.id, preview: result.preview });
      } else if (operation === "delete") {
        await merchantPolicyRequest<unknown>(path, token, {
          method: "DELETE", body: JSON.stringify({ expectedVersion: rule.version }), signal,
        }, adminMerchantId);
        if (signal.aborted) return;
        setConfirmDelete("");
        setHistory(null);
        setSimulation(null);
        setNotice(`Deleted ${rule.proposal.name}.`);
        setRevision(value => value + 1);
        onDelete(rule.id);
      } else {
        const result = await merchantPolicyRequest<PolicySave>(`${path}/${operation}`, token, {
          method: "POST", body: JSON.stringify({ version, expectedVersion: rule.version }), signal,
        }, adminMerchantId);
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
    <div className="flex items-center justify-between gap-2">
      <h2 id="merchant-policy-list" className="text-lg font-semibold">Saved rules</h2>
      <button type="button" className={outlineButton} disabled={loading || legacyLoading || !!busy}
        onClick={() => setRevision(value => value + 1)}>Refresh rules</button>
    </div>
    {(loading || legacyLoading) && <p role="status" className="text-sm text-muted">Loading rules…</p>}
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
    {legacyError && <p role="alert" className="text-sm text-red-800">Could not load or save booking review rules. {legacyError}</p>}
    {notice && <p role="status" className="text-sm text-emerald-900">{notice}</p>}
    {!loading && !legacyLoading && !error && !legacyError && !rules.length && !legacy?.rules.length && <p className="text-sm text-muted">No saved rules.</p>}
    {(rules.length > 0 || !!legacy?.rules.length) && <ul className="divide-y divide-line">
      {rules.map(rule => <li className="space-y-3 py-3" key={rule.id}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold wrap-anywhere">{rule.proposal.name}</p>
            <p className="mt-1 text-xs text-muted">{statusLabels[rule.status]} · Version {rule.version} · {policyActionLabels[rule.proposal.action]}
              {rule.proposal.conditions.rules.some(condition => condition.operator === "semantic_match") && " · Jev condition"}
              {rule.activeVersion && rule.activeVersion !== rule.version ? ` · Version ${rule.activeVersion} remains active` : ""}
            </p>
            <PolicyExpiry expiresAt={rule.expiresAt} status={rule.status} />
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
            <button type="button" className={outlineButton} disabled={Boolean(busy) || confirmDelete === rule.id}
              onClick={() => setConfirmDelete(rule.id)}>Delete</button>
          </div>
        </div>
        {confirmDelete === rule.id && <div className="space-y-2 rounded-md border border-red-200 bg-red-50 p-3" aria-label="Confirm rule deletion">
          <p className="text-sm text-red-900">Delete <strong>{rule.proposal.name}</strong> from future decisions? Saved transactions retain their rule versions.</p>
          <div className="flex flex-wrap gap-2">
            <button type="button" className="rounded-md bg-red-700 px-3 py-1.5 text-sm font-semibold text-white disabled:opacity-55"
              disabled={Boolean(busy)} onClick={() => void change(rule, "delete")}>Confirm deletion</button>
            <button type="button" className={outlineButton} disabled={Boolean(busy)} onClick={() => setConfirmDelete("")}>Cancel</button>
          </div>
        </div>}
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
                  Version {version.version} · {statusLabels[version.status]} · {version.proposal.name}
                </summary>
                <PolicyExpiry expiresAt={version.expiresAt} status={version.status} />
                <div className="mt-2"><RulePreview proposal={version.proposal} profile={profile} /></div>
              </details>
              {version.version !== rule.version && <button type="button" className={outlineButton}
                disabled={Boolean(busy)} onClick={() => void change(rule, "rollback", version.version)}>Restore version {version.version}</button>}
            </div>
          </li>)}
        </ul>}
      </li>)}
      {legacy?.rules.map(rule => <LegacyJevRuleItem key={rule.id} rule={rule} version={legacy.version} busy={!!busy || legacyLoading}
        onChange={proposal => changeLegacy(rule.id, proposal)} />)}
    </ul>}
  </section>;
}

function PolicySample({ preview }: { preview: PolicyPreview }) {
  return <div className="mt-1 text-xs text-muted">
    <p>
      {preview.matched} of {preview.evaluated} matched · {preview.missing} with missing signals
      {!!preview.requiresJev && ` · ${preview.requiresJev} require Jev at transaction time`}
      {!!preview.conflictRuleIds?.length && ` · Conflicts with ${preview.conflictRuleIds.length} active policies`}
    </p>
    {preview.approvalReason && <p className="mt-1">{preview.approvalReason}</p>}
  </div>;
}

function MerchantPolicyWorkspace({ token, adminMerchantId }: { token: string; adminMerchantId?: string }) {
  const [refreshKey, setRefreshKey] = useState(0);
  const [edit, setEdit] = useState<PolicyRule>();
  const merchantProfile = useMerchantRuleProfile(token, adminMerchantId);
  return <>
    <div id="merchant-policy-editor" tabIndex={-1}>
      <ConnectedComposer key={`${edit?.id ?? ""}:${edit?.version ?? ""}`} scope="MERCHANT" token={token} adminMerchantId={adminMerchantId}
        merchantProfile={merchantProfile} edit={edit} onCancel={() => setEdit(undefined)} onSaved={() => setRefreshKey(value => value + 1)} />
    </div>
    <ConnectedPolicyRules token={token} adminMerchantId={adminMerchantId} profile={merchantProfile.profile} refreshKey={refreshKey} onEdit={setEdit}
      onDelete={id => setEdit(current => current?.id === id ? undefined : current)} />
  </>;
}

export function AdminMerchantPolicyWorkspace({ merchantId, token }: { merchantId: string; token: string }) {
  return <MerchantPolicyWorkspace key={`${merchantId}:${token}`} token={token} adminMerchantId={merchantId} />;
}

export function MerchantRulesWorkspace() {
  const token = useRuleCredential("MERCHANT");
  return <main className="mx-auto max-w-[1050px] space-y-5 px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header>
      <h1 className="text-[30px] font-semibold tracking-tight sm:text-[36px]">Merchant AI Rules</h1>
    </header>
    <RuleAccess scope="MERCHANT" />
    {token && <MerchantPolicyWorkspace key={token} token={token} />}
    <div className="flex flex-wrap gap-4 text-sm font-semibold text-accent">
      <Link href="/settings/merchants?view=transactions" className="hover:underline">Open transactions</Link>
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
