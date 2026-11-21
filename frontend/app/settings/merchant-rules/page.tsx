import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { MerchantRulesWorkspace } from "../rule-composer";

export const metadata: Metadata = { title: "Merchant AI Rules | PayGuard" };

export default async function MerchantRulesPage({ searchParams }: { searchParams: Promise<{ merchant?: string; tab?: string }> }) {
  const { merchant, tab } = await searchParams;
  if (tab === "review") {
    const query = typeof merchant === "string" && merchant ? `?${new URLSearchParams({ merchant })}` : "";
    redirect(`/settings/merchant-rules${query}`);
  }
  return <MerchantRulesWorkspace />;
}
