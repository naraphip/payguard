"use client";

import { useEffect, useRef, useState } from "react";
import { CreditCard, LoaderCircle, ShieldCheck } from "lucide-react";
import { thb } from "./storefront";
import type { Prices } from "./storefront";

export type CheckoutConfig = {
  enabled: boolean;
  publicKey?: string;
  currency: "THB";
  prices: Prices;
};

type Charge = {
  status: "successful" | "failed" | "pending" | "unknown";
  chargeId?: string;
  amount: number;
  currency: "THB";
  failureCode?: string;
};

type HostedCard = {
  configure(settings: { publicKey: string }): void;
  close?: () => void;
  open(settings: {
    amount: number;
    currency: "THB";
    defaultPaymentMethod: "credit_card";
    otherPaymentMethods: string;
    frameLabel: string;
    frameDescription: string;
    submitLabel: string;
    submitAuto: "no";
    onCreateTokenSuccess: (token: string) => void;
    onFormClosed: () => void;
  }): boolean | void;
};

declare global {
  interface Window {
    OmiseCard?: HostedCard;
  }
}

const statuses = {
  successful: "Test payment successful.",
  failed: "Test payment declined.",
  pending: "Test payment pending.",
  unknown: "Test payment outcome could not be confirmed.",
};

let hostedCardRequest: Promise<HostedCard> | undefined;

function loadHostedCard() {
  if (window.OmiseCard) return Promise.resolve(window.OmiseCard);
  hostedCardRequest ??= new Promise<HostedCard>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = "https://cdn.omise.co/omise.js";
    script.async = true;
    const timeout = window.setTimeout(fail, 10_000);

    function fail() {
      window.clearTimeout(timeout);
      script.remove();
      reject(new Error("Could not load the Omise form."));
    }

    script.onload = () => {
      window.clearTimeout(timeout);
      if (window.OmiseCard) resolve(window.OmiseCard);
      else fail();
    };
    script.onerror = fail;
    document.head.append(script);
  }).catch((error: unknown) => {
    hostedCardRequest = undefined;
    throw error;
  });

  return hostedCardRequest;
}

type PaymentAttempt = { runId: string; submitting: boolean };

export function TestPayment({ config, runId, amount, disabledReason, onBusyChange }: {
  config: CheckoutConfig | null;
  runId: string | null;
  amount: number;
  disabledReason: string;
  onBusyChange: (busy: boolean) => void;
}) {
  const [busy, setBusy] = useState<"loading" | "form" | "submitting" | null>(null);
  const [result, setResult] = useState<{ runId: string; message: string; charge?: Charge } | null>(null);
  const [error, setError] = useState("");
  const attempt = useRef<PaymentAttempt | null>(null);
  const outcome = result?.runId === runId ? result : null;
  const reason = outcome ? "Run a new simulation for another payment attempt." : disabledReason;

  useEffect(() => () => {
    const active = attempt.current;
    attempt.current = null;
    if (active) window.OmiseCard?.close?.();
  }, []);

  function finish(current: PaymentAttempt) {
    if (attempt.current !== current) return;
    attempt.current = null;
    setBusy(null);
    onBusyChange(false);
  }

  async function charge(current: PaymentAttempt, token: string) {
    if (attempt.current !== current || current.submitting) return;
    if (!token.startsWith("tokn_test_")) {
      setError("Omise did not return a test card token. Try opening the payment form again.");
      finish(current);
      return;
    }
    current.submitting = true;
    setBusy("submitting");

    try {
      const response = await fetch("/api/v1/simulation/checkout/charges", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ runId: current.runId, token }),
        signal: AbortSignal.timeout(25_000),
      });
      if (response.status === 409) {
        if (attempt.current === current) setResult({ runId: current.runId, message: "This saved run is no longer eligible for a test payment." });
        return;
      }
      if (!response.ok) throw new Error("Payment response unavailable.");
      const outcome = (await response.json()) as Charge;
      if (!(outcome.status in statuses)) throw new Error("Payment response invalid.");
      if (attempt.current === current) setResult({ runId: current.runId, message: statuses[outcome.status], charge: outcome });
    } catch {
      if (attempt.current === current) setResult({ runId: current.runId, message: statuses.unknown });
    } finally {
      finish(current);
    }
  }

  async function open() {
    const publicKey = config?.publicKey;
    if (!config?.enabled || !publicKey?.startsWith("pkey_test_") || !runId || reason || attempt.current) return;

    const current = { runId, submitting: false };
    attempt.current = current;
    setError("");
    setBusy("loading");
    onBusyChange(true);
    try {
      const hostedCard = await loadHostedCard();
      if (attempt.current !== current) return;
      hostedCard.configure({ publicKey });
      setBusy("form");
      const shown = hostedCard.open({
        amount,
        currency: "THB",
        defaultPaymentMethod: "credit_card",
        otherPaymentMethods: "",
        frameLabel: "PayGuard sandbox",
        frameDescription: "Use an Omise test card.",
        submitLabel: "Test payment",
        submitAuto: "no",
        onCreateTokenSuccess: token => { void charge(current, token); },
        onFormClosed: () => { if (!current.submitting) finish(current); },
      });
      if (shown === false) throw new Error("Omise form unavailable.");
    } catch {
      if (attempt.current !== current) return;
      setError("The Omise payment form could not open. Try again.");
      finish(current);
    }
  }

  return <div className="space-y-2">
    <button type="button" className="shop-secondary" disabled={Boolean(reason || busy)} onClick={open} aria-describedby="test-payment-hint">
      {busy ? <LoaderCircle size={16} className="animate-spin" aria-hidden="true" /> : <CreditCard size={17} aria-hidden="true" />}
      {busy === "loading" ? "Loading payment form…" : busy === "form" ? "Payment form open" : busy === "submitting" ? "Submitting payment…" : "Pay in test mode"}
    </button>
    <p id="test-payment-hint" className="flex items-start gap-1.5 text-xs leading-relaxed text-[#5f516d]">
      <ShieldCheck size={14} className="mt-[1px] shrink-0" aria-hidden="true" />
      {reason || "Use an Omise test card in the hosted form."}
    </p>
    {error && <p role="alert" className="rounded-lg bg-red-50 p-2 text-xs text-red-800">{error}</p>}
    {outcome && <div role="status" className="space-y-1 rounded-xl border border-[#e8ddef] bg-[#f8f3fb] p-3 text-xs text-[#39264b]">
      <p className="font-semibold">{outcome.message}</p>
      {outcome.charge && <p>{thb(outcome.charge.amount)}{outcome.charge.failureCode ? ` · ${outcome.charge.failureCode}` : ""}</p>}
      {outcome.charge?.chargeId && <p className="wrap-anywhere">Reference <code>{outcome.charge.chargeId}</code></p>}
    </div>}
  </div>;
}
