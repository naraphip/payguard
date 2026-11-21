import type { Metadata } from "next";
import { MerchantDirectory } from "./directory";

export const metadata: Metadata = { title: "Merchants | PayGuard" };

export default async function MerchantsPage({ searchParams }: {
  searchParams: Promise<{ merchant?: string; transaction?: string; view?: string }>;
}) {
  const { merchant, transaction, view } = await searchParams;
  return (
    <main className="mx-auto max-w-[1400px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
      <MerchantDirectory key={JSON.stringify([merchant, transaction, view])} />
    </main>
  );
}
