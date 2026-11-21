import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { MerchantRulesWorkspace } from "../rule-composer";

export const metadata: Metadata = { title: "Merchant AI Rules | PayGuard" };

export default async function MerchantRulesPage({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const { tab } = await searchParams;
  if (tab === "review") redirect("/settings/merchant-rules");
  return <MerchantRulesWorkspace />;
}
