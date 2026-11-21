"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { isMerchantSetup, merchantSetupStorageKey } from "../merchant-setup/contracts";
import type { MerchantSetup } from "../merchant-setup/contracts";
import { AssessmentResult, featureLabels } from "./assessment-result";
import { bookingRequest, submitBookingAssessment } from "./booking-api";
import type { Assessment, BookingAttempt, JevMode, Proposal } from "./booking-api";

const primaryButton = "rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50";
const secondaryButton = "rounded-lg border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold hover:bg-slate-50 disabled:opacity-50";
const card = "min-w-0 rounded-xl border border-slate-200 bg-white p-5 shadow-xs sm:p-6";
type Operation = "proposal" | "approve" | "assessment";

export function BookingAssessmentDemo() {
  const [setup, setSetup] = useState<MerchantSetup | null>(null);
  const [setupLoaded, setSetupLoaded] = useState(false);
  const [token, setToken] = useState("");
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [profileVersion, setProfileVersion] = useState("");
  const [amount, setAmount] = useState("");
  const [mode, setMode] = useState<JevMode>("mock");
  const [pending, setPending] = useState<BookingAttempt | null>(null);
  const [assessment, setAssessment] = useState<Assessment | null>(null);
  const [busy, setBusy] = useState<Operation | null>(null);
  const [error, setError] = useState<{ operation: Operation; message: string } | null>(null);
  const [amountError, setAmountError] = useState("");
  const bearer = token.trim();

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(merchantSetupStorageKey);
      const saved: unknown = raw ? JSON.parse(raw) : null;
      if (isMerchantSetup(saved)) setSetup(saved);
    } catch {
      // The empty state links back to the form where users can save a valid setup.
    } finally {
      setSetupLoaded(true);
    }
  }, []);

  function changeToken(value: string) {
    setToken(value);
    if (value.trim() === bearer) return;
    setProposal(null);
    setProfileVersion("");
    setPending(null);
    setAssessment(null);
    setError(null);
  }

  async function run(operation: Operation, execute: () => Promise<void>) {
    setBusy(operation);
    setError(null);
    try {
      await execute();
    } catch (cause) {
      setError({ operation, message: cause instanceof Error ? cause.message : "The booking API is unavailable." });
    } finally {
      setBusy(null);
    }
  }

  function generateProposal() {
    if (!setup) return;
    void run("proposal", async () => {
      setProposal(await bookingRequest<Proposal>("/profiles/proposal", bearer, setup));
    });
  }

  function approveProfile() {
    if (!proposal) return;
    void run("approve", async () => {
      const approved = await bookingRequest<{ profileVersion: string }>("/profiles/approve", bearer, proposal);
      setProfileVersion(approved.profileVersion);
    });
  }

  function assessBooking(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const entered = amount.trim();
    const amountMinor = entered === "" ? undefined : Number(entered);
    if (!pending && amountMinor !== undefined && (!/^\d+$/.test(entered) || !Number.isSafeInteger(amountMinor))) {
      setAmountError("Enter a whole number from 0 to 9,007,199,254,740,991 or leave it empty.");
      return;
    }

    const attempt: BookingAttempt = pending ?? {
      event: { eventId: crypto.randomUUID(), schemaVersion: 1, stage: "booking", occurredAt: new Date().toISOString(), amountMinor },
      jevMode: mode,
    };
    setPending(attempt);
    setAssessment(null);
    void run("assessment", async () => {
      const { assessment } = await submitBookingAssessment(bearer, attempt);
      setAssessment(assessment);
      setPending(null);
    });
  }

  return <div className="space-y-5">
    <section className={card} aria-labelledby="booking-profile-heading">
      <h2 id="booking-profile-heading" className="text-lg font-semibold">Scoring profile</h2>
      {setup && <p className="mt-2 text-sm text-slate-600">
        {setup.businessModel === "ota" ? "Online travel agency (OTA)" : "E-commerce"} · Booking
        <Link href="/settings/merchant-setup" className="ml-3 font-medium text-accent underline underline-offset-2">Edit merchant setup</Link>
      </p>}
      {setupLoaded && !setup && <p className="mt-2 text-sm text-slate-600">
        <Link href="/settings/merchant-setup" className="font-medium text-accent underline underline-offset-2">Save your merchant setup</Link> to generate a proposal.
      </p>}
      <label htmlFor="booking-demo-token" className="mt-5 block text-sm font-semibold">Demo merchant token</label>
      <input id="booking-demo-token" className="control mt-2 max-w-md" autoComplete="off" type="password" value={token}
        disabled={busy !== null} aria-describedby="booking-token-help" onChange={event => changeToken(event.currentTarget.value)} />
      <p id="booking-token-help" className="mt-2 text-xs text-slate-600">Use the bearer token configured for your local demo merchant. This page keeps it in memory.</p>
      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button type="button" className={proposal ? secondaryButton : primaryButton} onClick={generateProposal}
          disabled={!setup || !bearer || busy !== null || pending !== null}>
          {busy === "proposal" ? "Generating…" : "Generate proposal"}
        </button>
      </div>
      {proposal && <div className="mt-4 rounded-lg border border-slate-200 bg-slate-50 p-4">
        <p className="text-sm font-semibold">Proposed booking signals</p>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-slate-600">
          {proposal.candidate.signals.map(signal => <li key={signal}>{featureLabels[signal] ?? signal}</li>)}
        </ul>
        <button type="button" className={`${primaryButton} mt-4`} onClick={approveProfile} disabled={busy !== null || pending !== null}>
          {busy === "approve" ? "Approving…" : "Approve profile"}
        </button>
      </div>}
      {profileVersion && <p role="status" className="mt-3 text-sm text-emerald-800 break-all">Active profile <span className="font-mono">{profileVersion}</span></p>}
      {error && error.operation !== "assessment" && <p role="alert" className="mt-3 text-sm text-red-800">{error.message}</p>}
    </section>

    <section className={card} aria-labelledby="synthetic-booking-heading">
      <h2 id="synthetic-booking-heading" className="text-lg font-semibold">Synthetic booking</h2>
      {!profileVersion && <p className="mt-2 text-sm text-slate-600">Approve a profile to run an assessment.</p>}
      <form className="mt-4 space-y-4" onSubmit={assessBooking} noValidate>
        <fieldset disabled={!profileVersion || busy !== null || pending !== null} className="grid gap-4 sm:grid-cols-2">
          <legend className="sr-only">Booking inputs</legend>
          <div>
            <label htmlFor="booking-amount" className="block text-sm font-semibold">Booking amount</label>
            <input id="booking-amount" className="control mt-2 disabled:bg-slate-50" inputMode="numeric" value={amount} placeholder="Optional"
              aria-invalid={!!amountError} aria-describedby={amountError ? "booking-amount-error" : "booking-amount-help"} onChange={event => {
                setAmount(event.currentTarget.value);
                setAmountError("");
              }} />
            {amountError
              ? <p id="booking-amount-error" role="alert" className="mt-2 text-xs text-red-800">{amountError}</p>
              : <p id="booking-amount-help" className="mt-2 text-xs text-slate-600">Enter minor currency units, such as cents. Leave blank to omit.</p>}
          </div>
          <div>
            <label htmlFor="booking-jev-mode" className="block text-sm font-semibold">Jev response</label>
            <select id="booking-jev-mode" className="control mt-2 disabled:bg-slate-50" value={mode} onChange={event => setMode(event.currentTarget.value === "timeout" ? "timeout" : "mock")}>
              <option value="mock">Mock response</option>
              <option value="timeout">Simulate timeout</option>
            </select>
          </div>
        </fieldset>
        {error?.operation === "assessment" && <p role="alert" className="text-sm text-red-800">{error.message}</p>}
        <div className="flex flex-wrap items-center gap-3">
          <button className={primaryButton} disabled={!profileVersion || busy !== null} type="submit">
            {busy === "assessment" ? "Assessing…" : pending ? "Retry assessment" : "Run assessment"}
          </button>
          {pending && busy === null && <button className={secondaryButton} type="button" onClick={() => {
            setError(null);
            setPending(null);
          }}>Start a new booking</button>}
        </div>
      </form>
    </section>

    {assessment && <AssessmentResult assessment={assessment} />}
  </div>;
}
