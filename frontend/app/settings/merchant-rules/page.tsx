import type { Metadata } from "next";
import { MerchantRulesWorkspace } from "../rule-composer";
import { JevRulesPanel } from "../jev-rules/panel";

export const metadata: Metadata = { title: "Merchant AI Rules | PayGuard" };

export default async function MerchantRulesPage({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const { tab } = await searchParams;
  return tab === "review" ? <JevRulesPanel /> : <MerchantRulesWorkspace />;
}
