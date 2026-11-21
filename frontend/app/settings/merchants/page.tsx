import type { Metadata } from "next";
import { MerchantDirectory } from "./directory";

export const metadata: Metadata = { title: "Merchants | PayGuard" };

export default function MerchantsPage() {
  return (
    <main className="mx-auto max-w-[1400px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
      <header className="mb-6">
        <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Merchants</h1>
        <p className="mt-2 text-sm text-slate-600">Manage demo merchants, configure their scenarios, and review mock Jev decisions.</p>
      </header>
      <MerchantDirectory />
    </main>
  );
}
