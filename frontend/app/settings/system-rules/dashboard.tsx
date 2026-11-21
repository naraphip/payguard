"use client";

import { useEffect, useState } from "react";
import { ArrowRight, ChevronRight, CircleAlert, FilePenLine, Layers3, LoaderCircle, Plus, Search, ShieldCheck } from "lucide-react";
import { PayGuardLogo } from "../../payguard-logo";
import { SettingsNavigation } from "../navigation";
import { RuleSheet } from "./rule-sheet";
import { actions, outlineButton, primaryButton, ruleRequest, rulesEndpoint, stages } from "./contracts";
import type { Rule, RuleCatalogue } from "./contracts";

const filters = { all: "All rules", ACTIVE: "Active", DRAFT: "Drafts" } as const;
type Filter = keyof typeof filters;

export function StatusBadge({ status }: { status: Rule["status"] }) {
  return status === "ACTIVE"
    ? <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-800">
      <span className="size-1.5 rounded-full bg-emerald-600" aria-hidden="true" />Active
    </span>
    : <span className="inline-flex items-center gap-1.5 rounded-full border border-slate-200 bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">
      <span className="size-1.5 rounded-full bg-slate-500" aria-hidden="true" />Draft
    </span>;
}

export function ActionBadge({ action }: { action: Rule["action"] }) {
  const color = action === "REVIEW" ? "bg-amber-50 text-amber-900 ring-amber-200" : "bg-indigo-50 text-indigo-800 ring-indigo-200";
  return <span className={`inline-flex rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${color}`}>{actions[action]}</span>;
}

export function SystemRulesDashboard() {
  const [catalogue, setCatalogue] = useState<RuleCatalogue | null>(null);
  const [loadError, setLoadError] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadKey, setLoadKey] = useState(0);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [editor, setEditor] = useState<Rule | "new" | null>(null);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    ruleRequest<RuleCatalogue>(rulesEndpoint, { signal: controller.signal, cache: "no-store" })
      .then(result => {
        if (!controller.signal.aborted) {
          setCatalogue(result);
          setLoading(false);
        }
      })
      .catch(error => {
        if (!controller.signal.aborted) {
          setLoadError((error as Error).message);
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [loadKey]);

  function refresh() {
    setLoadError("");
    setLoading(true);
    setLoadKey(key => key + 1);
  }

  function saved(rule: Rule) {
    setCatalogue(data => data && {
      ...data,
      rules: [...data.rules.filter(item => item.status === "ACTIVE"), rule,
        ...data.rules.filter(item => item.status === "DRAFT" && item.id !== rule.id)],
    });
    setNotice(`${rule.name} saved as a draft.`);
    setQuery("");
    setFilter("DRAFT");
    setEditor(null);
  }

  const rules = catalogue?.rules ?? [];
  const queryText = query.trim().toLowerCase();
  const counts = { all: rules.length, ACTIVE: rules.filter(item => item.status === "ACTIVE").length, DRAFT: rules.filter(item => item.status === "DRAFT").length };
  const filtered = rules.filter(rule =>
    (filter === "all" || rule.status === filter)
    && (!queryText || [rule.name, rule.description, rule.code ?? "", stages[rule.stage], actions[rule.action]]
      .some(value => value.toLowerCase().includes(queryText))),
  );

  return <div className="min-h-screen bg-[#f8fafc] text-slate-900 md:grid md:grid-cols-[218px_minmax(0,1fr)]">
    <aside className="border-b border-slate-200 bg-white md:sticky md:top-0 md:h-dvh md:border-r md:border-b-0">
      <div className="px-4 pt-4 pb-3 md:border-b md:border-slate-100 md:px-5 md:py-4">
        <PayGuardLogo />
      </div>
      <div className="px-4 pb-3 md:px-3 md:py-5">
        <SettingsNavigation current="/settings/system-rules" />
      </div>
    </aside>

    <main className="min-w-0">
      <div className="flex min-h-[69px] items-center justify-between gap-3 border-b border-slate-200 bg-white px-4 sm:px-7 lg:px-10">
        <div className="flex min-w-0 items-center gap-2 text-sm text-slate-500">
          <span>Settings</span><ChevronRight className="size-3.5 shrink-0 text-slate-400" aria-hidden="true" />
          <span className="truncate font-medium text-slate-900">Platform Rules</span>
        </div>
        <span className="inline-flex items-center gap-1.5 rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs font-medium text-slate-700">
          <span className="size-1.5 rounded-full bg-teal-600" aria-hidden="true" />Sandbox
        </span>
      </div>

      <div className="mx-auto max-w-[1360px] space-y-6 px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:space-y-7 lg:px-10">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-[30px] font-semibold leading-tight tracking-[-0.035em] sm:text-[36px]">Platform Rule Settings</h1>
            <p className="mt-2 max-w-xl text-sm leading-relaxed text-slate-600">Review system fraud rules and test drafts against saved Simulation Lab runs.</p>
          </div>
          <button type="button" className={`${primaryButton} self-start sm:shrink-0 sm:self-auto`} disabled={!catalogue} onClick={() => {
            setNotice("");
            setEditor("new");
          }}>
            <Plus className="size-4" aria-hidden="true" />New draft
          </button>
        </header>

        <div className="grid grid-cols-3 gap-2.5 sm:gap-3">
          {[
            { label: "Total rules", total: counts.all, Icon: Layers3, color: "bg-indigo-50 text-indigo-700" },
            { label: "Active rules", total: counts.ACTIVE, Icon: ShieldCheck, color: "bg-emerald-50 text-emerald-700" },
            { label: "Draft rules", total: counts.DRAFT, Icon: FilePenLine, color: "bg-slate-100 text-slate-700" },
          ].map(({ label, total, Icon, color }) =>
            <div key={label} className="flex min-w-0 items-center justify-between gap-3 rounded-xl border border-slate-200 bg-white px-2.5 py-2.5 shadow-xs sm:px-5 sm:py-4">
              <div>
                <p className="text-xs font-medium text-slate-600 sm:text-sm">{label}</p>
                <p className="mt-1 text-2xl font-semibold tracking-tight tabular-nums">{catalogue ? total : "—"}</p>
              </div>
              <div className={`hidden size-9 shrink-0 items-center justify-center rounded-lg sm:flex ${color}`}><Icon className="size-[18px]" aria-hidden="true" /></div>
            </div>,
          )}
        </div>

        {notice && <p role="status" className="flex items-start gap-2 rounded-lg border border-emerald-200 bg-emerald-50 px-3.5 py-2.5 text-sm text-emerald-800">
          <ShieldCheck className="mt-0.5 size-4 shrink-0" aria-hidden="true" />{notice}
        </p>}

        <section aria-labelledby="rule-library" className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xs">
          <div className="flex flex-wrap items-start justify-between gap-3 border-b border-slate-100 px-4 py-4 sm:px-5">
            <div>
              <h2 id="rule-library" className="text-base font-semibold tracking-tight">Rule library</h2>
              <p className="mt-1 text-sm text-slate-600">Built-in safeguards and drafts for review.</p>
            </div>
            {catalogue && <span className="rounded-md border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs text-slate-600">
              Engine <code className="font-medium text-slate-800">{catalogue.ruleVersion}</code>
            </span>}
          </div>
          {catalogue && <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 px-4 py-3 sm:px-5">
            <div aria-label="Rule status" role="group" className="inline-flex max-w-full flex-wrap gap-0.5 rounded-lg bg-slate-100 p-0.5">
              {(Object.keys(filters) as Filter[]).map(key => <button
                key={key}
                type="button"
                aria-pressed={filter === key}
                className={`rounded-md px-2.5 py-1.5 text-xs font-medium transition ${filter === key ? "bg-white text-slate-900 shadow-xs" : "text-slate-600 hover:text-slate-900"}`}
                onClick={() => setFilter(key)}
              >{filters[key]} <span className="ml-0.5 text-slate-500 tabular-nums">{counts[key]}</span></button>)}
            </div>
            <label className="relative w-full min-w-0 sm:w-64">
              <span className="sr-only">Search rules</span>
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-slate-500" aria-hidden="true" />
              <input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search rules…" className="h-9 w-full rounded-lg border border-slate-200 bg-white pr-3 pl-9 text-sm shadow-xs placeholder:text-slate-500" />
            </label>
          </div>}

          {loadError && <div role="alert" className="flex flex-wrap items-center gap-3 border-b border-red-100 bg-red-50 px-4 py-4 text-sm text-red-800 sm:px-5">
            <CircleAlert className="size-4 shrink-0" aria-hidden="true" />
            <p className="min-w-0 flex-1">{loadError}</p><button type="button" className={outlineButton} onClick={refresh}>Retry</button>
          </div>}
          {loading && <div role="status" className="flex items-center justify-center gap-2 px-5 py-12 text-sm text-slate-600">
            <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />Loading rules…
          </div>}

          {!loading && catalogue && (filtered.length > 0 ? <>
            <div className="hidden overflow-x-auto lg:block">
              <table className="w-full border-collapse text-left text-sm">
                <thead className="bg-slate-50 text-xs font-medium text-slate-600">
                  <tr>
                    <th scope="col" className="w-[44%] px-5 py-3 font-medium">Rule</th>
                    <th scope="col" className="px-4 py-3 font-medium">Stage</th>
                    <th scope="col" className="px-4 py-3 font-medium">Action</th>
                    <th scope="col" className="px-4 py-3 font-medium">Status</th>
                    <th scope="col" className="px-5 py-3 text-right font-medium"><span className="sr-only">Open rule</span></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {filtered.map(rule => <tr key={rule.id} className="transition hover:bg-slate-50/70">
                    <td className="px-5 py-4"><RuleName rule={rule} /></td>
                    <td className="px-4 py-4 text-slate-600">{stages[rule.stage]}</td>
                    <td className="px-4 py-4"><ActionBadge action={rule.action} /></td>
                    <td className="px-4 py-4"><StatusBadge status={rule.status} /></td>
                    <td className="px-5 py-4 text-right"><OpenButton rule={rule} onOpen={setEditor} /></td>
                  </tr>)}
                </tbody>
              </table>
            </div>
            <ul aria-label="Rules" className="divide-y divide-slate-100 lg:hidden">
              {filtered.map(rule => <li key={rule.id} className="space-y-3 px-4 py-4 sm:px-5">
                <div className="flex items-start justify-between gap-3"><RuleName rule={rule} /><OpenButton rule={rule} onOpen={setEditor} /></div>
                <div className="flex flex-wrap items-center gap-2.5"><StatusBadge status={rule.status} /><ActionBadge action={rule.action} /><span className="text-xs text-slate-600">{stages[rule.stage]}</span></div>
              </li>)}
            </ul>
            <p className="border-t border-slate-100 px-4 py-3 text-xs text-slate-600 sm:px-5">{filtered.length} of {rules.length} {rules.length === 1 ? "rule" : "rules"}</p>
          </> : <div className="grid justify-items-center gap-3 px-4 py-12 text-center">
            <div className="grid size-10 place-items-center rounded-lg bg-slate-100 text-slate-600">
              {filter === "DRAFT" && !queryText ? <FilePenLine className="size-5" aria-hidden="true" /> : <Search className="size-5" aria-hidden="true" />}
            </div>
            {filter === "DRAFT" && !queryText ? <>
              <div><p className="text-sm font-semibold">No drafts yet</p><p className="mt-1 text-sm text-slate-600">Create a draft to test a proposed rule.</p></div>
              <button type="button" className={outlineButton} onClick={() => setEditor("new")}><Plus className="size-4" aria-hidden="true" />New draft</button>
            </> : <>
              <p className="text-sm font-semibold">No matching rules</p>
              <button type="button" className={outlineButton} onClick={() => { setQuery(""); setFilter("all"); }}>Clear filters</button>
            </>}
          </div>)}
        </section>
      </div>
    </main>

    {editor && catalogue && <RuleSheet
      rule={editor === "new" ? undefined : editor}
      signals={catalogue.signals}
      runs={catalogue.runs}
      onClose={() => setEditor(null)}
      onSaved={saved}
    />}
  </div>;
}

function RuleName({ rule }: { rule: Rule }) {
  return <div className="min-w-0">
    <p className="break-words text-sm font-semibold text-slate-900">{rule.name}</p>
    {rule.code && <code className="mt-1 block wrap-anywhere text-[10px] font-medium text-slate-500">{rule.code}</code>}
    {rule.description && <p className="mt-1.5 line-clamp-2 text-xs leading-relaxed text-slate-600">{rule.description}</p>}
  </div>;
}

function OpenButton({ rule, onOpen }: { rule: Rule; onOpen: (rule: Rule) => void }) {
  const verb = rule.status === "ACTIVE" ? "View" : "Edit";
  return <button type="button" className={`${outlineButton} h-8 shrink-0 text-xs`} aria-label={`${verb} ${rule.name}`} onClick={() => onOpen(rule)}>
    {verb}<ArrowRight className="size-3.5" aria-hidden="true" />
  </button>;
}
