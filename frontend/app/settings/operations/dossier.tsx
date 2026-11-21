"use client";

import { Download, FileText } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { operationsRequest } from "./contracts";

type Dossier = { eventId: string; citations: { field: string; value: unknown; version: string }[] };
type DossierJob = { status: "PENDING" | "READY"; dossier?: Dossier };

export function BookingDossier({ eventId, token }: { eventId: string; token: string }) {
  const [job, setJob] = useState<DossierJob | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [polling, setPolling] = useState(false);
  const request = useRef<AbortController>(null);
  const path = `/${eventId}/dossier`;

  useEffect(() => () => request.current?.abort(), []);

  async function generate() {
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError("");
    try {
      const next = await operationsRequest<DossierJob>(path, token, { write: true, signal: controller.signal });
      if (!controller.signal.aborted) {
        setJob(next);
        setPolling(next.status === "PENDING");
      }
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "Could not start the dossier.");
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  useEffect(() => {
    if (!polling) return;
    const controller = new AbortController();
    let pendingChecks = 0;
    let timer: ReturnType<typeof setTimeout>;
    const check = async () => {
      try {
        const next = await operationsRequest<DossierJob>(path, token, { signal: controller.signal });
        if (controller.signal.aborted) return;
        setJob(next);
        if (next.status === "PENDING" && ++pendingChecks < 20) timer = setTimeout(check, 1000);
        else setPolling(false);
      } catch (cause) {
        if (!controller.signal.aborted) {
          setError(cause instanceof Error ? cause.message : "Could not check dossier status.");
          setPolling(false);
        }
      }
    };
    timer = setTimeout(check, 750);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [path, polling, token]);

  function download(dossier: Dossier) {
    const url = URL.createObjectURL(new Blob([JSON.stringify(dossier, null, 2) + "\n"], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = `booking-dossier-${dossier.eventId}.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  const dossier = job?.status === "READY" ? job.dossier : undefined;
  return <section aria-label="Evidence dossier" className="space-y-3 border-t border-slate-100 pt-4">
    <h3 className="text-sm font-semibold">Evidence dossier</h3>
    {dossier ? <>
      <ul className="divide-y divide-slate-100 rounded-lg border border-slate-200">
        {dossier.citations.map((citation, index) => <li key={`${citation.field}-${index}`} className="p-3 text-sm">
          <p className="font-semibold text-slate-800">{typeof citation.value === "string" ? citation.value : JSON.stringify(citation.value)}</p>
          <p className="mt-1 break-all text-xs text-slate-600">{citation.field} · {citation.version}</p>
        </li>)}
      </ul>
      <button type="button" className="inline-flex items-center gap-2 rounded-lg border border-slate-300 px-3.5 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50" onClick={() => download(dossier)}>
        <Download className="size-4" aria-hidden="true" /> Export masked JSON
      </button>
    </> : <>
      {job?.status === "PENDING" && <p role="status" className="text-sm text-slate-600">Dossier queued.</p>}
      {!polling && <button type="button" disabled={busy} className="inline-flex items-center gap-2 rounded-lg border border-slate-300 px-3.5 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50" onClick={() => {
        if (job?.status === "PENDING") {
          setError("");
          setPolling(true);
        } else void generate();
      }}>
        <FileText className="size-4" aria-hidden="true" />{busy ? "Queueing…" : job?.status === "PENDING" ? "Check status" : "Generate dossier"}
      </button>}
    </>}
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
  </section>;
}
