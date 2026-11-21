import type { Metadata } from "next";
import { MerchantSetupForm } from "./setup-form";

export const metadata: Metadata = { title: "Merchant setup | PayGuard" };

export default function MerchantSetupPage() {
  return <main className="mx-auto max-w-[960px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header className="mb-6">
      <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Merchant setup</h1>
      <p className="mt-2 text-sm leading-relaxed text-slate-600">Choose your business model and the signals available at booking.</p>
    </header>
    <MerchantSetupForm />
  </main>;
}
