"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { merchantDemoRequest } from "../../merchant-demo";
import { examplePrompts, parseProposal, type Proposal } from "./proposal";

type Snapshot = { version: string; rules: (Proposal & { id: string })[] };
type Preview = { evaluated: number; matched: number; missing: number };
type Message = { role: "You" | "Jev Rules"; text: string };
type RuleRevision = { id: string; version: string };

const endpoint = "/api/v1/booking/jev/rules";
const aiInstructions = [
  "Convert my booking review policy into a single JSON object with name, action, and conditions.",
  "Use REVIEW for action and 1–5 conditions joined by AND. Each condition has signal, operator, and value.",
  "Signals: amountMinor (minor currency units) or bookingVelocity60s (bookings in 60 seconds).",
  "Operators: gte, gt, lte, lt, eq. Values must be nonnegative integers up to 9007199254740991.",
  "Ask me to clarify any missing units or unsupported logic. My policy is:",
].join("\n");
const operators = { gte: "≥", gt: ">", lte: "≤", lt: "<", eq: "=" };

function describe(rule: Proposal) {
  return rule.conditions.map(({ signal, operator, value }) =>
    `${signal === "amountMinor" ? "Booking amount (minor units)" : "Bookings in 60 seconds"} ${operators[operator]} ${value}`,
  ).join(" AND ");
}

const button = "rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium disabled:opacity-50";
const primary = `${button} border-indigo-700 bg-indigo-700 text-white`;

export function JevRulesPanel({ merchant }: {
  merchant?: { id: string; name: string; token: string; canEdit: boolean };
} = {}) {
  const Container = merchant ? "section" : "main";
  const Heading = merchant ? "h2" : "h1";
  const canEdit = merchant?.canEdit ?? true;
  const [token, setToken] = useState("");
  const canRequest = Boolean(merchant || token.trim());
  const [input, setInput] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [editing, setEditing] = useState<RuleRevision | null>(null);
  const [removing, setRemoving] = useState<RuleRevision | null>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (merchant) void request("load");
  }, [merchant?.id, merchant?.token]);

  function submitPrompt() {
    if (!input.trim()) return;
    let reply: string;
    setPreview(null);
    setStatus("");
    try {
      const candidate = parseProposal(input);
      if (merchant && candidate.conditions.some(condition => condition.signal !== "amountMinor")) {
        throw new Error("Merchant Jev review rules support booking amount conditions.");
      }
      setProposal(candidate);
      reply = "Draft ready for review.";
    } catch (err) {
      setProposal(null);
      reply = err instanceof Error ? err.message : "The rule could not be parsed.";
    }
    setMessages([...messages, { role: "You", text: input.trim() }, { role: "Jev Rules", text: reply }]);
    setInput("");
  }

  function changeInput(value: string) {
    setInput(value);
    if (!editing) return;
    setProposal(null);
    setPreview(null);
  }

  function editRule(rule: Snapshot["rules"][number], version: string) {
    setEditing({ id: rule.id, version });
    setRemoving(null);
    setInput(JSON.stringify({ name: rule.name, action: rule.action, conditions: rule.conditions }, null, 2));
    setProposal(null);
    setPreview(null);
    setError("");
    setStatus("");
    document.getElementById("jev-rule-prompt")?.focus();
  }

  function cancelEdit() {
    setEditing(null);
    setInput("");
    setProposal(null);
    setPreview(null);
  }

  async function request(operation: "load" | "preview" | "push" | "remove") {
    if (!canRequest || (operation !== "load" && !canEdit)) return;
    if ((operation === "preview" || operation === "push") && !proposal) return;
    if (operation === "remove" && (!merchant || !removing)) return;
    const target = operation === "remove" ? removing : operation === "push" ? editing : null;
    setBusy(true);
    setError("");
    setStatus("");
    try {
      const suffix = target ? `/${encodeURIComponent(target.id)}` : operation === "preview" ? "/preview" : "";
      let body: string | undefined;
      if (operation === "remove") body = JSON.stringify({ expectedVersion: target?.version });
      else if (operation !== "load") {
        body = JSON.stringify(target ? { definition: proposal, expectedVersion: target.version } : proposal);
      }
      const init = {
        method: operation === "load" ? "GET" : operation === "remove" ? "DELETE" : target ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body,
      };
      let result: Snapshot | Preview;
      if (merchant) {
        result = await merchantDemoRequest<Snapshot | Preview>(`/${encodeURIComponent(merchant.id)}/jev/rules${suffix}`, merchant.token, init);
      } else {
        const response = await fetch(endpoint + suffix, {
          ...init, headers: { ...init.headers, Authorization: `Bearer ${token.trim()}` }, cache: "no-store",
        });
        const payload = await response.json().catch(() => null);
        if (!response.ok || !payload) throw new Error(payload?.error ?? "Jev Rules API unavailable. Check the local booking service.");
        result = payload;
      }
      if (operation === "preview") {
        setPreview(result as Preview);
      } else {
        setSnapshot(result as Snapshot);
        setRemoving(null);
        if (operation === "push") {
          cancelEdit();
          setStatus(target ? "Jev review rule updated." : merchant ? "Rule is active for new merchant tests." : "Rule is active for new booking assessments.");
        } else if (operation === "remove") {
          if (editing?.id === target?.id) cancelEdit();
          setStatus("Jev review rule removed.");
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Jev Rules request failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Container className={merchant ? "min-w-0 space-y-6" : "mx-auto min-w-0 max-w-5xl space-y-6 p-4 md:p-8"}>
      <header>
        {!merchant && <Link href="/settings/merchant-rules" className="mb-2 inline-block text-sm font-semibold text-accent hover:underline">← Merchant AI Rules</Link>}
        <Heading className={merchant ? "text-lg font-semibold" : "text-3xl font-bold text-slate-900"}>{merchant ? "Booking review rules (Jev)" : "Legacy booking review rules"}</Heading>
        <p className="mt-2 text-slate-600">{merchant
          ? `Review amount-based Jev conditions for ${merchant.name}.`
          : "Author, preview, and publish review rules using the legacy booking credential."}</p>
      </header>

      <section className="space-y-3 rounded-xl border border-slate-200 bg-white p-4">
        {!merchant && <><label className="block text-sm font-medium" htmlFor="jev-rules-token">Merchant demo token</label>
        <div className="flex flex-wrap gap-2">
          <input id="jev-rules-token" type="password" autoComplete="off" value={token} disabled={busy}
            onChange={(event) => {
              setToken(event.target.value);
              setSnapshot(null);
              setPreview(null);
              setStatus("");
              setError("");
            }}
            className="min-w-0 flex-1 rounded-lg border border-slate-300 px-3 py-2" />
          <button className={button} disabled={busy || !canRequest} onClick={() => void request("load")}>Load active rules</button>
        </div>
        </>}
        {merchant && <button className={button} disabled={busy} onClick={() => void request("load")}>{busy ? "Loading…" : "Refresh rules"}</button>}
        {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
        {status && <p role="status" className="text-sm text-emerald-800">{status}</p>}
      </section>

      {canEdit && <section className="space-y-4 rounded-xl border border-slate-200 bg-white p-4">
        <h2 className="text-lg font-semibold">{editing ? "Edit Jev review rule" : "Rule chat"}</h2>
        <p className="text-sm text-slate-600">Use one of the supported prompt formats, or paste rule JSON generated by your AI chat.</p>
        <div className="flex flex-wrap gap-2">
          {examplePrompts.filter(prompt => !merchant || !/60 seconds|bookings in/i.test(prompt)).map((prompt) => <button key={prompt} className={button} disabled={busy} onClick={() => changeInput(prompt)}>{prompt}</button>)}
        </div>
        <details className="text-sm text-slate-600">
          <summary className="cursor-pointer font-medium">Prompt for an external AI chat</summary>
          <label htmlFor="jev-ai-instructions" className="sr-only">Copyable AI prompt</label>
          <textarea id="jev-ai-instructions" readOnly rows={6} value={merchant ? aiInstructions.replace(" or bookingVelocity60s (bookings in 60 seconds)", "") : aiInstructions}
            onFocus={(event) => event.target.select()} className="mt-2 w-full rounded-lg border border-slate-300 px-3 py-2" />
        </details>
        {messages.length > 0 && <ol aria-label="Rule conversation" className="max-h-72 space-y-2 overflow-y-auto">
          {messages.map(({ role, text }, index) => <li key={index} className="rounded-lg bg-slate-50 p-3 text-sm">
            <p className="font-semibold">{role}</p>
            <p className="whitespace-pre-wrap break-words">{text}</p>
          </li>)}
        </ol>}
        <form className="space-y-2" onSubmit={(event) => { event.preventDefault(); submitPrompt(); }}>
          <label className="block text-sm font-medium" htmlFor="jev-rule-prompt">Rule prompt or JSON</label>
          <textarea id="jev-rule-prompt" value={input} onChange={(event) => changeInput(event.target.value)}
            rows={3} maxLength={4000} disabled={busy} className="w-full rounded-lg border border-slate-300 px-3 py-2" />
          <div className="flex flex-wrap gap-2">
            <button className={primary} disabled={busy || !input.trim()}>{editing ? "Review changes" : "Draft rule"}</button>
            {editing && <button className={button} type="button" disabled={busy} onClick={cancelEdit}>Cancel edit</button>}
          </div>
        </form>
      </section>}

      {canEdit && proposal && <section className="space-y-3 rounded-xl border border-indigo-200 bg-indigo-50 p-4">
        <h2 className="text-lg font-semibold">Review draft</h2>
        <p className="font-medium">{proposal.name}</p>
        <p className="text-sm">{describe(proposal)} → Manual review</p>
        {preview && <p role="status" className="text-sm">{preview.matched} matched across {preview.evaluated} saved assessments. {preview.missing} lacked required evidence.</p>}
        <div className="flex flex-wrap gap-2">
          <button className={button} disabled={busy || !canRequest} onClick={() => void request("preview")}>Preview on saved assessments</button>
          <button className={primary} disabled={busy || !canRequest} onClick={() => void request("push")}>
            {editing ? "Save Jev review rule" : merchant ? "Publish Jev review rule" : "Push to Jev"}
          </button>
        </div>
      </section>}

      <section className="space-y-3 rounded-xl border border-slate-200 bg-white p-4">
        <h2 className="text-lg font-semibold">Active rules</h2>
        <p className="text-sm text-slate-600">{merchant ? "Published review rules apply to new tests for this merchant." : "Default built-in · Bookings in 60 seconds ≥ 5 → Manual review"}</p>
        {snapshot && <>
          <p className="break-all text-sm text-slate-600">Version {snapshot.version}</p>
          {snapshot.rules.length === 0 && <p className="text-sm text-slate-600">No custom rules published.</p>}
          <ul className="divide-y divide-slate-200">
            {snapshot.rules.map((rule) => <li key={rule.id} className="py-2">
              <p className="font-medium">{rule.name}</p>
              <p className="text-sm text-slate-600">{describe(rule)} → Manual review</p>
              <p className="break-all font-mono text-xs text-slate-600">{rule.id}</p>
              {merchant && canEdit && <div className="mt-2 flex flex-wrap items-center gap-2">
                {removing?.id === rule.id ? <>
                  <p className="text-sm text-slate-700">Remove this rule from the active ruleset?</p>
                  <button className={button} type="button" disabled={busy} onClick={() => void request("remove")}>Confirm removal</button>
                  <button className={button} type="button" disabled={busy} onClick={() => setRemoving(null)}>Cancel</button>
                </> : <>
                  <button className={button} type="button" disabled={busy} aria-label={`Edit ${rule.name}`}
                    onClick={() => editRule(rule, snapshot.version)}>Edit</button>
                  <button className={button} type="button" disabled={busy} aria-label={`Remove ${rule.name}`}
                    onClick={() => setRemoving({ id: rule.id, version: snapshot.version })}>Remove</button>
                </>}
              </div>}
            </li>)}
          </ul>
        </>}
      </section>
    </Container>
  );
}
