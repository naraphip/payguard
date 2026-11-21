import { Check, Clock3, X } from "lucide-react";
import type { DemoTransaction, MerchantTransactionStatus } from "./merchant-demo";
import { merchantDate } from "./settings/merchants/management-contracts";

const appearances = {
  APPROVED: { label: "Approved", Icon: Check, classes: "border-emerald-200 bg-emerald-50 text-emerald-900" },
  REJECTED: { label: "Rejected", Icon: X, classes: "border-red-200 bg-red-50 text-red-900" },
  REVIEW: { label: "Review", Icon: Clock3, classes: "border-amber-200 bg-amber-50 text-amber-900" },
} as const;

export const transactionStatuses: MerchantTransactionStatus[] = ["APPROVED", "REJECTED", "REVIEW"];

export function transactionStatusLabel(status: MerchantTransactionStatus) {
  return appearances[status].label;
}

export function transactionStatus(transaction: DemoTransaction): MerchantTransactionStatus {
  if (transaction.disposition === "cleared") return "APPROVED";
  if (transaction.disposition === "rejected") return "REJECTED";
  if (transaction.status) return transaction.status;
  const action = transaction.policyDecision?.finalDecision.action ?? transaction.outcome;
  if (transaction.outcome === "HIGH_RISK" || action === "BLOCK" || action === "HIGH_RISK") return "REJECTED";
  if (action === "ALLOW" || action === "LOW_RISK") return "APPROVED";
  return "REVIEW";
}

export function TransactionStatusBadge({ status }: { status: MerchantTransactionStatus }) {
  const { label, Icon, classes } = appearances[status];
  return <span className={`inline-flex w-fit items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 py-1 text-xs font-semibold ${classes}`}>
    <Icon size={14} aria-hidden="true" />{label}
  </span>;
}

export function transactionReviewCause(transaction: DemoTransaction) {
  const final = transaction.policyDecision?.finalDecision;
  if (final?.action === "CHALLENGE") return "A rule requested additional verification.";
  if (final?.source === "MERCHANT_POLICY" && final.action === "UNRESOLVED") {
    return "A merchant rule needed more evidence.";
  }
  if (final?.source === "PLATFORM") return "A platform rule flagged this transaction.";
  if (final?.source === "MERCHANT_POLICY") return "A merchant rule flagged this transaction.";
  if (transaction.jevStatus === "TIMEOUT") return "Jev timed out.";
  if (transaction.jevStatus === "UNAVAILABLE") return "Jev was unavailable.";
  if (transaction.jevStatus === "NOT_RUN") return "Jev did not have enough information to assess this transaction.";
  if (transaction.jevAnswer === "REVIEW") return "Jev recommended review.";
  return "The automated assessment called for review.";
}

export function TransactionDisposition({ transaction }: { transaction: DemoTransaction }) {
  if (!transaction.disposition) return null;
  return <p className="text-sm text-muted wrap-anywhere">
    <strong className="text-ink">{transaction.disposition === "cleared" ? "Manually approved" : "Manually rejected"}</strong>
    {transaction.decidedBy && ` · ${transaction.decidedBy}`}
    {transaction.decidedAt && <> · <time dateTime={transaction.decidedAt}>{merchantDate(transaction.decidedAt)}</time></>}
  </p>;
}
