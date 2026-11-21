import type { MerchantRiskScore } from "./merchant-demo";

const factorLabels: Record<string, string> = {
  BASE: "Starting score",
  ACCOUNT_AGE_NEW: "Account age of 7 days or less",
  ACCOUNT_AGE_OBSERVED: "Account age above 7 days",
  ACCOUNT_AGE_MISSING: "Account age unavailable",
  DISPOSABLE_EMAIL_DOMAIN: "Listed disposable email domain",
  EMAIL_DOMAIN_NOT_LISTED: "Email domain absent from the disposable list",
  EMAIL_DOMAIN_MISSING: "Email domain missing",
  EMAIL_DOMAIN_UNAVAILABLE: "Disposable email check unavailable",
  CUSTOMER_EMAIL_CHANGED_AFTER_DISPOSABLE: "Same customer ID · email changed after a disposable email booking",
  PLATFORM_CHALLENGE: "Platform rule matched · Challenge",
  PLATFORM_REVIEW: "Platform rule matched · Review",
  PLATFORM_BLOCK: "Platform rule matched · Block",
  PLATFORM_EVIDENCE_INCOMPLETE: "Some platform rule signals were unavailable",
  MERCHANT_CHALLENGE: "Merchant rule matched · Challenge",
  MERCHANT_REVIEW: "Merchant rule matched · Review",
  MERCHANT_BLOCK: "Merchant rule matched · Block",
  MERCHANT_EVIDENCE_INCOMPLETE: "Some merchant rule signals were unavailable",
  AMOUNT_RULE_REVIEW: "Amount-based rule matched · Review",
  JEV_LIVE_REVIEW: "Live Jev · Review",
  JEV_LIVE_LOW_RISK: "Live Jev · Low risk",
  JEV_MOCKED_REVIEW: "Mock Jev · Review",
  JEV_MOCKED_LOW_RISK: "Mock Jev · Low risk",
  JEV_TIMEOUT: "Jev timed out",
  JEV_UNAVAILABLE: "Jev unavailable",
  JEV_NOT_RUN: "Jev assessment skipped",
};

function factorLabel(code: string) {
  return factorLabels[code] ?? code.toLowerCase().replaceAll("_", " ");
}

export function riskScoreLabel(score?: MerchantRiskScore) {
  if (!score) return "Not recorded";
  if (score.value === null) return "Insufficient evidence";
  return `${score.value}/100`;
}

export function RiskScoreDetails({ score }: { score?: MerchantRiskScore }) {
  if (!score) return <p className="text-xs text-muted">Risk score · Not recorded</p>;

  return <details className="rounded-lg border border-line bg-white px-3 text-sm">
    <summary className="cursor-pointer py-3 font-semibold text-ink">
      <span className="ml-1">Risk score</span>
      <span className={`ml-2 tabular-nums ${score.value === null ? "text-amber-900" : "text-accent"}`}>{riskScoreLabel(score)}</span>
      {score.partial && score.value !== null && <span className="ml-2 rounded-full bg-amber-50 px-2 py-0.5 text-xs text-amber-900">Partial evidence</span>}
    </summary>
    <div className="border-t border-line pb-3 pt-2">
      <p className="text-xs text-muted">Use this advisory index alongside the final decision. Points show the effect of available evidence. A matched rule or Jev review adds points only if its minimum exceeds the current score.</p>
      <dl className="mt-2 divide-y divide-line">
        {score.factors.filter(({ code }) => score.value !== null || code !== "BASE").map(({ code, points }) => (
          <div key={code} className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 py-1.5">
            <dt className="wrap-anywhere">{factorLabel(code)}</dt>
            <dd className={`tabular-nums ${points === 0 ? "text-muted" : "font-semibold"}`}>{points > 0 ? "+" : ""}{points}</dd>
          </div>
        ))}
      </dl>
      <p className="mt-2 text-xs text-muted">Scoring version <span className="font-mono">{score.version}</span></p>
    </div>
  </details>;
}
