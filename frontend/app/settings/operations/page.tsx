import type { Metadata } from "next";
import { OperationsWorkspace } from "./workspace";

export const metadata: Metadata = { title: "Risk operations | PayGuard" };

export default function OperationsPage() {
  return <main className="mx-auto max-w-[1400px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header className="mb-6">
      <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Risk operations</h1>
      <p className="mt-2 text-sm text-slate-600">Inspect synthetic booking assessments and work the manual review queue.</p>
    </header>
    <div id="transactions"><OperationsWorkspace /></div>
  </main>;
}
