import type { NextRequest } from "next/server";
import { localPreviewAllowed } from "../../../production-auth-server";

type RouteContext = { params: Promise<{ path?: string[] }> };

const uuid = /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/;
const draftId = /^[a-f0-9]{32}$/;
const noStore = { "Cache-Control": "no-store" };

function allowed(method: string, path: string[]) {
  const [merchant, collection, record, action] = path;
  if (path.length === 0) return method === "GET" || method === "POST";
  if (merchant === "admin") {
    const route = path.slice(1).join("/");
    if (route === "system/rules") return method === "GET" || method === "POST";
    if (route === "system/rules/preview") return method === "POST";
    if (collection === "system" && record === "rules" && path.length === 4 && draftId.test(action)) return method === "PUT" || method === "DELETE";
    if (route === "booking/ai-configuration/jev") return method === "GET" || method === "PUT";
    if (["booking/ai-configuration/jev/test", "booking/rule-composer/interpret", "booking/rule-composer/platform/save"].includes(route)) return method === "POST";
    return collection === "booking" && record === "rule-composer" && action === "platform" && path.length === 6 &&
      draftId.test(path[4]) && ((method === "GET" && path[5] === "versions") || (method === "POST" && path[5] === "submit"));
  }
  if (!uuid.test(merchant)) return false;
  if (path.length === 1) return ["GET", "PUT", "DELETE"].includes(method);
  if (path.length === 2) {
    if (collection === "policies" || collection === "transactions") return method === "GET" || method === "POST";
    if (collection === "access-token" || collection === "policy-interpret") return method === "POST";
    return ["audit", "test-context", "policy-profile"].includes(collection) && method === "GET";
  }
  if ((collection === "jev" || collection === "platform") && record === "rules") {
    if (path.length === 3) return method === "GET" || method === "POST";
    if (path.length !== 4) return false;
    if (action === "preview") return method === "POST";
    const validRule = collection === "jev" ? /^jev-rule-[0-9a-f]{64}$/.test(action) : draftId.test(action);
    return validRule && (method === "PUT" || method === "DELETE");
  }
  if (!uuid.test(record)) return false;
  if (collection === "policies") {
    if (path.length === 3) return ["GET", "PUT", "DELETE"].includes(method);
    return path.length === 4 && method === "POST" && ["activate", "disable", "rollback", "simulate"].includes(action);
  }
  return collection === "transactions" && ((path.length === 3 && method === "GET") ||
    (path.length === 4 && method === "POST" && action === "decision"));
}

async function proxy(request: NextRequest, { params }: RouteContext) {
  const token = process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim();
  const host = request.headers.get("host") ?? "";
  if (!token || !localPreviewAllowed(request.headers, request.nextUrl.host)) {
    return Response.json({ code: "MERCHANT_PREVIEW_DISABLED", error: "Local merchant preview is unavailable." }, { status: 404, headers: noStore });
  }

  const path = (await params).path ?? [];
  if (request.headers.has("authorization")) {
    return Response.json({ error: "Use the authenticated API for bearer credentials." }, { status: 403, headers: noStore });
  }
  if (!allowed(request.method, path)) {
    return Response.json({ error: "Merchant preview route not found." }, { status: 404, headers: noStore });
  }

  const write = request.method !== "GET";
  if (write && request.headers.get("origin") !== `${request.nextUrl.protocol}//${host}`) {
    return Response.json({ error: "Merchant preview changes require a same-origin request." }, { status: 403, headers: noStore });
  }

  const backend = process.env.PAYGUARD_BOOKING_API_URL ?? process.env.PAYGUARD_API_URL ?? "http://127.0.0.1:8081";
  const resource = path[0] === "admin" ? path.slice(1) : ["booking", "demo", "merchants", ...path];
  const endpoint = `${backend.replace(/\/$/, "")}/api/v1/${resource.join("/")}${write ? "" : request.nextUrl.search}`;
  const timeout = AbortSignal.timeout(65_000);
  let response: Response;
  let body: string;
  try {
    const headers = new Headers({ Authorization: `Bearer ${token}`, "Content-Type": "application/json" });
    const creationKey = path.length === 0 && request.method === "POST" ? request.headers.get("Idempotency-Key") : null;
    if (creationKey) headers.set("Idempotency-Key", creationKey);
    response = await fetch(endpoint, {
      method: request.method,
      headers,
      body: write ? await request.text() : undefined,
      cache: "no-store",
      redirect: "error",
      signal: AbortSignal.any([request.signal, timeout]),
    });
    body = await response.text();
  } catch {
    return timeout.aborted
      ? Response.json({ code: "MERCHANT_API_TIMEOUT", error: "The local PayGuard backend did not respond in time. Retry the request." }, { status: 504, headers: noStore })
      : Response.json({ code: "MERCHANT_API_UNREACHABLE", error: "The preview could not connect to the local PayGuard backend. Check that the backend is running, then retry." }, { status: 502, headers: noStore });
  }

  let data: unknown;
  try {
    data = JSON.parse(body);
  } catch {
    data = null;
  }
  if (data === null) {
    if (response.status === 404 || response.status === 405) {
      return Response.json({
        code: "MERCHANT_API_ROUTE_UNAVAILABLE",
        error: "The local PayGuard backend does not have this API route. Restart it with the current build, then retry.",
      }, { status: response.status, headers: noStore });
    }
    return Response.json({
      code: "MERCHANT_API_INVALID_RESPONSE",
      error: `The local PayGuard backend returned an unreadable response (HTTP ${response.status}). Check the backend logs, then retry.`,
    }, { status: response.ok ? 502 : response.status, headers: noStore });
  }
  return Response.json(data, { status: response.status, headers: noStore });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const DELETE = proxy;
