import type { Metadata } from "next";
import { connection } from "next/server";
import { redirect } from "next/navigation";
import { OperationsWorkspace } from "../operations/workspace";

export const metadata: Metadata = { title: "Merchant Rules | PayGuard" };

export default async function MerchantRulesPage() {
  await connection();
  if (process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim()) redirect("/settings/merchants?view=jev");
  return <main className="mx-auto max-w-[1400px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header className="mb-6">
      <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Merchant Rules</h1>
      <p className="mt-2 text-sm text-slate-600">Review rule changes and evaluate booking rules for your workspace.</p>
    </header>
    <OperationsWorkspace initialSection="rules" />
  </main>;
}
