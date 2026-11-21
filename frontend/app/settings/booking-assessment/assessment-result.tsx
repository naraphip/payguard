import type { Assessment, BookingReceipt } from "./booking-api";

export const featureLabels: Record<string, string> = {
  occurredAt: "Booking timestamp",
  amountMinor: "Booking amount",
  bookingVelocity60s: "Distinct bookings in 60 seconds",
};

const sdkLabels = { observed: "Observed", unavailable: "Unavailable", unknown: "Unknown" } as const;

export function AssessmentResult({ assessment, booking, context }: { assessment: Assessment; booking?: BookingReceipt; context?: string }) {
  const { assessedAt, baseline, eventId, features, handoff, jev, jevRuleVersion, outcome, profileVersion, ruleHits, ruleVersion } = assessment;
  const unresolved = outcome === "UNRESOLVED";
  const resultLabel = unresolved ? "Unresolved" : outcome === "LOW_RISK" ? "Low risk" : "Review";
  return <section className="min-w-0 rounded-xl border border-slate-200 bg-white p-5 shadow-xs sm:p-6" aria-labelledby="assessment-result-heading">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h2 id="assessment-result-heading" className="text-lg font-semibold">Assessment result</h2>
      <span role="status" className={`rounded-full px-3 py-1 text-xs font-semibold ${unresolved ? "bg-amber-100 text-amber-900" : "bg-indigo-100 text-indigo-900"}`}>{resultLabel}</span>
    </div>
    {context && <p className="mt-2 text-sm text-slate-600">{context}</p>}
    {handoff === "MANUAL_REVIEW" && <p className="mt-3 rounded-lg bg-amber-50 p-3 text-sm text-amber-900">Manual review required. No payment action was taken.</p>}
    <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-2">
      <Detail label="Event ID" value={eventId} />
      {booking && <Detail label="Booking received" value={booking.receivedAt} />}
      {assessedAt && <Detail label="Assessed at" value={assessedAt} />}
      {booking?.sdkStatus && <Detail label="SDK signal" value={sdkLabels[booking.sdkStatus]} />}
      <Detail label="Profile version" value={profileVersion} />
      <Detail label="Rule version" value={ruleVersion} />
      {jevRuleVersion && <Detail label="Jev custom rules version" value={jevRuleVersion} />}
      <Detail label="Baseline" value={baseline.state === "COLD_START" ? "Cold start" : baseline.version ?? baseline.state} />
      {baseline.sampleCount !== undefined && <Detail label="Baseline samples" value={String(baseline.sampleCount)} />}
      <Detail label="Jev status" value={jev.status === "TIMEOUT" ? "Timeout" : "Mock response"} />
      <Detail label="Jev answer" value={jev.answer} />
      <Detail label="Jev question" value={jev.question} />
      <Detail label="Jev model and rubric" value={`${jev.modelVersion} · ${jev.rubricVersion}`} />
      <Detail label="Rule hits" value={ruleHits.length ? ruleHits.join(", ") : "None"} />
    </dl>
    <h3 className="mt-6 text-sm font-semibold">Signal evidence</h3>
    <dl className="mt-2 divide-y divide-slate-200 rounded-lg border border-slate-200">
      {features.map(feature => <div key={feature.name} className="grid gap-1 p-3 text-sm sm:grid-cols-[1fr_1fr] sm:gap-3">
        <dt className="font-medium">{featureLabels[feature.name] ?? feature.name}</dt>
        <dd className="min-w-0 text-slate-700 break-words">
          <span>{feature.presence === "MISSING" ? "Missing" : feature.value ?? feature.presence}</span>
          <span className="block text-xs text-slate-500">{feature.source}</span>
        </dd>
      </div>)}
    </dl>
  </section>;
}

function Detail({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0">
    <dt className="text-xs font-medium text-slate-600">{label}</dt>
    <dd className="mt-1 break-words">{value}</dd>
  </div>;
}
