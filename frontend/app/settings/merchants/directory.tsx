"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowRight, Check, LoaderCircle, Plus, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { merchantDemoRequest, merchantPolicyProducts } from "../../merchant-demo";
import type { DemoDirectory, DemoMerchant } from "../../merchant-demo";
import { useWorkspaceView } from "../../workspace-view";
import { useProductionSession } from "../../production-auth";
import { useLocalAdminPreview } from "../../local-admin-preview";
import { actionButton, stateBadge } from "../../action-styles";
import { MerchantTransactions } from "./transactions";
import { isBusinessModel } from "../merchant-setup/contracts";
import type { BusinessModel, MerchantSetup } from "../merchant-setup/contracts";
import { MerchantTokenControls } from "./access-token";
import { CustomSignalSetup } from "./custom-signal-setup";
import { MerchantManagementAccess, managementError } from "./management-access";
import { merchantDate, merchantDetailsHref } from "./management-contracts";
import type { MerchantDirectoryPage, MerchantTab } from "./management-contracts";

const action = actionButton.primary;
const secondary = actionButton.secondary;
const input = "control mt-1 min-w-0";
const card = "min-w-0 rounded-xl border border-line bg-white p-4 sm:p-5";
const count = new Intl.NumberFormat("en-US");
const modelLabels = { ota: "Online travel agency (OTA)", ecommerce: "E-commerce" };
const policyProducts = Object.entries(merchantPolicyProducts);
const limit = 20;
type Sort = "name" | "created" | "activity";

function errorMessage(error: unknown) {
  return managementError(error);
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block min-w-0 text-sm font-medium">{label}{children}</label>;
}

export function MerchantDirectory({ preferredTab = "overview", transaction }: { preferredTab?: MerchantTab; transaction?: string }) {
  const merchantView = useWorkspaceView() === "merchant";
  const ownDirectory = useProductionSession()?.role === "merchant" && preferredTab !== "transactions";
  const showingTransactions = merchantView && !ownDirectory;
  return <>
    <header className="mb-4">
      <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">{showingTransactions ? "Your transactions" : "Merchants"}</h1>
      <p className="mt-2 text-sm text-muted">{showingTransactions
        ? "Review bookings and decisions for your merchant."
        : ownDirectory ? "Open your merchant workspace." : "Manage merchant accounts and open their workspaces."}</p>
    </header>
    <MerchantManagementAccess merchant={merchantView}>
      {token => merchantView ? <OwnMerchant key={token} token={token} transaction={transaction} directory={ownDirectory} />
        : <DirectoryList key={token} token={token} preferredTab={preferredTab} />}
    </MerchantManagementAccess>
  </>;
}

function OwnMerchant({ token, transaction, directory }: { token: string; transaction?: string; directory: boolean }) {
  const [merchant, setMerchant] = useState<DemoMerchant | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void merchantDemoRequest<DemoDirectory>("", token, { signal: controller.signal }).then(result => {
      if (controller.signal.aborted) return;
      if (result.role !== "merchant" || result.merchants.length !== 1) throw new Error("Access denied.");
      setMerchant(result.merchants[0]);
    }).catch(error => { if (!controller.signal.aborted) setError(errorMessage(error)); });
    return () => controller.abort();
  }, [token]);
  if (error) return <p role="alert" className="text-sm text-red-800">{error}</p>;
  if (!merchant) return <p role="status" className="text-sm text-muted">Loading your merchant workspace…</p>;
  if (directory) return <section aria-label="Merchant directory" className={`${card} space-y-3`}>
    <h2 className="text-lg font-semibold">{merchant.name}</h2>
    <div className="flex flex-wrap gap-4 text-sm font-semibold text-accent">
      <Link className="hover:underline" href="/settings/merchants?view=transactions">Transactions</Link>
      <Link className="hover:underline" href="/settings/merchant-rules">Merchant AI Rules</Link>
    </div>
  </section>;
  return <MerchantTransactions token={token} merchant={merchant} canDecide={false} initialTransaction={transaction} />;
}

function DirectoryList({ token, preferredTab }: { token: string; preferredTab: MerchantTab }) {
  const session = useProductionSession();
  const localAdmin = useLocalAdminPreview();
  const [query, setQuery] = useState("");
  const [applied, setApplied] = useState("");
  const [sort, setSort] = useState<Sort>("name");
  const [page, setPage] = useState(1);
  const [result, setResult] = useState<MerchantDirectoryPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const [error, setError] = useState("");
  const [createMode, setCreateMode] = useState(false);
  const [receipt, setReceipt] = useState<{ merchant: DemoMerchant; token: string }>();
  const router = useRouter();

  useEffect(() => {
    const controller = new AbortController();
    const filters = new URLSearchParams({ page: String(page), limit: String(limit), sort });
    if (applied) filters.set("q", applied);
    setLoading(true);
    setError("");
    void merchantDemoRequest<MerchantDirectoryPage>(`?${filters}`, token, { signal: controller.signal }).then(value => {
      if (!controller.signal.aborted) setResult(value);
    }).catch(error => {
      if (!controller.signal.aborted) {
        setError(errorMessage(error));
        setResult(null);
      }
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [applied, page, revision, sort, token]);

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPage(1);
    setApplied(query.trim());
  }

  if (receipt && !session) return <section className={`${card} max-w-2xl space-y-3`} aria-labelledby="merchant-access-token">
    <h2 id="merchant-access-token" className="text-lg font-semibold">Merchant access token</h2>
    <p role="status" className="text-sm text-muted">Created {receipt.merchant.name}. Copy its one-time sandbox token before leaving.</p>
    <MerchantTokenControls token={receipt.token} />
    <button type="button" className={action} onClick={() => {
      const href = merchantDetailsHref(receipt.merchant.id);
      setReceipt(undefined);
      router.push(href);
    }}>Open merchant details <ArrowRight size={16} aria-hidden="true" /></button>
  </section>;

  if (createMode) return <MerchantNameForm token={token} onCancel={() => setCreateMode(false)}
    onSave={(merchant, issued) => {
      setCreateMode(false);
      if (issued && !session && !localAdmin) setReceipt({ merchant, token: issued });
      else router.push(merchantDetailsHref(merchant.id));
    }} />;

  const merchants = result?.merchants ?? [];
  const pageCount = result ? Math.max(1, Math.ceil(result.total / result.limit)) : 1;

  return <section aria-label="Merchant directory" className="space-y-4">
    <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end sm:justify-between">
      <form className="flex min-w-0 flex-wrap items-end gap-2" onSubmit={search}>
        <label className="min-w-0 flex-1 text-sm font-semibold">Search merchants
          <input className={input} type="search" value={query} maxLength={100} placeholder="Name or merchant ID"
            onChange={event => setQuery(event.currentTarget.value)} />
        </label>
        <button type="submit" className={secondary}>Search</button>
      </form>
      <div className="flex flex-wrap items-end gap-3">
        <label className="min-w-0 flex-1 text-sm font-semibold">Sort
          <select className={input} value={sort} onChange={event => {
            setSort(event.currentTarget.value as Sort);
            setPage(1);
          }}>
            <option value="name">Name</option><option value="created">Creation date</option><option value="activity">Last activity</option>
          </select>
        </label>
        <button type="button" className={`${action} w-full shrink-0 sm:w-auto`} onClick={() => setCreateMode(true)}>
          <Plus size={17} aria-hidden="true" />Create merchant
        </button>
      </div>
    </div>
    {error && <div className="flex flex-wrap items-center gap-3 rounded-lg bg-red-50 p-3 text-sm text-red-800">
      <p role="alert">{error}</p><button type="button" className={secondary} onClick={() => setRevision(value => value + 1)}>Retry</button>
    </div>}
    {loading && <p role="status" className="text-sm text-muted">Loading merchants…</p>}
    {!loading && !error && !merchants.length && <div className={card}>
      <p className="text-sm text-muted">{applied ? "No merchants match this search." : "No merchants yet."}</p>
    </div>}
    {!!merchants.length && <>
      <div className="hidden rounded-xl border border-line bg-white md:block">
        <table className="w-full table-fixed text-left text-sm">
          <caption className="sr-only">Merchants</caption>
          <thead className="border-b border-line bg-paper text-xs text-muted">
            <tr>
              <th scope="col" className="w-[32%] px-3 py-2.5">Merchant</th>
              <th scope="col" className="hidden w-[18%] px-3 py-2.5 xl:table-cell">Business</th>
              <th scope="col" className="px-2 py-2.5">Transactions</th>
              <th scope="col" className="px-2 py-2.5">Review</th>
              <th scope="col" className="px-2 py-2.5">Rules</th>
              <th scope="col" className="hidden px-2 py-2.5 2xl:table-cell">Last activity</th>
              <th scope="col" className="px-2 py-2.5"><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {merchants.map(merchant => <tr key={merchant.id}>
              <th scope="row" className="min-w-0 px-3 py-3 font-semibold wrap-anywhere">
                {merchant.name}<span className="mt-1 block font-mono text-[11px] font-normal text-muted">{merchant.id}</span>
              </th>
              <td className="hidden px-3 py-3 text-xs wrap-anywhere xl:table-cell">{businessSummary(merchant)}</td>
              <td className="px-2 py-3 tabular-nums">{merchant.transactionCount}</td>
              <td className="px-2 py-3 tabular-nums"><ReviewCount value={merchant.underReview} /></td>
              <td className="px-2 py-3 tabular-nums">{merchant.ruleCount}</td>
              <td className="hidden px-2 py-3 text-xs 2xl:table-cell">{merchantDate(merchant.lastActivity)}</td>
              <td className="px-2 py-3"><ViewMerchant merchant={merchant} tab={preferredTab} /></td>
            </tr>)}
          </tbody>
        </table>
      </div>
      <ul className="space-y-2 md:hidden" aria-label="Merchants">
        {merchants.map(merchant => <li className={`${card} space-y-3`} key={merchant.id}>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="font-semibold wrap-anywhere">{merchant.name}</p>
              <p className="font-mono text-[11px] text-muted wrap-anywhere">{merchant.id}</p>
            </div>
            <ViewMerchant merchant={merchant} tab={preferredTab} />
          </div>
          <p className="text-sm text-muted wrap-anywhere">{businessSummary(merchant)}</p>
          <dl className="grid grid-cols-3 gap-2 text-xs">
            <div><dt className="text-muted">Transactions</dt><dd className="font-semibold">{merchant.transactionCount}</dd></div>
            <div><dt className="text-muted">Review</dt><dd className="font-semibold"><ReviewCount value={merchant.underReview} /></dd></div>
            <div><dt className="text-muted">Rules</dt><dd className="font-semibold">{merchant.ruleCount}</dd></div>
          </dl>
          {merchant.lastActivity && <p className="text-xs text-muted">Last activity {merchantDate(merchant.lastActivity)}</p>}
        </li>)}
      </ul>
    </>}
    {result && <nav aria-label="Merchant directory pages" className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm text-muted">{count.format(result.total)} merchants · Page {result.page} of {pageCount}</p>
      <div className="flex gap-2">
        <button type="button" className={secondary} disabled={loading || page <= 1} onClick={() => setPage(value => value - 1)}>Previous</button>
        <button type="button" className={secondary} disabled={loading || page >= pageCount} onClick={() => setPage(value => value + 1)}>Next</button>
      </div>
    </nav>}
  </section>;
}

function ViewMerchant({ merchant, tab }: { merchant: DemoMerchant; tab: MerchantTab }) {
  return <Link aria-label={`View ${merchant.name}`} href={merchantDetailsHref(merchant.id, tab)}
    className={secondary}>View <ArrowRight size={14} aria-hidden="true" /></Link>;
}

function ReviewCount({ value }: { value: number }) {
  return value > 0 ? <span className={stateBadge("warning")}>{value}</span> : <span className="text-muted">0</span>;
}

function businessSummary(merchant: DemoMerchant) {
  if (merchant.description) return merchant.description;
  const products = merchant.policyIntegration?.products ?? [];
  if (products.length) return products.map(product => merchantPolicyProducts[product as keyof typeof merchantPolicyProducts] ?? product).join(", ");
  return merchant.setup ? modelLabels[merchant.setup.businessModel] : "Profile unavailable";
}

export function MerchantNameForm({ token, merchant, onSave, onDelete, onCancel }: {
  token: string;
  merchant?: DemoMerchant;
  onSave: (merchant: DemoMerchant, newToken?: string) => void;
  onDelete?: (merchantId: string) => void;
  onCancel?: () => void;
}) {
  const [name, setName] = useState(merchant?.name ?? "");
  const [description, setDescription] = useState(merchant?.description ?? "");
  const [businessModel, setBusinessModel] = useState<BusinessModel | "">(merchant?.setup?.businessModel ?? "");
  const existingAmount = Boolean(merchant?.policyIntegration?.amountMinor || merchant?.setup?.availableSignals.includes("amountMinor"));
  const [includesAmount, setIncludesAmount] = useState(existingAmount);
  const [products, setProducts] = useState(merchant?.policyIntegration?.products ?? []);
  const [emailDomain, setEmailDomain] = useState(merchant?.policyIntegration?.emailDomain ?? false);
  const savedCustomFields = merchant?.policyIntegration?.customFields ?? [];
  const [customFields, setCustomFields] = useState(savedCustomFields);
  const [saving, setSaving] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const creationKey = useRef("");
  const isUnchanged = name.trim() === merchant?.name && description.trim() === (merchant?.description ?? "") && (!businessModel || businessModel === merchant.setup?.businessModel) &&
    includesAmount === existingAmount &&
    emailDomain === (merchant.policyIntegration?.emailDomain ?? false) &&
    products.length === (merchant.policyIntegration?.products?.length ?? 0) &&
    products.every(product => merchant.policyIntegration?.products?.includes(product)) &&
    customFields.length === savedCustomFields.length &&
    customFields.every(field => savedCustomFields.some(saved => saved.key === field.key && saved.label === field.label && saved.type === field.type));

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!name.trim() || confirmDelete) return;
    setSaving(true);
    setError("");
    setSaved(false);
    const profileType = businessModel || merchant?.setup?.businessModel;
    const setup: MerchantSetup | undefined = profileType ? {
      schemaVersion: 1, businessModel: profileType, stage: "booking", availableSignals: includesAmount ? ["occurredAt", "amountMinor"] : ["occurredAt"],
    } : undefined;
    try {
      if (!merchant && !creationKey.current) creationKey.current = crypto.randomUUID();
      const result = await merchantDemoRequest<{ merchant: DemoMerchant; token?: string }>(merchant ? `/${merchant.id}` : "", token, {
        method: merchant ? "PUT" : "POST",
        headers: merchant ? undefined : { "Idempotency-Key": creationKey.current },
        body: JSON.stringify({ name: name.trim(), description: description.trim(), setup, policyIntegration: { products, emailDomain, amountMinor: includesAmount, customFields } }),
      });
      onSave(result.merchant, result.token);
      setName(result.merchant.name);
      setCustomFields(result.merchant.policyIntegration?.customFields ?? []);
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
    <form onSubmit={submit} className={`${card} space-y-4 ${merchant ? "" : "max-w-3xl"}`}>
      <h2 className="text-lg font-semibold">{merchant ? "Merchant details" : "New merchant"}</h2>
      <fieldset disabled={saving || deleting || confirmDelete} className="space-y-4">
        <legend className="sr-only">Merchant name and setup</legend>
        <Field label="Merchant name">
          <input className={input} value={name} onChange={event => { setName(event.target.value); setSaved(false); }} maxLength={80} required />
        </Field>
        <details open={merchant ? true : undefined} className="rounded-lg border border-line p-3">
          <summary className="cursor-pointer text-sm font-semibold">
            {merchant ? "Business and integration settings" : "Business and integration settings (optional)"}
          </summary>
          {!merchant && <p className="mt-2 text-sm text-muted">You can set up products and signals later in Merchant Details.</p>}
          <div className="mt-3 grid gap-4 sm:grid-cols-2">
            <label className="block min-w-0 text-sm font-medium sm:col-span-2">Business description (optional)
              <textarea className={input} rows={2} maxLength={500} value={description} onChange={event => {
                setDescription(event.currentTarget.value);
                setSaved(false);
              }} />
            </label>
            <Field label="Legacy booking profile type (optional)">
              <select className={input} value={businessModel} onChange={event => {
                const value = event.target.value;
                setBusinessModel(isBusinessModel(value) ? value : "");
                setSaved(false);
              }}>
                <option value="">{merchant?.setup ? "Keep existing profile type" : "Unspecified"}</option>
                <option value="ota">{modelLabels.ota}</option>
                <option value="ecommerce">{modelLabels.ecommerce}</option>
              </select>
            </Field>
            <fieldset className="space-y-2">
              <legend className="mb-1 text-sm font-medium">Available signals</legend>
              <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked disabled />Transaction timestamp (required)</label>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={includesAmount} onChange={event => { setIncludesAmount(event.target.checked); setSaved(false); }} />
                Booking amount
              </label>
            </fieldset>
            <fieldset className="space-y-2 sm:col-span-2">
              <legend className="mb-1 text-sm font-medium">Products supplied by the integration</legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {policyProducts.map(([product, label]) => <label key={product} className="flex items-center gap-2 text-sm">
                  <input type="checkbox" checked={products.includes(product)} onChange={event => {
                    const checked = event.currentTarget.checked;
                    setProducts(current => checked ? [...current, product] : current.filter(item => item !== product));
                    setSaved(false);
                  }} />
                  {label}
                </label>)}
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={emailDomain} onChange={event => {
                  setEmailDomain(event.currentTarget.checked);
                  setSaved(false);
                }} />
                Integration supplies customer email domains
              </label>
            </fieldset>
            <div className="sm:col-span-2">
              <CustomSignalSetup fields={customFields} onChange={fields => {
                setCustomFields(fields);
                setSaved(false);
              }} />
            </div>
          </div>
        </details>
      </fieldset>
      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" className={action} disabled={saving || deleting || confirmDelete || !name.trim() || isUnchanged}>
          {saving ? <LoaderCircle size={16} className="animate-spin motion-reduce:animate-none" aria-hidden="true" />
            : merchant ? <Check size={16} aria-hidden="true" /> : <Plus size={16} aria-hidden="true" />}
          {saving ? "Saving…" : merchant ? "Save details" : "Create merchant"}
        </button>
        {onCancel && <button type="button" className={secondary} onClick={onCancel} disabled={saving}>Cancel</button>}
        {merchant && onDelete && !confirmDelete && <button type="button" className={actionButton.dangerOutline} disabled={saving} onClick={() => {
          setConfirmDelete(true);
          setSaved(false);
          setError("");
        }}><Trash2 size={16} aria-hidden="true" />Delete merchant</button>}
      </div>
      {confirmDelete && merchant && <section className="space-y-3 rounded-lg border border-red-200 bg-red-50 p-3" aria-label="Confirm merchant deletion">
        <p className="text-sm text-red-900">Delete <strong>{merchant.name}</strong>? It will be removed from Merchants and the test pickers. Saved transaction history is retained.</p>
        <div className="flex flex-wrap gap-2">
          <button type="button" className={actionButton.danger} disabled={deleting} onClick={() => void deleteMerchant()}>
            {deleting ? "Deleting…" : "Confirm deletion"}
          </button>
          <button type="button" className={secondary} disabled={deleting} onClick={() => setConfirmDelete(false)}>Cancel</button>
        </div>
      </section>}
      {saved && <p role="status" className={stateBadge("positive")}><Check size={14} aria-hidden="true" />Merchant saved</p>}
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
    </form>
  );
}
