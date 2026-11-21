"use client";

import { Hotel, Package, Plane, ShoppingBag, TabletSmartphone } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import {
  DemoScenarioPicker, DemoScenarioResult, invalidDemoAmount, readDemoAmount, useDemoScenario,
} from "../../demo-scenario-runner";
import type { Product } from "../../types";
import { AssessmentResult } from "../booking-assessment/assessment-result";
import { bookingRequest, submitBookingAssessment } from "../booking-assessment/booking-api";
import type { Assessment, BookingAttempt, BookingReceipt, JevMode, Proposal, SDKStatus } from "../booking-assessment/booking-api";
import type { BusinessModel, MerchantSetup } from "../merchant-setup/contracts";

const products = {
  flight_ticket: { label: "Flight ticket", name: "Bangkok to Singapore", detail: "One-way economy fare", icon: Plane, model: "ota", price: 18900 },
  hotel_room: { label: "Hotel room", name: "Singapore city stay", detail: "One night in a standard room", icon: Hotel, model: "ota", price: 14500 },
  physical: { label: "Physical goods", name: "Wireless headphones", detail: "One pair with standard delivery", icon: Package, model: "ecommerce", price: 7900 },
  digital: { label: "Digital goods", name: "Digital gift card", detail: "One gift card with instant delivery", icon: TabletSmartphone, model: "ecommerce", price: 5000 },
} satisfies Record<Product, { label: string; name: string; detail: string; icon: typeof Plane; model: BusinessModel; price: number }>;

const currency = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" });
const panel = "min-w-0 rounded-xl border border-slate-200 bg-white p-5 shadow-xs sm:p-6";
const primaryButton = "inline-flex items-center justify-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-50";
const secondaryButton = "rounded-lg border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold hover:bg-slate-50 disabled:opacity-50";

type CheckoutAttempt = BookingAttempt & { event: BookingAttempt["event"] & { product: Product } };
type CheckoutResult = { assessment: Assessment; booking: BookingReceipt; product: Product };

function displayAmount(amountMinor: number | undefined) {
  if (amountMinor === undefined) return "Omitted";
  if (Number.isNaN(amountMinor)) return "Invalid amount";

  const amount = BigInt(amountMinor);
  const centsPerDollar = BigInt(100);
  const cents = String(amount % centsPerDollar).padStart(2, "0");
  return currency.formatToParts(amount / centsPerDollar).map(part => part.type === "fraction" ? cents : part.value).join("");
}

export function MockCheckout() {
  const scenarioFlow = useDemoScenario();
  const [product, setProduct] = useState<Product>("flight_ticket");
  const [amount, setAmount] = useState(String(products.flight_ticket.price));
  const [sdkStatus, setSDKStatus] = useState<SDKStatus>("observed");
  const [mode, setMode] = useState<JevMode>("mock");
  const [token, setToken] = useState("");
  const [profileVersion, setProfileVersion] = useState("");
  const [busy, setBusy] = useState<"approval" | "checkout" | null>(null);
  const [error, setError] = useState("");
  const [amountError, setAmountError] = useState("");
  const [pending, setPending] = useState<CheckoutAttempt | null>(null);
  const [result, setResult] = useState<CheckoutResult | null>(null);
  const resultRef = useRef<HTMLDivElement>(null);

  const item = products[product];
  const amountMinor = readDemoAmount(amount);
  const bearer = token.trim();
  const merchantCheckout = Boolean(scenarioFlow.directory || scenarioFlow.connecting || scenarioFlow.requestedSelection);
  const checkoutLocked = merchantCheckout ? scenarioFlow.pending !== null : busy !== null || pending !== null;

  useEffect(() => {
    if (result || scenarioFlow.result) resultRef.current?.focus();
  }, [result, scenarioFlow.result]);

  function selectProduct(key: Product) {
    setProduct(key);
    setAmount(String(products[key].price));
    setAmountError("");
  }

  function changeToken(value: string) {
    setToken(value);
    if (value.trim() === bearer) return;
    setPending(null);
    setResult(null);
    setProfileVersion("");
    setError("");
  }

  async function approveProfile() {
    if (!bearer || busy) return;
    setBusy("approval");
    setError("");
    const setup: MerchantSetup = {
      schemaVersion: 1,
      businessModel: item.model,
      stage: "booking",
      availableSignals: ["occurredAt", "amountMinor"],
    };
    try {
      const proposal = await bookingRequest<Proposal>("/profiles/proposal", bearer, setup);
      const profile = await bookingRequest<{ profileVersion: string }>("/profiles/approve", bearer, proposal);
      setProfileVersion(profile.profileVersion);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The booking API is unavailable.");
    } finally {
      setBusy(null);
    }
  }

  async function checkout(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (Number.isNaN(amountMinor)) {
      setAmountError(invalidDemoAmount);
      return;
    }
    if (merchantCheckout) {
      await scenarioFlow.submit({ source: "checkout", amountMinor, product, simulateTimeout: mode === "timeout" });
      return;
    }
    if (!bearer || busy) return;
    const attempt: CheckoutAttempt = pending ?? {
      event: {
        eventId: crypto.randomUUID(),
        schemaVersion: 1,
        stage: "booking",
        occurredAt: new Date().toISOString(),
        amountMinor,
        product,
        sdkStatus,
      },
      jevMode: mode,
    };
    setPending(attempt);
    setBusy("checkout");
    setError("");
    setResult(null);
    try {
      const { assessment, booking } = await submitBookingAssessment(bearer, attempt);
      setResult({ assessment, booking, product: booking.product ?? attempt.event.product });
      setPending(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The booking API is unavailable.");
    } finally {
      setBusy(null);
    }
  }

  return <div className="space-y-5">
    <form className="grid items-start gap-5 lg:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]" onSubmit={checkout} noValidate>
      <div className="space-y-5">
        <div className={panel}>
          <fieldset disabled={checkoutLocked}>
            <legend className="text-lg font-semibold">Choose a product</legend>
            <div className="mt-4 grid grid-cols-2 gap-2.5">
              {(Object.keys(products) as Product[]).map(key => {
                const { label, icon: Icon, model } = products[key];
                return <label key={key} className="flex min-w-0 cursor-pointer flex-col items-start gap-3 rounded-lg border border-slate-200 p-3 hover:bg-slate-50 has-checked:border-indigo-500 has-checked:bg-indigo-50 has-focus-visible:ring-2 has-focus-visible:ring-indigo-500 has-disabled:cursor-default sm:p-4">
                  <input className="sr-only" type="radio" name="checkout-product" value={key} checked={product === key} onChange={() => selectProduct(key)} />
                  <Icon className="size-5 text-indigo-700" aria-hidden="true" />
                  <span className="min-w-0 text-sm font-semibold">{label}<span className="mt-0.5 block text-xs font-normal text-slate-600">{model === "ota" ? "Travel" : "E-commerce"}</span></span>
                </label>;
              })}
            </div>
          </fieldset>
        </div>

        <div className={panel}>
          <fieldset disabled={checkoutLocked}>
            <legend className="text-lg font-semibold">Booking signals</legend>
            <label htmlFor="checkout-amount" className="mt-4 block text-sm font-semibold">Booking amount</label>
            <input id="checkout-amount" className="control mt-2" inputMode="numeric" value={amount} placeholder="Optional"
              aria-invalid={!!amountError} aria-describedby="checkout-amount-help" onChange={event => {
                setAmount(event.currentTarget.value);
                setAmountError("");
              }} />
            <p id="checkout-amount-help" role={amountError ? "alert" : undefined} className={`mt-2 text-xs ${amountError ? "text-red-800" : "text-slate-600"}`}>
              {amountError || "Enter cents. Leave blank to omit the amount."}
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              {!merchantCheckout && <label className="block text-sm font-semibold">SDK signal
                <select className="control mt-2" value={sdkStatus} onChange={event => setSDKStatus(event.currentTarget.value === "unavailable" ? "unavailable" : "observed")}>
                  <option value="observed">Observed</option>
                  <option value="unavailable">Unavailable</option>
                </select>
              </label>}
              <label className="block text-sm font-semibold">Jev response
                <select className="control mt-2" value={mode} onChange={event => setMode(event.currentTarget.value === "timeout" ? "timeout" : "mock")}>
                  <option value="mock">Mock response</option>
                  <option value="timeout">Simulate timeout</option>
                </select>
              </label>
            </div>
          </fieldset>
        </div>
      </div>

      <section className={panel} aria-labelledby="checkout-summary-heading">
        <h2 id="checkout-summary-heading" className="text-lg font-semibold">Order summary</h2>
        <div className="mt-4 flex items-center gap-3 rounded-lg bg-slate-50 p-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-md bg-indigo-100 text-indigo-800"><item.icon className="size-5" aria-hidden="true" /></span>
          <div className="min-w-0">
            <div className="text-sm font-semibold">{item.name}</div>
            <div className="mt-0.5 text-xs text-slate-600">{item.detail}</div>
          </div>
        </div>
        <dl className="mt-4 flex flex-wrap items-baseline justify-between gap-1 border-b border-slate-200 pb-4 text-sm">
          <dt className="text-slate-600">Booking amount</dt>
          <dd className="break-all font-semibold">{displayAmount(amountMinor)}</dd>
        </dl>
        <fieldset className="mt-4 border-b border-slate-200 pb-4" disabled={busy !== null}>
          <legend className="mb-2 text-sm font-semibold">Merchant scenario</legend>
          <DemoScenarioPicker flow={scenarioFlow} allowLegacy />
        </fieldset>
        {!merchantCheckout && <>
        <label htmlFor="checkout-demo-token" className="mt-4 block text-sm font-semibold">Demo merchant token</label>
        <input id="checkout-demo-token" className="control mt-2" autoComplete="off" type="password" value={token}
          disabled={busy !== null} aria-describedby="checkout-token-help" onChange={event => changeToken(event.currentTarget.value)} />
        <p id="checkout-token-help" className="mt-2 text-xs text-slate-600">Use your configured local token. This page keeps it in memory.</p>
        <button type="button" className={`${secondaryButton} mt-3 w-full`} disabled={!bearer || busy !== null} onClick={() => void approveProfile()}>
          {busy === "approval" ? "Approving…" : "Approve demo profile"}
        </button>
        {profileVersion && <p role="status" className="mt-2 text-xs font-medium text-emerald-800">Demo profile approved.</p>}
        </>}
        <div className="mt-4 space-y-2 border-t border-slate-200 pt-4">
          {(merchantCheckout ? scenarioFlow.submissionError : error) && <p role="alert" className="rounded-lg bg-red-50 p-3 text-xs leading-relaxed text-red-800">{merchantCheckout ? scenarioFlow.submissionError : error}</p>}
          <button type="submit" className={`${primaryButton} w-full`} disabled={merchantCheckout
            ? !scenarioFlow.scenario || scenarioFlow.submitting
            : !bearer || busy !== null}>
            <ShoppingBag className="size-4" aria-hidden="true" />
            {busy === "checkout" || scenarioFlow.submitting ? "Assessing…" : (merchantCheckout ? scenarioFlow.pending : pending) ? "Retry checkout" : "Submit mock checkout"}
          </button>
          {merchantCheckout && scenarioFlow.pending && !scenarioFlow.submitting && <button type="button" className={`${secondaryButton} w-full`} onClick={scenarioFlow.clearAttempt}>Start a new checkout</button>}
          {!merchantCheckout && pending && busy === null && <button type="button" className={`${secondaryButton} w-full`} onClick={() => {
            setPending(null);
            setError("");
          }}>Start a new checkout</button>}
        </div>
        <p className="mt-3 text-xs text-slate-600">Synthetic order. No payment will be collected.</p>
      </section>
    </form>
    {scenarioFlow.result && scenarioFlow.merchant && <div ref={resultRef} tabIndex={-1} className="scroll-mt-4 focus:outline-none" aria-label="Checkout assessment">
      <DemoScenarioResult transaction={scenarioFlow.result} merchantName={scenarioFlow.merchant.name} />
    </div>}
    {!merchantCheckout && result && <div ref={resultRef} tabIndex={-1} className="scroll-mt-4 focus:outline-none" aria-label="Checkout assessment">
      <AssessmentResult assessment={result.assessment} booking={result.booking} context={`${products[result.product].label} · ${products[result.product].name}`} />
    </div>}
  </div>;
}
