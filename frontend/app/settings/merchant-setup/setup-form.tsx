"use client";

import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Check, CircleAlert, Plane, ShoppingBag } from "lucide-react";
import { isBusinessModel, isRecord, merchantSetupStorageKey, validBookingSignals } from "./contracts";
import type { BusinessModel, MerchantSetup } from "./contracts";

const businessModels = [
  { id: "ota", label: "Online travel agency (OTA)", description: "Flights and hotel reservations", Icon: Plane },
  { id: "ecommerce", label: "E-commerce", description: "Physical and digital goods", Icon: ShoppingBag },
] as const;
type FieldErrors = { businessModel?: string; stage?: string; availableSignals?: string };

const businessModelError = "Choose a supported business model.";
const stageError = "Choose Booking as the journey stage.";

export function MerchantSetupForm() {
  const [businessModel, setBusinessModel] = useState<BusinessModel | "">("");
  const [stage, setStage] = useState("");
  const [includesAmount, setIncludesAmount] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [storageError, setStorageError] = useState("");
  const [notice, setNotice] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    try {
      const saved = window.localStorage.getItem(merchantSetupStorageKey);
      if (saved === null) return;

      const setup: unknown = JSON.parse(saved);
      if (!isRecord(setup) || setup.schemaVersion !== 1) {
        setStorageError("Saved setup has an unsupported format. Save a new setup to replace it.");
        return;
      }

      const restoredErrors: FieldErrors = {};
      if (isBusinessModel(setup.businessModel)) setBusinessModel(setup.businessModel);
      else restoredErrors.businessModel = businessModelError;
      if (setup.stage === "booking") setStage(setup.stage);
      else restoredErrors.stage = stageError;

      const signals: unknown[] = Array.isArray(setup.availableSignals) ? setup.availableSignals : [];
      setIncludesAmount(signals.includes("amountMinor"));
      if (!validBookingSignals(signals)) {
        restoredErrors.availableSignals = "Saved signals are invalid. Review the booking signals below and save again.";
      }
      setErrors(restoredErrors);
      if (Object.keys(restoredErrors).length === 0) setNotice("Saved setup loaded.");
    } catch {
      setStorageError("Could not read your saved setup. You can enter your choices and try saving again.");
    } finally {
      setReady(true);
    }
  }, []);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const selectedStage = new FormData(event.currentTarget).get("stage");
    setNotice("");
    if (!businessModel || selectedStage !== "booking") {
      setErrors(current => ({
        ...current,
        businessModel: businessModel ? undefined : businessModelError,
        stage: selectedStage === "booking" ? undefined : stageError,
      }));
      return;
    }

    const setup: MerchantSetup = { schemaVersion: 1, businessModel, stage: selectedStage, availableSignals: ["occurredAt"] };
    if (includesAmount) setup.availableSignals.push("amountMinor");
    try {
      window.localStorage.setItem(merchantSetupStorageKey, JSON.stringify(setup));
      setErrors({});
      setStorageError("");
      setNotice("Setup saved in this browser.");
    } catch {
      setStorageError("Could not save this setup. Check whether browser storage is available and try again.");
    }
  }

  return <form noValidate onSubmit={submit} className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xs">
    <fieldset disabled={!ready} className="min-w-0 divide-y divide-slate-200">
      <legend className="sr-only">Merchant setup details</legend>
      <div className="grid gap-6 p-5 sm:p-6">
        <fieldset aria-invalid={!!errors.businessModel} aria-describedby={errors.businessModel ? "business-model-error" : undefined}>
          <legend className="text-sm font-semibold">Business model</legend>
          <div className="mt-3 grid gap-2.5 sm:grid-cols-2">
            {businessModels.map(({ id, label, description, Icon }) => <label key={id} className="flex cursor-pointer items-start gap-3 rounded-lg border border-slate-200 p-3.5 hover:bg-slate-50 has-checked:border-indigo-300 has-checked:bg-indigo-50/60">
              <Icon className="mt-0.5 size-4 shrink-0 text-indigo-700" aria-hidden="true" />
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold">{label}</span>
                <span className="mt-1 block text-xs text-slate-600">{description}</span>
              </span>
              <input type="radio" name="businessModel" value={id} required checked={businessModel === id} className="mt-0.5 size-4 shrink-0 accent-accent" onChange={() => {
                setBusinessModel(id);
                setErrors(current => ({ ...current, businessModel: undefined }));
                setNotice("");
              }} />
            </label>)}
          </div>
          {errors.businessModel && <p id="business-model-error" role="alert" className="mt-2 text-sm text-red-800">{errors.businessModel}</p>}
        </fieldset>

        <div>
          <label htmlFor="merchant-journey-stage" className="block text-sm font-semibold">Journey stage</label>
          <select id="merchant-journey-stage" name="stage" required value={stage} aria-invalid={!!errors.stage} aria-describedby={errors.stage ? "journey-stage-error" : undefined} className="control mt-2 max-w-sm" onChange={event => {
            setStage(event.currentTarget.value);
            setErrors(current => ({ ...current, stage: undefined }));
            setNotice("");
          }}>
            <option value="">Choose a stage</option>
            <option value="booking">Booking</option>
          </select>
          {errors.stage && <p id="journey-stage-error" role="alert" className="mt-2 text-sm text-red-800">{errors.stage}</p>}
        </div>
      </div>

      <div className="p-5 sm:p-6">
        <fieldset aria-invalid={!!errors.availableSignals} aria-describedby={errors.availableSignals ? "available-signals-error" : undefined}>
          <legend className="text-sm font-semibold">Available signals</legend>
          {errors.availableSignals && <p id="available-signals-error" role="alert" className="mt-2 text-sm text-red-800">{errors.availableSignals}</p>}
          <div className="mt-3 divide-y divide-slate-200 rounded-lg border border-slate-200">
            <label className="flex items-start gap-3 px-3.5 py-3">
              <input type="checkbox" checked disabled className="mt-0.5 size-4 shrink-0 accent-accent" />
              <span>
                <span className="block text-sm font-medium">Booking timestamp <span className="text-xs font-normal text-slate-600">· Required</span></span>
                <span className="mt-1 block text-xs text-slate-600">When the booking event occurred</span>
              </span>
            </label>
            <label className="flex cursor-pointer items-start gap-3 px-3.5 py-3 hover:bg-slate-50">
              <input type="checkbox" checked={includesAmount} className="mt-0.5 size-4 shrink-0 accent-accent" onChange={event => {
                setIncludesAmount(event.currentTarget.checked);
                setNotice("");
              }} />
              <span>
                <span className="block text-sm font-medium">Booking amount</span>
                <span className="mt-1 block text-xs text-slate-600">Amount in minor currency units, such as cents</span>
              </span>
            </label>
          </div>
        </fieldset>
      </div>

      <div className="p-5 sm:p-6">
        {storageError && <p role="alert" className="mb-4 flex items-start gap-2 text-sm text-red-800"><CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />{storageError}</p>}
        <div className="flex flex-wrap items-center gap-3">
          <button type="submit" className="rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-white hover:bg-indigo-700 disabled:opacity-60">Save setup</button>
          <span role="status" className="flex items-center gap-1.5 text-sm text-emerald-800">
            {notice && <><Check className="size-4" aria-hidden="true" />{notice}</>}
          </span>
        </div>
        <p className="mt-3 text-xs text-slate-600">Demo settings are stored in this browser.</p>
      </div>
    </fieldset>
  </form>;
}
