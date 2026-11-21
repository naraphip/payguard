import type { Metadata } from "next";
import SimulationLab from "../simulation-lab";

export const metadata: Metadata = { title: "Simulation Lab | PayGuard" };

export default async function Page({ searchParams }: { searchParams: Promise<{ run?: string | string[] }> }) {
  const { run } = await searchParams;
  const sharedRunId = Array.isArray(run) ? run.join(",") : run;
  return <SimulationLab key={sharedRunId} sharedRunId={sharedRunId} />;
}
