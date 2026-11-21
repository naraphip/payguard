import "server-only";
import type { NextRequest } from "next/server";
import {
  backendAPI, productionAuthenticationEnabled, productionHostAllowed, productionSession, sandboxHostAllowed,
} from "../production-auth-server";
import { authenticationCookies } from "../production-auth-policy";

const requestHeaders = ["accept", "content-type", "idempotency-key", "origin"];
const responseHeaders = ["content-type", "location", "retry-after", "www-authenticate"];

function errorResponse(status: number, message: string) {
  return Response.json({ error: message }, { status, headers: { "Cache-Control": "no-store" } });
}

export async function forwardBackend(request: NextRequest, path: string, authRoute = false) {
  const production = productionAuthenticationEnabled();
  const allowedHost = production ? productionHostAllowed(request.headers) : sandboxHostAllowed(request.headers);
  if (!allowedHost) return errorResponse(421, "This host is not configured for PayGuard.");
  if (authRoute && !production) return errorResponse(404, "Authentication route not found.");
  if (production && request.headers.has("authorization")) return errorResponse(401, "Sign in to access PayGuard.");
  const headers = new Headers();
  for (const name of requestHeaders) {
    const value = request.headers.get(name);
    if (value !== null) headers.set(name, value);
  }
  const bearer = request.headers.get("authorization");
  if (!production && bearer) headers.set("authorization", bearer);
  const cookies = authenticationCookies(request.headers.get("cookie"));
  if (production && !authRoute) {
    const auth = await productionSession(cookies);
    if (auth.unavailable) return errorResponse(503, "The authentication service is unavailable.");
    if (!auth.session) return errorResponse(401, "Sign in to access PayGuard.");
  }
  if (production && cookies) headers.set("cookie", cookies);
  try {
    const options: RequestInit & { duplex?: "half" } = {
      method: request.method,
      headers,
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(65000)]),
    };
    if (request.body) {
      options.body = request.body;
      options.duplex = "half";
    }
    const upstream = await fetch(backendAPI(path, request.nextUrl.search), options);
    const result = new Headers({ "Cache-Control": "no-store", "Referrer-Policy": "no-referrer" });
    for (const name of responseHeaders) {
      const value = upstream.headers.get(name);
      if (value !== null) result.set(name, value);
    }
    if (authRoute) for (const value of upstream.headers.getSetCookie()) result.append("Set-Cookie", value);
    if (path === "/api/auth/logout" && (upstream.status === 204 || upstream.status === 401)) {
      result.set("Location", "/login");
      return new Response(null, { status: 303, headers: result });
    }
    return new Response(upstream.body, { status: upstream.status, headers: result });
  } catch {
    return errorResponse(502, "The PayGuard API is unavailable.");
  }
}
