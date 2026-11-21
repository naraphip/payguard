import { policyActionLabels } from "./merchant-demo";
import type { MerchantPolicyDecision } from "./merchant-demo";

const sources: Record<string, string> = { PLATFORM: "Platform fraud rules", MERCHANT_POLICY: "Merchant policy", JEV_TIMEOUT: "Jev timeout", NONE: "No restrictive rule" };

export function PolicyDecisionDetails({ decision }: { decision: MerchantPolicyDecision }) {
  const { platformDecision, merchantPolicyDecision, finalDecision } = decision;
  return <dl className="grid gap-3 rounded-lg border border-line bg-white p-3 text-sm sm:grid-cols-3">
    <div>
      <dt className="text-xs text-muted">Final decision</dt>
      <dd className="mt-0.5 font-semibold">{policyActionLabels[finalDecision.action] ?? finalDecision.action}</dd>
      <dd className="text-xs text-muted">{sources[finalDecision.source] ?? finalDecision.source}</dd>
    </div>
    <div>
      <dt className="text-xs text-muted">Platform decision</dt>
      <dd className="mt-0.5 font-semibold">{policyActionLabels[platformDecision.action] ?? platformDecision.action}</dd>
      {!!platformDecision.reasonCodes?.length && <dd className="text-xs text-muted wrap-anywhere">{platformDecision.reasonCodes.join(", ")}</dd>}
    </div>
    <div>
      <dt className="text-xs text-muted">Merchant policy decision</dt>
      <dd className="mt-0.5 font-semibold">{policyActionLabels[merchantPolicyDecision.action] ?? merchantPolicyDecision.action}</dd>
      {!!merchantPolicyDecision.reasonCodes?.length && <dd className="text-xs text-muted wrap-anywhere">{merchantPolicyDecision.reasonCodes.join(", ")}</dd>}
      {!!merchantPolicyDecision.missingSignals?.length && <dd className="text-xs text-amber-900 wrap-anywhere">
        Missing signals {merchantPolicyDecision.missingSignals.join(", ")}
      </dd>}
      {!!merchantPolicyDecision.matchedRuleIds?.length && <dd className="text-xs text-muted wrap-anywhere">
        {merchantPolicyDecision.matchedRuleIds.map(id => {
          const version = merchantPolicyDecision.ruleVersions?.[id];
          return version ? `${id} (v${version})` : id;
        }).join(", ")}
      </dd>}
    </div>
  </dl>;
}
