"use client";

import { useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import {
  LocalMerchantPreviewDisabled, MerchantDemoError, merchantDemoRequest, setMerchantDemoToken,
} from "../../merchant-demo";
import type { DemoDirectory } from "../../merchant-demo";
import { useRuleCredential } from "../rule-composer";
import { outlineButton, primaryButton } from "../system-rules/contracts";
import type { MerchantDirectoryPage } from "./management-contracts";

const directoryProbe = "?page=1&limit=1&sort=name";

export function managementError(error: unknown) {
  return error instanceof Error ? error.message : "Could not reach the merchant workspace.";
}

export function MerchantManagementAccess({ merchant, children }: {
  merchant: boolean;
  children: (token: string) => ReactNode;
}) {
  const token = useRuleCredential(merchant ? "MERCHANT" : "PLATFORM");
  const [preview, setPreview] = useState<"checking" | "enabled" | "disabled">("checking");
  const [credential, setCredential] = useState("");
  const [error, setError] = useState("");
  const [connecting, setConnecting] = useState(false);
  const signIn = useRef<AbortController>(null);

  useEffect(() => () => signIn.current?.abort(), []);
  useEffect(() => {
    if (merchant || token) return;
    const controller = new AbortController();
    void merchantDemoRequest<MerchantDirectoryPage>(directoryProbe, "", { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) setPreview(result.role === "admin" ? "enabled" : "disabled"); })
      .catch(error => {
        if (controller.signal.aborted) return;
        setPreview("disabled");
        if (!(error instanceof LocalMerchantPreviewDisabled)) setError(managementError(error));
      });
    return () => controller.abort();
  }, [merchant, token]);

  async function connect(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const bearer = credential.trim();
    if (!bearer || signIn.current) return;
    const controller = new AbortController();
    const { signal } = controller;
    signIn.current = controller;
    setConnecting(true);
    setError("");
    try {
      let role: DemoDirectory["role"];
      if (merchant) {
        const directory = await merchantDemoRequest<DemoDirectory>("", bearer, { signal });
        if (directory.role !== "merchant") throw new Error("Use the access token issued for your merchant.");
        role = directory.role;
      } else {
        try {
          const directory = await merchantDemoRequest<MerchantDirectoryPage>(directoryProbe, bearer, { signal });
          role = directory.role;
        } catch (error) {
          if (!(error instanceof MerchantDemoError) || error.status !== 403) throw error;
          role = (await merchantDemoRequest<DemoDirectory>("", bearer, { signal })).role;
        }
      }
      if (signal.aborted) return;
      setCredential("");
      setMerchantDemoToken(bearer, role);
    } catch (error) {
      if (!signal.aborted) setError(managementError(error));
    } finally {
      if (!signal.aborted) setConnecting(false);
      signIn.current = null;
    }
  }

  if (token || (!merchant && preview === "enabled")) return <>
    <div className="mb-4 flex flex-wrap items-center justify-between gap-3 text-xs text-muted">
      <p>{token ? (merchant ? "Merchant access" : "Administrator access") : "Local sandbox preview"}</p>
      {token && <button type="button" className={outlineButton} onClick={() => setMerchantDemoToken("")}>Sign out</button>}
    </div>
    {children(token)}
  </>;
  if (!merchant && preview === "checking") return <p role="status" className="text-sm text-muted">Opening merchant workspace…</p>;

  return <form className="max-w-lg space-y-3 rounded-xl border border-line bg-white p-4" onSubmit={connect}>
    <label className="block text-sm font-semibold">
      {merchant ? "Merchant access token" : "Demo access token"}
      <input className="control mt-1" type="password" autoComplete="off" required value={credential}
        onChange={event => setCredential(event.currentTarget.value)} disabled={connecting} />
    </label>
    {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
    <button type="submit" className={primaryButton} disabled={connecting || !credential.trim()}>{connecting ? "Connecting…" : "Connect"}</button>
  </form>;
}
