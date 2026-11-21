"use client";

import { useEffect, useId, useRef, useState } from "react";
import { merchantDemoRequest } from "../../merchant-demo";
import { outlineButton, primaryButton } from "../system-rules/contracts";
import { managementError } from "./management-access";
import { merchantDate } from "./management-contracts";

export function MerchantTokenControls({ token }: { token: string }) {
  const [visible, setVisible] = useState(false);
  const [copyState, setCopyState] = useState<"idle" | "copied" | "manual">("idle");
  const field = useRef<HTMLInputElement>(null);
  const fieldId = useId();

  useEffect(() => {
    if (copyState === "manual" && visible) field.current?.focus();
  }, [copyState, visible]);

  async function copyToken() {
    try {
      await navigator.clipboard.writeText(token);
      setCopyState("copied");
    } catch {
      setVisible(true);
      setCopyState("manual");
      field.current?.focus();
      field.current?.select();
    }
  }

  return <div className="space-y-2">
    <div id={fieldId}>
      {visible
        ? <input ref={field} aria-label="Merchant token" className="control font-mono text-sm" value={token} readOnly
            autoComplete="off" spellCheck={false} onFocus={event => event.currentTarget.select()} />
        : <div className="rounded-md bg-paper p-3 font-mono text-sm" aria-label="Token hidden">••••••••••••••••</div>}
    </div>
    <div className="flex flex-wrap items-center gap-2">
      <button type="button" className={outlineButton} aria-controls={fieldId} aria-expanded={visible} onClick={() => {
        setVisible(value => !value);
        setCopyState("idle");
      }}>{visible ? "Hide token" : "Show token"}</button>
      <button type="button" className={outlineButton} onClick={() => void copyToken()}>Copy token</button>
      <p role="status" className="text-xs text-muted">
        {copyState === "copied" ? "Token copied." : copyState === "manual" ? "Clipboard unavailable. Copy the selected token." : ""}
      </p>
    </div>
  </div>;
}

type TemporaryToken = { token: string; expiresAt: string };

export function MerchantAccessToken({ merchantId, adminToken }: { merchantId: string; adminToken: string }) {
  const headingId = useId();
  const [issued, setIssued] = useState<TemporaryToken>();
  const [pending, setPending] = useState(false);
  const [expired, setExpired] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<AbortController>(null);
  const expiresAt = issued?.expiresAt;

  useEffect(() => () => request.current?.abort(), []);

  useEffect(() => {
    if (!expiresAt) return;
    const timer = window.setTimeout(() => {
      setExpired(true);
      setIssued(undefined);
    }, Date.parse(expiresAt) - Date.now());
    return () => window.clearTimeout(timer);
  }, [expiresAt]);

  async function issueToken() {
    if (request.current) return;
    const controller = new AbortController();
    request.current = controller;
    setPending(true);
    setError("");
    try {
      const result = await merchantDemoRequest<TemporaryToken>(`/${encodeURIComponent(merchantId)}/access-token`, adminToken, {
        method: "POST", signal: controller.signal,
      });
      if (!controller.signal.aborted) {
        setExpired(false);
        setIssued(result);
      }
    } catch (cause) {
      if (!controller.signal.aborted) setError(managementError(cause));
    } finally {
      if (!controller.signal.aborted) setPending(false);
      request.current = null;
    }
  }

  return <section className="min-w-0 space-y-3 rounded-xl border border-line bg-white p-4 sm:p-5" aria-labelledby={headingId}>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="space-y-1">
        <h2 id={headingId} className="text-lg font-semibold">Merchant access token</h2>
        <p className="text-sm text-muted">Use a temporary token to connect as this merchant. It is valid for 1 hour.</p>
      </div>
      {!issued && <button type="button" className={primaryButton} disabled={pending} onClick={() => void issueToken()}>
        {pending ? "Creating token…" : "Get temporary token"}
      </button>}
    </div>
    {issued && <>
      <p role="status" className="text-sm text-muted">Expires <time dateTime={issued.expiresAt}>{merchantDate(issued.expiresAt)}</time></p>
      <MerchantTokenControls token={issued.token} />
    </>}
    {expired && <p role="status" className="text-sm text-muted">This token expired. Get a new token to reconnect.</p>}
    {error && <p role="alert" className="rounded-md bg-red-50 p-2 text-sm text-red-800">{error}</p>}
  </section>;
}
