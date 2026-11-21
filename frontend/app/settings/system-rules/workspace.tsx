"use client";

import Link from "next/link";
import { useCallback, useState } from "react";
import { RuleAccess, RuleComposer, useRuleCredential } from "../rule-composer";
import { SystemRulesDashboard } from "./dashboard";
import { PlatformGovernance } from "./governance";
import { globalRuleTransport } from "./contracts";

export function PlatformRulesWorkspace() {
  const token = useRuleCredential("PLATFORM");
  const [revision, setRevision] = useState(0);

  const request = useCallback<typeof globalRuleTransport>((suffix, init) => {
    const headers = new Headers(init?.headers);
    if (token) headers.set("Authorization", `Bearer ${token}`);
    return globalRuleTransport(suffix, { ...init, headers });
  }, [token]);

  return <main className="mx-auto max-w-[1360px] space-y-6 px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header>
      <h1 className="text-[30px] font-semibold tracking-tight sm:text-[36px]">Platform Rules</h1>
      <p className="mt-2 text-sm text-muted">Draft global fraud rules for platform review.</p>
    </header>
    <RuleAccess scope="PLATFORM" />
    <RuleComposer scope="PLATFORM" onSaved={() => setRevision(value => value + 1)} />
    <PlatformGovernance catalogueRevision={revision} />
    <SystemRulesDashboard key={revision} request={request} canEdit={Boolean(token)} embedded />
    <Link href="/settings/merchants?view=platform" className="inline-block text-sm font-semibold text-accent hover:underline">
      Open merchant-specific platform drafts
    </Link>
  </main>;
}
