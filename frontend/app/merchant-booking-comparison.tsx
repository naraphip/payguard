import { merchantPolicyProducts } from "./merchant-demo";
import type { MerchantBookingComparison } from "./merchant-demo";

const amountNumber = new Intl.NumberFormat("en-US");
const descriptionLabels = { SAME: "Same", CHANGED: "Changed", UNKNOWN: "Unavailable" } as const;
const similarityLabels = { SIMILAR: "Similar", DIFFERENT: "Different", INSUFFICIENT: "Insufficient evidence" } as const;
const statusLabels = { LIVE: "Live Jev", MOCKED: "Mock Jev", TIMEOUT: "Jev timed out", UNAVAILABLE: "Jev unavailable", NOT_RUN: "Jev not run" } as const;

function productLabel(product: string) {
  return merchantPolicyProducts[product as keyof typeof merchantPolicyProducts] ?? "Unavailable";
}

function amountLabel(amount: number | null) {
  return amount === null ? "Unavailable" : amountNumber.format(amount);
}

export function BookingComparisonDetails({ comparison }: { comparison: MerchantBookingComparison }) {
  const { prior, current, descriptionComparison, customFieldComparison, similarity, status, confidence } = comparison;
  return <section aria-label="Previous booking comparison" className="rounded-lg border border-line bg-white p-3 text-sm">
    <h4 className="font-semibold">Previous booking comparison</h4>
    <p className="mt-1 text-xs text-muted">Matched the same email at this merchant.</p>
    <table className="mt-3 w-full table-fixed border-collapse text-left text-xs sm:text-sm">
      <thead className="border-b border-line text-muted">
        <tr>
          <th scope="col" className="py-1.5 pr-2 font-medium">Booking detail</th>
          <th scope="col" className="py-1.5 pr-2 font-medium">Previous booking</th>
          <th scope="col" className="py-1.5 font-medium">This booking</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-line">
        <tr>
          <th scope="row" className="py-1.5 pr-2 font-medium">Product</th>
          <td className="py-1.5 pr-2 wrap-anywhere">{productLabel(prior.product)}</td>
          <td className="py-1.5 wrap-anywhere">{productLabel(current.product)}</td>
        </tr>
        <tr>
          <th scope="row" className="py-1.5 pr-2 font-medium">Amount <span className="font-normal text-muted">(minor units)</span></th>
          <td className="py-1.5 pr-2 tabular-nums wrap-anywhere">{amountLabel(prior.amountMinor)}</td>
          <td className="py-1.5 tabular-nums wrap-anywhere">{amountLabel(current.amountMinor)}</td>
        </tr>
      </tbody>
    </table>
    <dl className="mt-3 grid grid-cols-2 gap-3 border-t border-line pt-3 xl:grid-cols-4">
      <div>
        <dt className="text-xs text-muted">Jev similarity</dt>
        <dd className="font-semibold">{similarity ? similarityLabels[similarity] : statusLabels[status]}</dd>
        {similarity && <dd className="text-xs text-muted">{statusLabels[status]}
          {typeof confidence === "number" && ` · ${Math.round(confidence * 100)}% confidence`}
        </dd>}
      </div>
      <div>
        <dt className="text-xs text-muted">Description comparison</dt>
        <dd>{descriptionLabels[descriptionComparison]}</dd>
      </div>
      {customFieldComparison && customFieldComparison.same + customFieldComparison.changed > 0 && <div>
        <dt className="text-xs text-muted">Custom fields compared</dt>
        <dd>{customFieldComparison.same} same · {customFieldComparison.changed} changed</dd>
      </div>}
      {prior.disposition && <div>
        <dt className="text-xs text-muted">Previous manual review</dt>
        <dd>{prior.disposition === "cleared" ? "Cleared" : "Rejected"}</dd>
      </div>}
    </dl>
  </section>;
}
