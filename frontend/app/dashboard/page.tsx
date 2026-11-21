import type { Metadata } from "next";
import { DashboardOverview } from "./overview";

export const metadata: Metadata = { title: "Dashboard | PayGuard" };

export default function DashboardPage() {
  return <DashboardOverview />;
}
