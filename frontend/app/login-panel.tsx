"use client";

import { useEffect, useState } from "react";
import { PayGuardLogo } from "./payguard-logo";
import { primaryButton } from "./settings/system-rules/contracts";

export function LoginPanel({ unavailable = false }: { unavailable?: boolean }) {
  const [denied, setDenied] = useState(false);
  useEffect(() => {
    setDenied(new URLSearchParams(window.location.search).get("error") === "sign_in");
  }, []);
  return <main className="min-h-dvh bg-paper px-5 py-12 sm:grid sm:place-items-center">
    <section className="mx-auto w-full max-w-md space-y-5 rounded-xl border border-line bg-white p-7">
      <PayGuardLogo />
      <h1 className="text-2xl font-semibold">Sign in to PayGuard</h1>
      {denied && <p role="alert" className="text-sm text-red-800">Sign-in could not be completed. Ask your PayGuard administrator to confirm your access.</p>}
      {unavailable
        ? <p role="alert" className="text-sm text-red-800">Sign-in is temporarily unavailable. Try again shortly.</p>
        : <a href="/api/auth/login" className={primaryButton}>Sign in</a>}
    </section>
  </main>;
}
