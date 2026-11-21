"use client";

import { createContext, useContext, useEffect } from "react";
import type { ReactNode } from "react";

export type ProductionRole = "platform_admin" | "platform_checker" | "merchant";
export type ProductionSession = { role: ProductionRole; merchantId?: string };

const ProductionAuthentication = createContext<ProductionSession | null>(null);

export function ProductionAuthenticationProvider({ session, children }: { session: ProductionSession; children: ReactNode }) {
  useEffect(() => {
    function signIn() {
      window.location.replace("/login");
    }
    async function validate() {
      if (document.visibilityState !== "visible") return;
      try {
        const result = await fetch("/api/auth/session", { cache: "no-store" });
        if (result.status === 401 || result.status === 403) signIn();
      } catch {
        // The API handles session expiry. A transient network error can recover.
      }
    }
    window.addEventListener("payguard-session-expired", signIn);
    window.addEventListener("pageshow", validate);
    document.addEventListener("visibilitychange", validate);
    const timer = window.setInterval(validate, 60000);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", validate);
      window.removeEventListener("pageshow", validate);
      window.removeEventListener("payguard-session-expired", signIn);
    };
  }, []);

  return <ProductionAuthentication.Provider value={session}>{children}</ProductionAuthentication.Provider>;
}

export function useProductionSession() {
  return useContext(ProductionAuthentication);
}

export function productionBrowser() {
  return typeof document !== "undefined" && document.documentElement.dataset.payguardAuth === "oidc";
}

export async function payguardFetch(path: string, init?: RequestInit) {
  const response = await fetch(path, init);
  if (response.status === 401 && productionBrowser()) {
    window.dispatchEvent(new Event("payguard-session-expired"));
  }
  return response;
}
