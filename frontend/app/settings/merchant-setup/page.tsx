import type { Metadata } from "next";
import { ChevronRight } from "lucide-react";
import { PayGuardLogo } from "../../payguard-logo";
import { SettingsNavigation } from "../navigation";
import { MerchantSetupForm } from "./setup-form";

export const metadata: Metadata = { title: "Merchant setup | PayGuard" };

export default function MerchantSetupPage() {
  return <div className="min-h-screen bg-[#f8fafc] text-slate-900 md:grid md:grid-cols-[218px_minmax(0,1fr)]">
    <aside className="border-b border-slate-200 bg-white md:sticky md:top-0 md:h-dvh md:border-r md:border-b-0">
      <div className="px-4 pt-4 pb-3 md:border-b md:border-slate-100 md:px-5 md:py-4"><PayGuardLogo /></div>
      <div className="px-4 pb-3 md:px-3 md:py-5"><SettingsNavigation current="/settings/merchant-setup" /></div>
    </aside>
    <main className="min-w-0">
      <div className="flex min-h-[69px] items-center gap-2 border-b border-slate-200 bg-white px-4 text-sm text-slate-500 sm:px-7 lg:px-10">
        <span>Settings</span><ChevronRight className="size-3.5 text-slate-400" aria-hidden="true" />
        <span className="font-medium text-slate-900">Merchant setup</span>
      </div>
      <div className="mx-auto max-w-[960px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
        <header className="mb-6">
          <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Merchant setup</h1>
          <p className="mt-2 text-sm leading-relaxed text-slate-600">Choose your business model and the signals available at booking.</p>
        </header>
        <MerchantSetupForm />
      </div>
    </main>
  </div>;
}
