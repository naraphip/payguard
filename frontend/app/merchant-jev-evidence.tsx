import { merchantJevStatus, merchantOutcomeLabels } from "./merchant-demo";
import type { DemoTransaction } from "./merchant-demo";

export function MerchantJevEvidence({ transaction }: { transaction: DemoTransaction }) {
  return <>
    <div className="min-w-0">
      <dt className="text-xs font-medium text-ink">Jev status</dt>
      <dd className="text-muted">{merchantJevStatus(transaction)}</dd>
    </div>
    {transaction.jevVersion && <div className="min-w-0">
      <dt className="text-xs font-medium text-ink">Jev model</dt>
      <dd className="text-muted wrap-anywhere">{transaction.jevVersion}</dd>
    </div>}
    {typeof transaction.jevConfigurationVersion === "number" && <div className="min-w-0">
      <dt className="text-xs font-medium text-ink">Jev configuration version</dt>
      <dd className="text-muted">v{transaction.jevConfigurationVersion}</dd>
    </div>}
    {transaction.jevAnswer && <div className="min-w-0">
      <dt className="text-xs font-medium text-ink">Jev answer</dt>
      <dd className="text-muted">
        {merchantOutcomeLabels[transaction.jevAnswer]}
        {typeof transaction.jevConfidence === "number" && ` · ${Math.round(transaction.jevConfidence * 100)}% confidence`}
      </dd>
    </div>}
    {transaction.jevDecisionId && <div className="min-w-0">
      <dt className="text-xs font-medium text-ink">Jev decision ID</dt>
      <dd className="font-mono text-xs text-muted wrap-anywhere">{transaction.jevDecisionId}</dd>
    </div>}
  </>;
}
