import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { MerchantDirectory } from "./directory";
import { asMerchantTab, merchantDetailsHref } from "./management-contracts";

export const metadata: Metadata = { title: "Merchants | PayGuard" };

export default async function MerchantsPage({ searchParams }: {
  searchParams: Promise<{ merchant?: string; transaction?: string; view?: string; tab?: string }>;
}) {
  const { merchant, transaction, view, tab } = await searchParams;
  const preferredTab = transaction ? "transactions" : asMerchantTab(tab ?? view);
  if (typeof merchant === "string" && merchant) {
    const href = merchantDetailsHref(merchant, preferredTab, transaction);
    redirect(!transaction && (tab ?? view) === "jev" ? `${href}#booking-review-rules` : href);
  }
  return <main className="mx-auto max-w-[1400px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <MerchantDirectory preferredTab={preferredTab} transaction={typeof transaction === "string" ? transaction : undefined} />
  </main>;
}
