import type { Metadata } from "next";
import { PlatformRulesWorkspace } from "./workspace";

export const metadata: Metadata = { title: "Platform Rules | PayGuard" };

export default function SystemRulesPage() {
  return <PlatformRulesWorkspace />;
}
