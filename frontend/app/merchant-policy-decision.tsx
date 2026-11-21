import { policyActionLabels } from "./merchant-demo";
import type { MerchantPolicyDecision } from "./merchant-demo";

const sources: Record<string, string> = {
  PLATFORM: "Platform fraud rules", MERCHANT_POLICY: "Merchant policy", JEV: "Jev", JEV_TIMEOUT: "Jev timeout", NONE: "No restrictive rule",
};

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
