import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { MerchantDetail } from "../merchant-detail";
import { asMerchantTab, merchantDetailsHref } from "../management-contracts";

export const metadata: Metadata = { title: "Merchant details | PayGuard" };

export default async function MerchantDetailsPage({ params, searchParams }: {
  params: Promise<{ merchantId: string }>;
  searchParams: Promise<{ tab?: string; transaction?: string }>;
}) {
  const [{ merchantId }, search] = await Promise.all([params, searchParams]);
  const transaction = typeof search.transaction === "string" ? search.transaction : undefined;
  if (!transaction && search.tab === "jev") redirect(`${merchantDetailsHref(merchantId, "rules")}#booking-review-rules`);
  const tab = transaction ? "transactions" : asMerchantTab(search.tab);
  return <main className="mx-auto max-w-[1400px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <MerchantDetail merchantId={merchantId} key={merchantId} tab={tab} transaction={transaction} />
  </main>;
}
