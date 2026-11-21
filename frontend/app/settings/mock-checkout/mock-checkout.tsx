"use client";

import { Hotel, Package, Plane, ShoppingBag, TabletSmartphone } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import {
  DemoScenarioAccountAgeField, DemoScenarioCustomerIdField, DemoScenarioCustomSignals, DemoScenarioEmailField, DemoScenarioPicker,
  DemoScenarioPolicyContext, DemoScenarioResult,
  invalidDemoAmount, readDemoAmount, useDemoScenario,
} from "../../demo-scenario-runner";
import type { DemoCustomSignalValues } from "../../demo-scenario-runner";
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

const standaloneProducts = Object.keys(products) as Product[];
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
  const merchantFlow = useDemoScenario("checkout");
  const [checkoutMode, setCheckoutMode] = useState<"merchant" | "standalone">("merchant");
  const merchantCheckout = merchantFlow.merchantRestricted || checkoutMode === "merchant";

  return <div className="space-y-5">
    {!merchantFlow.merchantRestricted && <div role="group" aria-label="Checkout mode" className="flex flex-wrap gap-2">
      <button type="button" aria-pressed={merchantCheckout} className={merchantCheckout ? primaryButton : secondaryButton}
        onClick={() => setCheckoutMode("merchant")}>Merchant rules</button>
      <button type="button" aria-pressed={!merchantCheckout} className={!merchantCheckout ? primaryButton : secondaryButton}
        disabled={merchantFlow.submitting} onClick={() => {
          if (merchantCheckout) merchantFlow.clearAttempt();
          setCheckoutMode("standalone");
        }}>Standalone checkout</button>
    </div>}
    {merchantCheckout ? <>
      <section className={`${panel} space-y-3`} aria-labelledby="checkout-merchant-heading">
        <h2 id="checkout-merchant-heading" className="text-lg font-semibold">Choose a merchant</h2>
        <DemoScenarioPicker flow={merchantFlow} />
      </section>
      {merchantFlow.admin && merchantFlow.merchantId && <CheckoutForm key={merchantFlow.merchantId} flow={merchantFlow} />}
    </> : <>
      <p className="text-sm text-slate-600">Run a standalone assessment with a demo booking profile.</p>
      <CheckoutForm />
    </>}
  </div>;
}

function CheckoutForm({ flow }: { flow?: ReturnType<typeof useDemoScenario> }) {
  const [standaloneProduct, setStandaloneProduct] = useState<Product>("flight_ticket");
  const [enteredAmount, setEnteredAmount] = useState<string | null>(null);
  const [accountAgeDays, setAccountAgeDays] = useState("");
  const [customSignals, setCustomSignals] = useState<DemoCustomSignalValues>({});
  const [email, setEmail] = useState("");
  const [customerId, setCustomerId] = useState("");
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

  const product = flow ? flow.product : standaloneProduct;
  const item = product && product !== "payment_gateway" ? products[product] : undefined;
  const availableProducts = flow ? flow.products.filter(value => value !== "payment_gateway") : standaloneProducts;
  const amount = enteredAmount ?? (item ? String(item.price) : "");
  const amountMinor = readDemoAmount(amount);
  const bearer = token.trim();
  const checkoutLocked = flow ? !flow.ready || flow.submitting || flow.pending !== null : busy !== null || pending !== null;
  const merchantResult = flow?.result;
  const pendingAttempt = flow ? flow.pending : pending;

  useEffect(() => {
    if (result || merchantResult) resultRef.current?.focus();
  }, [result, merchantResult]);

  function selectProduct(key: Product) {
    if (flow) flow.selectProduct(key);
    else {
      setStandaloneProduct(key);
      setResult(null);
    }
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
    if (!bearer || busy || !item) return;
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
    if (!product || product === "payment_gateway") return;
    if (Number.isNaN(amountMinor)) {
      setAmountError(invalidDemoAmount);
      return;
    }
    if (flow) {
      await flow.submit({
        source: "checkout", amountMinor, product, customerEmail: email, customerId, accountAgeDays, customSignals,
        simulateTimeout: mode === "timeout",
      });
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

  if (!item) return flow ? <DemoScenarioPolicyContext flow={flow} /> : null;

  return <>
    <form className="grid items-start gap-5 lg:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]" onSubmit={checkout} noValidate>
      <div className="space-y-5">
        <div className={panel}>
          <fieldset disabled={checkoutLocked}>
            <legend className="text-lg font-semibold">Choose a product</legend>
            <div className="mt-4 grid grid-cols-2 gap-2.5">
              {availableProducts.map(key => {
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

        {flow && <DemoScenarioPolicyContext flow={flow} />}

        <div className={panel}>
          <fieldset disabled={checkoutLocked}>
            <legend className="text-lg font-semibold">Transaction signals</legend>
            <label htmlFor="checkout-amount" className="mt-4 block text-sm font-semibold">Booking amount in minor units</label>
            <input id="checkout-amount" className="control mt-2" inputMode="numeric" value={amount} placeholder="Optional"
              aria-invalid={!!amountError} aria-describedby="checkout-amount-help" onChange={event => {
                setEnteredAmount(event.currentTarget.value);
                setAmountError("");
                if (flow) flow.clearAttempt();
                else setResult(null);
              }} />
            <p id="checkout-amount-help" role={amountError ? "alert" : undefined} className={`mt-2 text-xs ${amountError ? "text-red-800" : "text-slate-600"}`}>
              {amountError || "Enter minor units. Leave blank to omit the amount."}
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              {flow && <DemoScenarioEmailField value={email} onChange={value => {
                setEmail(value);
                flow.clearAttempt();
              }} />}
              {flow && <DemoScenarioCustomerIdField value={customerId} onChange={value => {
                setCustomerId(value);
                flow.clearAttempt();
              }} />}
              {flow && <DemoScenarioAccountAgeField value={accountAgeDays} onChange={value => {
                setAccountAgeDays(value);
                flow.clearAttempt();
              }} />}
              {!flow && <label className="block text-sm font-semibold">SDK signal
                <select className="control mt-2" value={sdkStatus} onChange={event => setSDKStatus(event.currentTarget.value === "unavailable" ? "unavailable" : "observed")}>
                  <option value="observed">Observed</option>
                  <option value="unavailable">Unavailable</option>
                </select>
              </label>}
              <label className="block text-sm font-semibold">Jev response
                <select className="control mt-2" value={mode} onChange={event => {
                  setMode(event.currentTarget.value === "timeout" ? "timeout" : "mock");
                  if (flow) flow.clearAttempt();
                  else setResult(null);
                }}>
                  <option value="mock">{flow ? "Use configured Jev" : "Mock response"}</option>
                  <option value="timeout">Simulate timeout</option>
                </select>
              </label>
            </div>
            {flow && <DemoScenarioCustomSignals flow={flow} values={customSignals} onChange={values => {
              setCustomSignals(values);
              flow.clearAttempt();
            }} />}
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
        {!flow && <>
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
          {(flow ? flow.submissionError : error) && <p role="alert" className="rounded-lg bg-red-50 p-3 text-xs leading-relaxed text-red-800">{flow ? flow.submissionError : error}</p>}
          <button type="submit" className={`${primaryButton} w-full`} disabled={flow
            ? !flow.ready || flow.submitting
            : !bearer || busy !== null}>
            <ShoppingBag className="size-4" aria-hidden="true" />
            {busy === "checkout" || flow?.submitting ? "Assessing…" : pendingAttempt ? "Retry checkout" : "Submit mock checkout"}
          </button>
          {flow && pendingAttempt && !flow.submitting && <button type="button" className={`${secondaryButton} w-full`} onClick={flow.clearAttempt}>Start a new checkout</button>}
          {!flow && pendingAttempt && busy === null && <button type="button" className={`${secondaryButton} w-full`} onClick={() => {
            setPending(null);
            setError("");
          }}>Start a new checkout</button>}
        </div>
        <p className="mt-3 text-xs text-slate-600">Synthetic order. No payment will be collected.</p>
      </section>
    </form>
    {merchantResult && flow?.merchant && <div ref={resultRef} tabIndex={-1} className="scroll-mt-4 focus:outline-none" aria-label="Checkout assessment">
      <DemoScenarioResult transaction={merchantResult} merchantName={flow.merchant.name} />
    </div>}
    {!flow && result && <div ref={resultRef} tabIndex={-1} className="scroll-mt-4 focus:outline-none" aria-label="Checkout assessment">
      <AssessmentResult assessment={result.assessment} booking={result.booking} context={`${products[result.product].label} · ${products[result.product].name}`} />
    </div>}
  </>;
}
