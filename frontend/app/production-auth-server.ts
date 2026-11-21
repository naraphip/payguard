import "server-only";
import type { ProductionSession } from "./production-auth";
import { authenticationCookies, loopbackHostRequest, loopbackPreviewRequest, productionOriginMatches } from "./production-auth-policy";

export function productionAuthenticationEnabled() {
  const mode = process.env.PAYGUARD_AUTH_MODE || "sandbox";
  if (mode !== "sandbox" && mode !== "oidc") throw new Error("Invalid PayGuard authentication mode.");
  return mode === "oidc";
}

export function backendAPI(path: string, query = "") {
  const fallback = process.env.PAYGUARD_API_URL?.trim() || "http://127.0.0.1:8081";
  const configured = !productionAuthenticationEnabled() && path.startsWith("/api/v1/booking/")
    ? process.env.PAYGUARD_BOOKING_API_URL?.trim() || fallback : fallback;
  const base = new URL(/^https?:\/\//i.test(configured) ? configured : `https://${configured}`);
  if (base.username || base.password || base.search || base.hash) throw new Error("Invalid PayGuard API origin.");
  return `${base.href.replace(/\/$/, "")}${path}${query}`;
}

export function sandboxHostAllowed(headers: Pick<Headers, "get" | "has">) {
  return process.env.PAYGUARD_AUTH_MODE === "sandbox" || loopbackHostRequest(headers);
}

export function localPreviewAllowed(headers: Pick<Headers, "get" | "has">, nextHost?: string) {
  return !productionAuthenticationEnabled() && Boolean(process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim()) &&
    loopbackPreviewRequest(headers, nextHost);
}

export function productionHostAllowed(headers: Pick<Headers, "get">) {
  return productionOriginMatches(headers, process.env.PAYGUARD_PUBLIC_ORIGIN ?? "");
}

export type SessionState = { session: ProductionSession | null; unavailable: boolean };

export async function productionSession(cookie: string | null): Promise<SessionState> {
  const authCookies = authenticationCookies(cookie);
  if (!authCookies) return { session: null, unavailable: false };
  try {
    const response = await fetch(backendAPI("/api/auth/session"), {
      headers: { Cookie: authCookies, Accept: "application/json" },
      cache: "no-store",
      redirect: "error",
      signal: AbortSignal.timeout(5000),
    });
    if (response.status === 401 || response.status === 403) return { session: null, unavailable: false };
    if (!response.ok) return { session: null, unavailable: true };
    const actor = await response.json() as Partial<ProductionSession>;
    if (actor.role === "platform_admin" || actor.role === "platform_checker") {
      return { session: { role: actor.role }, unavailable: false };
    }
    if (actor.role === "merchant" && typeof actor.merchantId === "string" &&
        /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(actor.merchantId)) {
      return { session: { role: actor.role, merchantId: actor.merchantId }, unavailable: false };
    }
  } catch {
    return { session: null, unavailable: true };
  }
  return { session: null, unavailable: true };
}
