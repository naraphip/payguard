import { connection } from "next/server";
import { redirect } from "next/navigation";
import type { Metadata } from "next";
import { SystemRulesDashboard } from "./dashboard";

export const metadata: Metadata = { title: "Platform Rules | PayGuard" };

export default async function SystemRulesPage() {
  await connection();
  if (process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim()) redirect("/settings/merchants?view=platform");
  return <SystemRulesDashboard />;
}
