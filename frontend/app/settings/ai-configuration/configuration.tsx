"use client";

import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { RuleAccess, useRuleCredential } from "../rule-composer";
import { outlineButton, primaryButton, ruleRequest } from "../system-rules/contracts";

const endpoint = "/api/v1/booking/ai-configuration/jev";
const models = ["typesafe/jev-1.14", "typesafe/jev-1.13"] as const;
const rubrics = { "standard-v1": "Standard", "conservative-v1": "Conservative" } as const;
const card = "min-w-0 space-y-4 rounded-xl border border-line bg-white p-4 sm:p-5";
type Model = typeof models[number];
type Config = {
  provider: "mock" | "openrouter";
  model: Model;
  fallbackModel: Model | "";
  rubric: keyof typeof rubrics;
  timeoutMs: number;
};
type SavedConfig = { config: Config; version: number; configured: boolean };
type Diagnostic = {
  status: "LIVE" | "MOCKED" | "TIMEOUT" | "UNAVAILABLE";
  provider: Config["provider"];
  model: string;
  rubric: string;
  decision?: string;
  fallbackUsed: boolean;
  message?: string;
};

function request<T>(token: string, path = endpoint, init?: RequestInit) {
  const headers = new Headers({ Authorization: `Bearer ${token}` });
  if (init?.body) headers.set("Content-Type", "application/json");
  return ruleRequest<T>(path, { ...init, headers, cache: "no-store" });
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Could not reach the configuration service.";
}

export function AIConfiguration() {
  const token = useRuleCredential("PLATFORM");
  return <main className="mx-auto max-w-[1050px] space-y-5 px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header>
      <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">AI Configuration</h1>
      <p className="mt-2 text-sm text-muted">Configure and test the platform Jev advisory.</p>
    </header>
    <RuleAccess scope="PLATFORM" />
    {token && <ConfigurationForm key={token} token={token} />}
  </main>;
}

function ConfigurationForm({ token }: { token: string }) {
  const [saved, setSaved] = useState<SavedConfig | null>(null);
  const [form, setForm] = useState<Config | null>(null);
  const [result, setResult] = useState<Diagnostic | null>(null);
  const [task, setTask] = useState<"save" | "test" | null>(null);
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const active = useRef<AbortController>(null);
  const changed = JSON.stringify(form) !== JSON.stringify(saved?.config);

  useEffect(() => () => active.current?.abort(), []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void request<SavedConfig>(token, endpoint, { signal: controller.signal }).then(response => {
      if (!controller.signal.aborted) {
        setSaved(response);
        setForm(response.config);
        setResult(null);
      }
    }).catch(error => {
      if (!controller.signal.aborted) setError(errorMessage(error));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [token, revision]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!saved || !form || !changed || active.current) return;
    if (!Number.isInteger(form.timeoutMs) || form.timeoutMs < 250 || form.timeoutMs > 10000) {
      setError("Enter a timeout from 250 to 10,000 milliseconds.");
      return;
    }
    const controller = new AbortController();
    active.current = controller;
    setTask("save");
    setError("");
    setNotice("");
    setResult(null);
    try {
      const response = await request<SavedConfig>(token, endpoint, {
        method: "PUT", body: JSON.stringify({ config: form, expectedVersion: saved.version }), signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      setSaved(response);
      setForm(response.config);
      setNotice("Jev configuration saved.");
    } catch (error) {
      if (!controller.signal.aborted) setError(errorMessage(error));
    } finally {
      if (!controller.signal.aborted) setTask(null);
      active.current = null;
    }
  }

  async function test() {
    if (!saved || changed || active.current) return;
    const controller = new AbortController();
    active.current = controller;
    setTask("test");
    setError("");
    setNotice("");
    setResult(null);
    try {
      const response = await request<Diagnostic>(token, `${endpoint}/test`, {
        method: "POST", body: "{}", signal: controller.signal,
      });
      if (!controller.signal.aborted) setResult(response);
    } catch (error) {
      if (!controller.signal.aborted) setError(errorMessage(error));
    } finally {
      if (!controller.signal.aborted) setTask(null);
      active.current = null;
    }
  }

  return <section className={card} aria-labelledby="jev-configuration">
    <div>
      <h2 id="jev-configuration" className="text-lg font-semibold">Jev advisory</h2>
      <p className="mt-1 text-sm text-muted">Saved settings apply to future merchant scenarios in Mock Checkout and Simulation Lab. The diagnostic runs separately with synthetic data. Standalone Booking Assessment uses its existing evaluator.</p>
    </div>
    {loading && <p role="status" className="text-sm text-muted">Loading Jev configuration…</p>}
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
    {notice && <p role="status" className="text-sm text-emerald-800">{notice}</p>}
    {saved && form && <form className="space-y-4" onSubmit={submit}>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="min-w-0 text-sm font-semibold">Provider
          <select className="control mt-1" value={form.provider} disabled={task !== null || loading}
            onChange={event => setForm({ ...form, provider: event.currentTarget.value as Config["provider"] })}>
            <option value="mock">Mock</option><option value="openrouter">OpenRouter Decisions</option>
          </select>
        </label>
        <label className="min-w-0 text-sm font-semibold">Primary model
          <select className="control mt-1" value={form.model} disabled={task !== null || loading} onChange={event => {
            const model = event.currentTarget.value as Model;
            setForm({ ...form, model, fallbackModel: model === form.fallbackModel ? "" : form.fallbackModel });
          }}>{models.map(model => <option key={model}>{model}</option>)}</select>
        </label>
        <label className="min-w-0 text-sm font-semibold">Fallback model
          <select className="control mt-1" value={form.fallbackModel} disabled={task !== null || loading}
            onChange={event => setForm({ ...form, fallbackModel: event.currentTarget.value as Config["fallbackModel"] })}>
            <option value="">None</option>
            {models.filter(model => model !== form.model).map(model => <option key={model}>{model}</option>)}
          </select>
        </label>
        <label className="min-w-0 text-sm font-semibold">Rubric
          <select className="control mt-1" value={form.rubric} disabled={task !== null || loading}
            onChange={event => setForm({ ...form, rubric: event.currentTarget.value as Config["rubric"] })}>
            {Object.entries(rubrics).map(([key, title]) => <option key={key} value={key}>{title}</option>)}
          </select>
        </label>
        <label className="min-w-0 text-sm font-semibold">Total timeout (ms)
          <input type="number" inputMode="numeric" className="control mt-1" min={250} max={10000} required
            value={Number.isNaN(form.timeoutMs) ? "" : form.timeoutMs} disabled={task !== null || loading}
            onChange={event => setForm({ ...form, timeoutMs: event.currentTarget.valueAsNumber })} />
        </label>
      </div>
      <p className="text-xs text-muted">Version {saved.version} · {saved.configured ? "Provider configured" : "Provider configuration incomplete"}</p>
      <div className="flex flex-wrap gap-2">
        <button type="submit" className={primaryButton} disabled={task !== null || loading || !changed}>
          {task === "save" ? "Saving…" : "Save configuration"}
        </button>
        <button type="button" className={outlineButton} disabled={task !== null || loading || changed} onClick={() => void test()}>
          {task === "test" ? "Testing…" : "Test saved configuration"}
        </button>
        <button type="button" className={outlineButton} disabled={task !== null || loading} onClick={() => setRevision(value => value + 1)}>Reload</button>
      </div>
    </form>}
    {!saved && error && <button type="button" className={outlineButton} onClick={() => setRevision(value => value + 1)}>Retry</button>}
    {result && <div role="status" className="space-y-1 rounded-lg bg-paper p-3 text-sm">
      <p className="font-semibold">Jev diagnostic · {result.status}</p>
      <p className="text-muted wrap-anywhere">{result.model} · {result.rubric}{result.fallbackUsed && " · Fallback used"}</p>
      {result.decision && <p className="text-muted">Decision {result.decision}</p>}
      {result.message && <p className="text-muted">{result.message}</p>}
    </div>}
  </section>;
}
