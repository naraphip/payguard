"use client";

import type { Actor, Filters } from "./contracts";
import { holdStates, outcomes, products } from "./contracts";

const filterKeys = { product: products, outcome: outcomes, hold: holdStates };
export const metricKeys = ["bookings", "review", "latency", "freshness"] as const;
export type DashboardMetric = (typeof metricKeys)[number];
export type DashboardPreferences = { filters: Filters; metrics: DashboardMetric[] };
export const defaultPreferences: DashboardPreferences = {
  filters: { product: "", outcome: "", hold: "" },
  metrics: [...metricKeys],
};

function preferenceKey(actor: Actor) {
  return `payguard:operations:v1:${encodeURIComponent(actor.merchantId)}:${encodeURIComponent(actor.actorId)}`;
}

export function readPreferences(actor: Actor): DashboardPreferences {
  try {
    const value = window.localStorage.getItem(preferenceKey(actor));
    if (!value) return defaultPreferences;
    const saved = JSON.parse(value) as Partial<DashboardPreferences>;
    const filters = { ...defaultPreferences.filters };
    for (const key of Object.keys(filterKeys) as (keyof Filters)[]) {
      const selected = saved.filters?.[key];
      if (typeof selected === "string" && Object.hasOwn(filterKeys[key], selected)) filters[key] = selected;
    }
    return {
      filters,
      metrics: Array.isArray(saved.metrics) ? metricKeys.filter(key => saved.metrics?.includes(key)) : [...metricKeys],
    };
  } catch {
    return defaultPreferences;
  }
}

const metricLabels: Record<DashboardMetric, string> = {
  bookings: "Bookings",
  review: "Manual reviews",
  latency: "Assessment latency",
  freshness: "Latest booking",
};

export function DashboardSettings({ actor, preferences, onChange, filters }: {
  actor: Actor;
  preferences: DashboardPreferences;
  onChange: (preferences: DashboardPreferences, saved: boolean) => void;
  filters: Filters;
}) {
  function apply(next: DashboardPreferences) {
    let saved = true;
    try {
      window.localStorage.setItem(preferenceKey(actor), JSON.stringify(next));
    } catch {
      saved = false;
    }
    onChange(next, saved);
  }

  return <details className="rounded-xl border border-slate-200 bg-white px-4 py-3 text-sm shadow-xs">
    <summary className="cursor-pointer font-semibold">Dashboard settings</summary>
    <div className="mt-3 space-y-3">
      <fieldset>
        <legend className="text-xs font-semibold text-slate-600">Metrics shown</legend>
        <div className="mt-2 flex flex-wrap gap-x-5 gap-y-2">
          {metricKeys.map(key => <label key={key} className="flex items-center gap-2">
            <input type="checkbox" checked={preferences.metrics.includes(key)} onChange={event => {
              const enabled = event.currentTarget.checked;
              apply({ ...preferences, metrics: metricKeys.filter(metric => metric === key ? enabled : preferences.metrics.includes(metric)) });
            }} />
            {metricLabels[key]}
          </label>)}
        </div>
      </fieldset>
      <div className="flex flex-wrap gap-2">
        <button type="button" className="rounded-lg border border-slate-300 px-3 py-2 text-xs font-semibold hover:bg-slate-50" onClick={() => apply({ ...preferences, filters })}>Use current filters by default</button>
        <button type="button" className="rounded-lg border border-slate-300 px-3 py-2 text-xs font-semibold hover:bg-slate-50" onClick={() => apply(defaultPreferences)}>Reset dashboard settings</button>
      </div>
      <p className="text-xs text-slate-600">Stored in this browser for this demo actor. Default filters apply when you connect.</p>
    </div>
  </details>;
}
