"use client";

import { useState } from "react";
import type { FormEvent } from "react";
import { outlineButton, primaryButton } from "../system-rules/contracts";
import { parseProposal, type Proposal } from "./proposal";

type LegacyJevRule = Proposal & { id: string };
export type LegacyJevRules = { version: string; rules: LegacyJevRule[] };
const comparisons = {
  gte: "At least",
  gt: "Greater than",
  lte: "At most",
  lt: "Less than",
  eq: "Equal to",
};
const signalLabel = { amountMinor: "Booking amount (minor units)", bookingVelocity60s: "Bookings in 60 seconds" };

export function LegacyJevRuleItem({ rule, version, busy, onChange }: {
  rule: LegacyJevRule;
  version: string;
  busy: boolean;
  onChange: (proposal?: Proposal) => Promise<boolean>;
}) {
  const [editing, setEditing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  return <li className="space-y-3 py-3">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="min-w-0 space-y-1">
        <p className="text-sm font-semibold wrap-anywhere">{rule.name}</p>
        <p className="text-xs text-muted">Active · {version} · Review · Local amount check</p>
        <p className="text-xs text-muted">{rule.conditions.map(({ signal, operator, value }) =>
          `${signalLabel[signal]} ${comparisons[operator].toLowerCase()} ${value}`).join(" and ")}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        <button type="button" className={outlineButton} disabled={busy} aria-label={`Edit ${rule.name}`} onClick={() => {
          setEditing(value => !value);
          setConfirmDelete(false);
        }}>{editing ? "Cancel edit" : "Edit"}</button>
        <button type="button" className={outlineButton} disabled={busy || confirmDelete} aria-label={`Delete ${rule.name}`} onClick={() => {
          setConfirmDelete(true);
          setEditing(false);
        }}>Delete</button>
      </div>
    </div>
    {editing && <LegacyRuleEditor rule={rule} busy={busy} onSave={async proposal => {
      if (await onChange(proposal)) setEditing(false);
    }} />}
    {confirmDelete && <div className="space-y-2 rounded-md border border-red-200 bg-red-50 p-3">
      <p className="text-sm text-red-900">Delete <strong>{rule.name}</strong> from future decisions?</p>
      <div className="flex flex-wrap gap-2">
        <button type="button" className="rounded-md bg-red-700 px-3 py-1.5 text-sm font-semibold text-white disabled:opacity-55"
          disabled={busy} onClick={() => void onChange()}>Confirm deletion</button>
        <button type="button" className={outlineButton} disabled={busy} onClick={() => setConfirmDelete(false)}>Cancel</button>
      </div>
    </div>}
  </li>;
}

type Condition = Proposal["conditions"][number];

function LegacyRuleEditor({ rule, busy, onSave }: {
  rule: LegacyJevRule;
  busy: boolean;
  onSave: (proposal: Proposal) => Promise<void>;
}) {
  const [name, setName] = useState(rule.name);
  const [conditions, setConditions] = useState(() => rule.conditions.map(({ value, ...condition }) => ({ ...condition, value: String(value) })));
  const [error, setError] = useState("");

  function changeCondition(index: number, change: Partial<(typeof conditions)[number]>) {
    setConditions(previous => previous.map((condition, position) => position === index ? { ...condition, ...change } : condition));
    setError("");
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    try {
      if (conditions.some(condition => !/^\d+$/.test(condition.value.trim()))) {
        throw new Error("Enter a nonnegative whole amount for each condition.");
      }
      const proposal = parseProposal(JSON.stringify({
        name, action: "REVIEW", conditions: conditions.map(condition => ({ ...condition, value: Number(condition.value) })),
      }));
      void onSave(proposal);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Review the rule conditions.");
    }
  }

  return <form onSubmit={submit} className="space-y-2 rounded-lg border border-line bg-paper p-3">
    <fieldset disabled={busy} className="space-y-3">
      <legend className="text-sm font-semibold">Edit booking review rule</legend>
      <label className="block text-sm font-medium">Rule name
        <input className="control mt-1" required maxLength={80} value={name} onChange={event => setName(event.currentTarget.value)} />
      </label>
      {conditions.map((condition, index) => <fieldset key={index} className="space-y-2">
        <legend className="text-xs font-semibold">Booking amount condition {index + 1}</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          <label className="text-sm font-medium">Comparison
            <select className="control mt-1" value={condition.operator} onChange={event =>
              changeCondition(index, { operator: event.currentTarget.value as Condition["operator"] })}>
              {Object.entries(comparisons).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
            </select>
          </label>
          <label className="text-sm font-medium">Amount in minor units
            <input className="control mt-1" value={condition.value} required inputMode="numeric" onChange={event =>
              changeCondition(index, { value: event.currentTarget.value })} />
          </label>
        </div>
        {conditions.length > 1 && <button type="button" className="text-xs font-semibold text-red-800 hover:underline" onClick={() =>
          setConditions(previous => previous.filter((_, position) => position !== index))}>Remove condition {index + 1}</button>}
      </fieldset>)}
      {conditions.length < 5 && <button className={outlineButton} type="button" onClick={() =>
        setConditions(previous => [...previous, { signal: "amountMinor", operator: "gte", value: "" }])}>Add condition</button>}
      {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
      <div><button className={primaryButton} type="submit">{busy ? "Saving…" : "Save changes"}</button></div>
    </fieldset>
  </form>;
}
