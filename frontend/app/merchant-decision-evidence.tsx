import { merchantOutcomeLabels } from "./merchant-demo";
import type { DemoTransaction } from "./merchant-demo";
import { MerchantJevEvidence } from "./merchant-jev-evidence";
import { PolicyDecisionDetails } from "./merchant-policy-decision";

const count = new Intl.NumberFormat("en-US");

export function MerchantDecisionEvidence({ transaction }: { transaction: DemoTransaction }) {
  return <details className="rounded-lg border border-line bg-white px-3 text-sm">
    <summary className="cursor-pointer py-3 font-semibold">Decision evidence</summary>
    <div className="space-y-3 border-t border-line pb-3 pt-2">
      {transaction.policyDecision && <PolicyDecisionDetails decision={transaction.policyDecision} />}
      <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <div><dt className="text-xs text-muted">Automated risk result</dt><dd>{merchantOutcomeLabels[transaction.outcome]}</dd></div>
        {transaction.ruleVersion && <div><dt className="text-xs text-muted">Platform ruleset</dt><dd className="wrap-anywhere">{transaction.ruleVersion}</dd></div>}
        {transaction.scenarioName && <div><dt className="text-xs text-muted">Saved scenario</dt><dd className="wrap-anywhere">{transaction.scenarioName}{transaction.scenarioVersion && ` · v${transaction.scenarioVersion}`}</dd></div>}
        {transaction.jevRuleVersion && <div><dt className="text-xs text-muted">Merchant Jev rules</dt><dd className="wrap-anywhere">{transaction.jevRuleVersion}</dd></div>}
        {transaction.thresholdMinor != null && <div><dt className="text-xs text-muted">Saved review threshold</dt><dd>{count.format(transaction.thresholdMinor)} minor units</dd></div>}
        <MerchantJevEvidence transaction={transaction} />
        <div><dt className="text-xs text-muted">Rule hits</dt><dd className="wrap-anywhere">{transaction.ruleHits.join(", ") || "None"}</dd></div>
      </dl>
      {transaction.reason && <p className="wrap-anywhere">
        <strong>{transaction.disposition ? "Original assessment reason" : "Assessment reason"}</strong> {transaction.reason}
      </p>}
      {transaction.jevReason && transaction.jevReason !== transaction.reason && <p className="wrap-anywhere">
        <strong>Jev assessment</strong> {transaction.jevReason}
      </p>}
    </div>
  </details>;
}
