"use client";

import { ArrowRight } from "lucide-react";
import { findSharedSignals } from "./simulation-shared-signals";
import type { Decision, EntityKind } from "./types";

const signalLabels: Record<Exclude<EntityKind, "account">, string> = {
  device: "Device",
  ip: "IP address",
  token: "Payment token",
};

const visibleEventCount = 5;

export function Network({ selected, decisions, onSelectEvent }: {
  selected: Decision;
  decisions: Decision[];
  onSelectEvent: (id: string) => void;
}) {
  const sharedSignals = findSharedSignals(selected.event, decisions);

  function eventLinks(eventIds: string[], signalLabel: string) {
    return (
      <ul className="mt-2 flex flex-wrap gap-2">
        {eventIds.map(id => <li key={id}>
          <button
            type="button"
            aria-label={`Open event ${id} sharing ${signalLabel.toLowerCase()}`}
            aria-controls="decision-evidence"
            className="inline-flex min-h-9 items-center gap-1.5 rounded-md border border-line bg-white px-2 text-xs font-medium text-accent hover:bg-soft"
            onClick={() => onSelectEvent(id)}
          >
            <span className="wrap-anywhere">{id}</span><ArrowRight size={13} className="shrink-0" aria-hidden />
          </button>
        </li>)}
      </ul>
    );
  }

  return (
    <section className="mt-5 border-t border-line pt-4" aria-labelledby="shared-signals-heading" key={selected.event.id}>
      <h4 id="shared-signals-heading" className="text-sm font-semibold">Shared signals in this run</h4>
      {sharedSignals.length === 0 ? (
        <p className="mt-1 text-xs text-muted">No earlier events from other accounts share these signals.</p>
      ) : <>
        <p className="mt-1 text-xs text-muted">
          Each link opens the latest earlier event for a different account in this simulation. Ordinary activity can share these signals too.
        </p>
        <ul className="mt-3 grid gap-2">
          {sharedSignals.map(({ kind, eventIds }) => (
            <li className="rounded-lg border border-line p-3" key={kind}>
              <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-xs">
                <h5 className="font-semibold">{signalLabels[kind]}</h5>
                <span className="tabular-nums text-muted">{eventIds.length} other {eventIds.length === 1 ? "account" : "accounts"}</span>
              </div>
              {eventLinks(eventIds.slice(0, visibleEventCount), signalLabels[kind])}
              {eventIds.length > visibleEventCount && <details className="mt-2 text-xs">
                <summary className="w-fit cursor-pointer font-medium text-accent">
                  Show {eventIds.length - visibleEventCount} more for {signalLabels[kind].toLowerCase()}
                </summary>
                {eventLinks(eventIds.slice(visibleEventCount), signalLabels[kind])}
              </details>}
            </li>
          ))}
        </ul>
      </>}
    </section>
  );
}
