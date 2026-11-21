import type { Metadata } from "next";
import { SystemRulesDashboard } from "./dashboard";

export const metadata: Metadata = { title: "Platform Rules | PayGuard" };

export default function SystemRulesPage() {
  return <SystemRulesDashboard />;
}
