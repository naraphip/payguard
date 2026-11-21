"use client";

import type { RuleTransport } from "./contracts";

import { useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import Link from "next/link";
import { ArrowUpRight, CircleAlert, FlaskConical, LoaderCircle, LockKeyhole, Plus, Trash2, X } from "lucide-react";
import {
  actions, fieldControl, formatDate, operators, outlineButton, primaryButton,
  globalRuleTransport, stages,
} from "./contracts";
import type { Action, Condition, Operator, Preview, PreviewRun, Rule, RuleInput, Signal, Stage } from "./contracts";

const draftLabel = "Draft rules are available for review and testing in the sandbox.";

function defaultCondition(signal: Signal | undefined): Condition {
  return signal?.type === "boolean"
    ? { signal: signal.id, operator: "eq", value: true }
    : { signal: signal?.id ?? "", operator: "gte", value: 1 };
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="grid min-w-0 gap-1.5 text-sm font-medium text-slate-800">{label}{children}</label>;
}

function Message({ children }: { children: ReactNode }) {
  return <div role="alert" className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">
    <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />{children}
  </div>;
}

function PreviewResults({ result }: { result: Preview }) {
  return <div className="space-y-3" role="status">
    <dl className="grid grid-cols-3 gap-2">
      {[
        { label: "Evaluated", value: result.eligible },
        { label: "Matched", value: result.matched },
        { label: "Skipped", value: result.skipped },
      ].map(({ label, value }) => <div key={label} className="rounded-lg border border-slate-200 bg-white p-2.5">
        <dt className="text-xs text-slate-600">{label}</dt>
        <dd className="mt-1 text-lg font-semibold text-slate-900 tabular-nums">{value}</dd>
      </div>)}
    </dl>
    {result.skipped > 0 && <p className="text-xs text-slate-600">Skipped events were missing a signal required by this rule.</p>}
    {result.matchedEventIds.length > 0 ? <details className="text-sm text-slate-700">
      <summary className="cursor-pointer font-medium">Matching event IDs</summary>
      <ul className="mt-2 max-h-36 space-y-1 overflow-auto rounded-lg border border-slate-200 bg-white p-2.5 text-xs">
        {result.matchedEventIds.map(id => <li key={id} className="wrap-anywhere"><code>{id}</code></li>)}
      </ul>
    </details> : <p className="text-sm text-slate-600">No matching events.</p>}
    <p className="text-xs text-slate-600">Run <code>{result.runId}</code> · Engine <code>{result.ruleVersion}</code></p>
  </div>;
}

type Props = {
  rule?: Rule;
  signals: Signal[];
  runs: PreviewRun[];
  onClose: () => void;
  onSaved: (rule: Rule) => void;
  onDeleted: () => void;
  request?: RuleTransport;
  canEdit?: boolean;
};

export function RuleSheet({ rule, signals, runs, onClose, onSaved, onDeleted, request = globalRuleTransport, canEdit = true }: Props) {
  const readOnly = !!rule && (!canEdit || rule.status === "ACTIVE");
  const dialog = useRef<HTMLDialogElement>(null);
  const nameInput = useRef<HTMLInputElement>(null);
  const [form, setForm] = useState<RuleInput>(() => rule
    ? {
      name: rule.name, description: rule.description, stage: rule.stage,
      action: rule.action, conditions: rule.conditions,
    }
    : {
      name: "", description: "", stage: "ANY", action: "REVIEW",
      conditions: [defaultCondition(signals.find(signal => signal.stages.includes("ANY")))],
    });
  const [runId, setRunId] = useState(runs[0]?.id ?? "");
  const [task, setTask] = useState<"save" | "delete" | "preview" | null>(null);
  const [saveError, setSaveError] = useState("");
  const [previewError, setPreviewError] = useState("");
  const [preview, setPreview] = useState<Preview | null>(null);
  const hasPreviewFields = form.conditions.every(condition => condition.signal
    && (typeof condition.value === "boolean" || (Number.isFinite(condition.value) && Math.abs(condition.value) <= Number.MAX_SAFE_INTEGER)));
  const available = signals.filter(signal => signal.stages.includes(form.stage));

  async function removeDraft() {
    if (!rule || readOnly || task || !window.confirm(`Delete the draft "${rule.name}"?`)) return;
    setTask("delete");
    setSaveError("");
    try {
      await request(`/${encodeURIComponent(rule.id)}`, { method: "DELETE", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ version: rule.version }) });
      onDeleted();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : "Could not delete draft.");
    } finally { setTask(null); }
  }

  useEffect(() => {
    const modal = dialog.current;
    const trigger = document.activeElement;
    modal?.showModal();
    nameInput.current?.focus({ preventScroll: true });
    return () => {
      modal?.close();
      if (trigger instanceof HTMLElement && trigger.isConnected) trigger.focus();
    };
  }, []);

  function change(next: RuleInput) {
    setForm(next);
    setPreview(null);
    setPreviewError("");
  }

  function changeStage(stage: Stage) {
    const stageSignals = signals.filter(signal => signal.stages.includes(stage));
    change({
      ...form,
      stage,
      conditions: form.conditions.map(condition => stageSignals.some(signal => signal.id === condition.signal)
        ? condition
        : defaultCondition(undefined)),
    });
  }

  function changeCondition(index: number, condition: Condition) {
    change({ ...form, conditions: form.conditions.map((item, position) => index === position ? condition : item) });
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (readOnly || task) return;
    if (!form.name.trim() || !hasPreviewFields) {
      setSaveError("Enter a rule name and complete every condition.");
      return;
    }
    setSaveError("");
    setTask("save");
    try {
      const isEdit = Boolean(rule);
      const result = await request<Rule>(isEdit ? `/${rule?.id}` : "", {
        method: isEdit ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(rule ? { version: rule.version, ...form } : form),
      });
      onSaved(result);
    } catch (error) {
      setSaveError((error as Error).message);
    } finally {
      setTask(null);
    }
  }

  async function runPreview() {
    if (!runId || task) return;
    if (!form.name.trim() || !hasPreviewFields) {
      setPreviewError("Enter a rule name and complete every condition to preview this draft.");
      return;
    }
    setPreviewError("");
    setPreview(null);
    setTask("preview");
    try {
      setPreview(await request<Preview>("/preview", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ runId, rule: form }),
      }));
    } catch (error) {
      setPreviewError((error as Error).message);
    } finally {
      setTask(null);
    }
  }

  return <dialog
    ref={dialog}
    aria-labelledby="rule-sheet-title"
    aria-describedby="rule-sheet-description"
    className="fixed inset-y-0 right-0 left-auto m-0 h-dvh max-h-dvh w-full max-w-none overflow-hidden border-l border-slate-200 bg-white text-slate-900 shadow-2xl backdrop:bg-slate-950/35 sm:w-[570px]"
    onCancel={event => {
      event.preventDefault();
      if (task !== "save") onClose();
    }}
    onClick={event => { if (event.target === event.currentTarget && !task) onClose(); }}
  >
    <form onSubmit={save} className="flex h-full min-h-0 flex-col">
      <div className="flex items-start justify-between gap-3 border-b border-slate-200 px-5 py-4 sm:px-6">
        <div className="min-w-0">
          <h2 id="rule-sheet-title" className="wrap-anywhere text-xl font-semibold tracking-tight">{readOnly ? rule.name : rule ? "Edit draft" : "New draft"}</h2>
          <p id="rule-sheet-description" className="mt-1.5 text-sm text-slate-600">
            {readOnly ? rule.status === "ACTIVE" ? "Active built-in platform rule" : "Merchant draft" : draftLabel}
          </p>
        </div>
        <button type="button" className="grid size-8 shrink-0 place-items-center rounded-lg text-slate-500 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-50" aria-label="Close rule editor" onClick={onClose} disabled={task === "save"}>
          <X className="size-4" aria-hidden="true" />
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5 sm:p-6">
        {readOnly ? <div className="space-y-6">
          <div className="space-y-2">
            {rule.code && <code className="block wrap-anywhere text-xs font-medium text-slate-600">{rule.code}</code>}
            <p className="text-sm leading-relaxed text-slate-700">{rule.description}</p>
          </div>
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <div className="rounded-lg border border-slate-200 p-3"><dt className="text-xs text-slate-600">Stage</dt><dd className="mt-1 font-medium">{stages[rule.stage]}</dd></div>
            <div className="rounded-lg border border-slate-200 p-3"><dt className="text-xs text-slate-600">Action</dt><dd className="mt-1 font-medium">{actions[rule.action]}</dd></div>
          </dl>
          <section aria-labelledby="current-conditions" className="space-y-2">
            <h3 id="current-conditions" className="text-sm font-semibold">Conditions</h3>
            <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200">
              {rule.conditions.map((condition, index) => <li className="px-3.5 py-2.5 text-sm text-slate-700" key={index}>
                <span className="font-medium text-slate-900">{signals.find(signal => signal.id === condition.signal)?.label ?? condition.signal}</span>
                {" "}{operators[condition.operator]}{" "}{String(condition.value)}
              </li>)}
            </ul>
          </section>
          <p className="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm text-slate-700">
            <LockKeyhole className="size-4 shrink-0 text-slate-500" aria-hidden="true" />{rule.status === "ACTIVE" ? "Built-in rules are managed by the simulation engine." : "An administrator can edit merchant drafts."}
          </p>
        </div> : <fieldset disabled={task !== null} className="min-w-0 space-y-6">
          <div className="space-y-4">
            <Field label="Rule name">
              <input ref={nameInput} type="text" required className={fieldControl} placeholder="Example: High device reuse"
                value={form.name} onChange={event => change({ ...form, name: event.target.value })} maxLength={80} />
            </Field>
            <Field label="Description">
              <textarea className={`${fieldControl} h-auto min-h-[70px] resize-y py-2`} rows={2} value={form.description} maxLength={500}
                onChange={event => change({ ...form, description: event.target.value })} />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Stage"><select className={fieldControl} value={form.stage} onChange={event => changeStage(event.target.value as Stage)}>
                {Object.entries(stages).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select></Field>
              <Field label="Proposed action"><select className={fieldControl} value={form.action} onChange={event => change({ ...form, action: event.target.value as Action })}>
                {Object.entries(actions).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select></Field>
            </div>
          </div>
          <section aria-labelledby="new-conditions" className="space-y-3 border-t border-slate-200 pt-5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h3 id="new-conditions" className="text-sm font-semibold">Conditions</h3>
                <p className="mt-1 text-xs text-slate-600">All conditions must match.</p>
              </div>
              <button type="button" className={`${outlineButton} h-8 text-xs`} disabled={form.conditions.length >= 5 || !available.length}
                onClick={() => change({ ...form, conditions: [...form.conditions, defaultCondition(available[0])] })}>
                <Plus className="size-3.5" aria-hidden="true" />Add condition
              </button>
            </div>
            {form.conditions.map((condition, index) => {
              const signal = available.find(item => item.id === condition.signal);
              return <fieldset key={index} className="min-w-0 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5">
                <legend className="px-1 text-xs font-medium text-slate-600">Condition {index + 1}</legend>
                <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-2">
                  <Field label="Signal"><select required className={fieldControl} value={condition.signal}
                    onChange={event => changeCondition(index, defaultCondition(available.find(item => item.id === event.target.value)))}>
                    <option value="" disabled>Choose a signal</option>
                    {available.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
                  </select></Field>
                  <button type="button" className="mt-auto grid size-9 place-items-center rounded-lg border border-slate-200 bg-white text-slate-600 hover:bg-slate-100 disabled:opacity-45"
                    disabled={form.conditions.length === 1} aria-label={`Remove condition ${index + 1}`}
                    onClick={() => change({ ...form, conditions: form.conditions.filter((_, position) => position !== index) })}>
                    <Trash2 className="size-4" aria-hidden="true" />
                  </button>
                </div>
                <div className="mt-2 grid min-w-0 grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)] gap-2">
                  <Field label="Operator"><select className={fieldControl} value={condition.operator}
                    onChange={event => changeCondition(index, { ...condition, operator: event.target.value as Operator })}>
                    {Object.entries(operators).filter(([key]) => signal?.type !== "boolean" || key === "eq" || key === "ne")
                      .map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                  </select></Field>
                  <Field label="Value">
                    {signal?.type === "boolean"
                      ? <select className={fieldControl} value={String(condition.value)}
                        onChange={event => changeCondition(index, { ...condition, value: event.target.value === "true" })}>
                        <option value="true">True</option><option value="false">False</option>
                      </select>
                      : <input required type="number" className={fieldControl} step="any" min={-Number.MAX_SAFE_INTEGER} max={Number.MAX_SAFE_INTEGER}
                        value={typeof condition.value === "number" && Number.isFinite(condition.value) ? condition.value : ""}
                        onChange={event => changeCondition(index, { ...condition, value: event.target.valueAsNumber })} />}
                  </Field>
                </div>
              </fieldset>;
            })}
          </section>

          <section aria-labelledby="preview-rule" className="space-y-3 rounded-xl border border-slate-200 bg-slate-50 p-3.5">
            <div className="flex items-start gap-2.5">
              <FlaskConical className="mt-0.5 size-4 shrink-0 text-indigo-700" aria-hidden="true" />
              <div>
                <h3 id="preview-rule" className="text-sm font-semibold">Preview on saved run</h3>
                <p className="mt-1 text-xs leading-relaxed text-slate-600">Compare this draft with saved synthetic events.</p>
              </div>
            </div>
            {runs.length > 0 ? <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
              <Field label="Simulation run"><select className={fieldControl} value={runId} onChange={event => {
                setRunId(event.target.value); setPreview(null); setPreviewError("");
              }}>
                {runs.map(run => <option key={run.id} value={run.id}>
                  {`Seed ${run.seed} · ${run.product.replaceAll("_", " ")} · ${run.scenario} · ${formatDate(run.createdAt)} · ${run.id.slice(0, 8)}`}
                </option>)}
              </select></Field>
              <button type="button" className={outlineButton} onClick={runPreview}>
                {task === "preview" && <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />}
                {task === "preview" ? "Previewing…" : "Preview"}
              </button>
            </div> : <Link href="/simulation-lab" className="inline-flex items-center gap-1.5 text-sm font-medium text-indigo-700 hover:underline">
              Save a run in Simulation Lab <ArrowUpRight className="size-4" aria-hidden="true" />
            </Link>}
            {previewError && <Message>{previewError}</Message>}
            {preview && <PreviewResults result={preview} />}
          </section>
        </fieldset>}
      </div>
      <footer className="shrink-0 border-t border-slate-200 bg-white px-5 py-3.5 sm:px-6">
        {saveError && <div className="mb-3"><Message>{saveError}</Message></div>}
        <div className="flex flex-wrap items-center justify-end gap-2">
          {rule?.updatedAt && !readOnly && <span className="mr-auto text-xs text-slate-600">Revision {rule.version} · {formatDate(rule.updatedAt)}</span>}
          {rule && !readOnly && <button type="button" className="mr-auto text-sm font-semibold text-red-700" disabled={!!task} onClick={() => void removeDraft()}>Delete draft</button>}
          <button type="button" className={outlineButton} onClick={onClose} disabled={task === "save"}>{readOnly ? "Close" : "Cancel"}</button>
          {!readOnly && <button type="submit" className={primaryButton} disabled={task !== null}>
            {task === "save" && <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />}
            {task === "save" ? "Saving…" : "Save draft"}
          </button>}
        </div>
      </footer>
    </form>
  </dialog>;
}
