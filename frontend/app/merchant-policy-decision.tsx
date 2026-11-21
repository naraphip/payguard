import { policyActionLabels } from "./merchant-demo";
import type { MerchantPolicyDecision } from "./merchant-demo";
import { merchantFieldLabels } from "./settings/structured-rule-builder";

const sources: Record<string, string> = {
  PLATFORM: "Platform fraud rules", MERCHANT_POLICY: "Merchant policy", JEV: "Jev", JEV_TIMEOUT: "Jev timeout", NONE: "No restrictive rule",
};
const jevAnswers: Record<string, string> = { MATCH: "Match", NO_MATCH: "No match", UNKNOWN: "Unresolved" };
const jevStatuses: Record<string, string> = { LIVE: "Live", MOCKED: "Mock", TIMEOUT: "Timed out", UNAVAILABLE: "Unavailable", NOT_RUN: "Not run" };

function DecisionPartDetails({ label, part }: { label: string; part: MerchantPolicyDecision["merchantPolicyDecision"] }) {
  return <div>
    <dt className="text-xs text-muted">{label}</dt>
    <dd className="mt-0.5 font-semibold">{policyActionLabels[part.action] ?? part.action}</dd>
    {!!part.reasonCodes?.length && <dd className="text-xs text-muted wrap-anywhere">{part.reasonCodes.join(", ")}</dd>}
    {!!part.missingSignals?.length && <dd className="text-xs text-amber-900 wrap-anywhere">
      Missing signals {part.missingSignals.join(", ")}
    </dd>}
    {!!part.matchedRuleIds?.length && <dd className="text-xs text-muted wrap-anywhere">
      Matched {part.matchedRuleIds.map(id => {
        const version = part.ruleVersions?.[id];
        return version ? `${id} (v${version})` : id;
      }).join(", ")}
    </dd>}
    {!!part.jevConditions?.length && <dd className="mt-2 text-xs text-muted">
      <p className="font-semibold text-ink">Jev condition results</p>
      <ul className="mt-1 space-y-1">
        {part.jevConditions.map(({ ruleId, version, conditionIndex, field, answer, status, model, decisionId }, index) => <li
          className="wrap-anywhere" key={`${ruleId}:${version}:${field}:${conditionIndex ?? index}`}>
          <p>{merchantFieldLabels[field] ?? field} · {answer ? jevAnswers[answer] ?? answer : "Unresolved"} · {jevStatuses[status] ?? status}</p>
          <p>Rule {ruleId} (v{version}){conditionIndex !== undefined && ` · Condition ${conditionIndex + 1}`}{model && ` · ${model}`}{decisionId && ` · Decision ${decisionId}`}</p>
        </li>)}
      </ul>
    </dd>}
  </div>;
}

export function PolicyDecisionDetails({ decision }: { decision: MerchantPolicyDecision }) {
  const { platformDecision, merchantPolicyDecision, jevDecision, finalDecision } = decision;
  return <dl className={`grid gap-3 rounded-lg border border-line bg-white p-3 text-sm ${jevDecision ? "sm:grid-cols-2 lg:grid-cols-4" : "sm:grid-cols-3"}`}>
    <div>
      <dt className="text-xs text-muted">Final decision</dt>
      <dd className="mt-0.5 font-semibold">{policyActionLabels[finalDecision.action] ?? finalDecision.action}</dd>
      <dd className="text-xs text-muted">{sources[finalDecision.source] ?? finalDecision.source}</dd>
    </div>
    <DecisionPartDetails label="Platform decision" part={platformDecision} />
    <DecisionPartDetails label="Merchant policy decision" part={merchantPolicyDecision} />
    {jevDecision && <DecisionPartDetails label="Jev decision" part={jevDecision} />}
  </dl>;
}
