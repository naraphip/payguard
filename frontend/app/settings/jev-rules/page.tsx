"use client";

import { useState } from "react";
import { examplePrompts, parseProposal, type Proposal } from "./proposal";

type Snapshot = { version: string; rules: (Proposal & { id: string })[] };
type Preview = { evaluated: number; matched: number; missing: number };
type Message = { role: "You" | "Jev Rules"; text: string };

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

export default function JevRulesPage() {
  const [token, setToken] = useState("");
  const [input, setInput] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  function submitPrompt() {
    if (!input.trim()) return;
    let reply: string;
    setPreview(null);
    setStatus("");
    try {
      const candidate = parseProposal(input);
      setProposal(candidate);
      reply = "Draft ready for review.";
    } catch (err) {
      setProposal(null);
      reply = err instanceof Error ? err.message : "The rule could not be parsed.";
    }
    setMessages([...messages, { role: "You", text: input.trim() }, { role: "Jev Rules", text: reply }]);
    setInput("");
  }

  async function request(operation: "load" | "preview" | "push") {
    if (!token.trim() || (operation !== "load" && !proposal)) return;
    setBusy(true);
    setError("");
    setStatus("");
    try {
      const response = await fetch(operation === "preview" ? `${endpoint}/preview` : endpoint, {
        method: operation === "load" ? "GET" : "POST",
        headers: { Authorization: `Bearer ${token.trim()}`, "Content-Type": "application/json" },
        cache: "no-store",
        body: operation === "load" ? undefined : JSON.stringify(proposal),
      });
      const result = await response.json().catch(() => null);
      if (!response.ok || !result) throw new Error(result?.error ?? "Jev Rules API unavailable. Check the local booking service.");
      if (operation === "preview") {
        setPreview(result);
      } else {
        setSnapshot(result);
        if (operation === "push") {
          setProposal(null);
          setPreview(null);
          setStatus("Rule is active for new booking assessments.");
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Jev Rules request failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="mx-auto min-w-0 max-w-5xl space-y-6 p-4 md:p-8">
      <header>
        <h1 className="text-3xl font-bold text-slate-900">Jev Rules</h1>
        <p className="mt-2 text-slate-600">Author, preview, and publish merchant review rules for the local booking evaluator.</p>
      </header>

      <section className="space-y-3 rounded-xl border border-slate-200 bg-white p-4">
        <label className="block text-sm font-medium" htmlFor="jev-rules-token">Merchant demo token</label>
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
          <button className={button} disabled={busy || !token.trim()} onClick={() => void request("load")}>Load active rules</button>
        </div>
        {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
        {status && <p role="status" className="text-sm text-emerald-800">{status}</p>}
      </section>

      <section className="space-y-4 rounded-xl border border-slate-200 bg-white p-4">
        <h2 className="text-lg font-semibold">Rule chat</h2>
        <p className="text-sm text-slate-600">Use one of the supported prompt formats, or paste rule JSON generated by your AI chat.</p>
        <div className="flex flex-wrap gap-2">
          {examplePrompts.map((prompt) => <button key={prompt} className={button} disabled={busy} onClick={() => setInput(prompt)}>{prompt}</button>)}
        </div>
        <details className="text-sm text-slate-600">
          <summary className="cursor-pointer font-medium">Prompt for an external AI chat</summary>
          <label htmlFor="jev-ai-instructions" className="sr-only">Copyable AI prompt</label>
          <textarea id="jev-ai-instructions" readOnly rows={6} value={aiInstructions}
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
          <textarea id="jev-rule-prompt" value={input} onChange={(event) => setInput(event.target.value)}
            rows={3} maxLength={4000} disabled={busy} className="w-full rounded-lg border border-slate-300 px-3 py-2" />
          <button className={primary} disabled={busy || !input.trim()}>Draft rule</button>
        </form>
      </section>

      {proposal && <section className="space-y-3 rounded-xl border border-indigo-200 bg-indigo-50 p-4">
        <h2 className="text-lg font-semibold">Review draft</h2>
        <p className="font-medium">{proposal.name}</p>
        <p className="text-sm">{describe(proposal)} → Manual review</p>
        {preview && <p role="status" className="text-sm">{preview.matched} matched across {preview.evaluated} saved assessments. {preview.missing} lacked required evidence.</p>}
        <div className="flex flex-wrap gap-2">
          <button className={button} disabled={busy || !token.trim()} onClick={() => void request("preview")}>Preview on saved assessments</button>
          <button className={primary} disabled={busy || !token.trim()} onClick={() => void request("push")}>Push to Jev</button>
        </div>
      </section>}

      <section className="space-y-3 rounded-xl border border-slate-200 bg-white p-4">
        <h2 className="text-lg font-semibold">Active rules</h2>
        <p className="text-sm text-slate-600">Default built-in · Bookings in 60 seconds ≥ 5 → Manual review</p>
        {snapshot && <>
          <p className="break-all text-sm text-slate-600">Version {snapshot.version}</p>
          {snapshot.rules.length === 0 && <p className="text-sm text-slate-600">No custom rules published.</p>}
          <ul className="divide-y divide-slate-200">
            {snapshot.rules.map((rule) => <li key={rule.id} className="py-2">
              <p className="font-medium">{rule.name}</p>
              <p className="text-sm text-slate-600">{describe(rule)} → Manual review</p>
              <p className="break-all font-mono text-xs text-slate-600">{rule.id}</p>
            </li>)}
          </ul>
        </>}
      </section>
    </main>
  );
}
