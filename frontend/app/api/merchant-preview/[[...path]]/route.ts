import type { NextRequest } from "next/server";

type RouteContext = { params: Promise<{ path?: string[] }> };

const loopback = /^(?:127\.0\.0\.1|localhost|\[::1\])(?::\d+)?$/;
const uuid = /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/;
const noStore = { "Cache-Control": "no-store" };

function allowed(method: string, path: string[]) {
  const [merchant, collection, record, action] = path;
  if (path.length === 0) return method === "GET" || method === "POST";
  if (!uuid.test(merchant)) return false;
  if (path.length === 1) return method === "GET" || method === "PUT" || method === "DELETE";
  if (path.length === 2) {
    return (method === "POST" && collection === "scenarios")
      || (method === "GET" && (collection === "policies" || collection === "audit"))
      || (collection === "transactions" && (method === "GET" || method === "POST"));
  }
  if ((collection === "jev" || collection === "platform") && record === "rules") {
    return path.length === 3 && method === "GET";
  }
  if (!uuid.test(record)) return false;
  if (path.length === 3) {
    return (method === "PUT" && collection === "scenarios")
      || (method === "GET" && (collection === "transactions" || collection === "policies"));
  }
  return path.length === 4 && method === "POST" && collection === "transactions" && action === "decision";
}

async function proxy(request: NextRequest, { params }: RouteContext) {
  const token = process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim();
  const host = request.headers.get("host") ?? "";
  if (!token || !loopback.test(host) || !loopback.test(request.nextUrl.host)) {
    return Response.json({ code: "MERCHANT_PREVIEW_DISABLED", error: "Local merchant preview is unavailable." }, { status: 404, headers: noStore });
  }

  const path = (await params).path ?? [];
  if (request.method !== "GET" && uuid.test(path[0]) && (path[1] === "jev" || path[1] === "platform") && path[2] === "rules") {
    return Response.json({ error: "Connect with a platform administrator token to write or preview rules." }, { status: 403, headers: noStore });
  }
  if (!allowed(request.method, path)) {
    return Response.json({ error: "Merchant preview route not found." }, { status: 404, headers: noStore });
  }

  const write = request.method !== "GET";
  if (write && request.headers.get("origin") !== `${request.nextUrl.protocol}//${host}`) {
    return Response.json({ error: "Merchant preview changes require a same-origin request." }, { status: 403, headers: noStore });
  }

  const backend = process.env.PAYGUARD_BOOKING_API_URL ?? process.env.PAYGUARD_API_URL ?? "http://127.0.0.1:8081";
  const endpoint = `${backend.replace(/\/$/, "")}/api/v1/booking/demo/merchants${path.length ? `/${path.join("/")}` : ""}${write ? "" : request.nextUrl.search}`;
  try {
    const headers = new Headers({ Authorization: `Bearer ${token}`, "Content-Type": "application/json" });
    const creationKey = path.length === 0 && request.method === "POST" ? request.headers.get("Idempotency-Key") : null;
    if (creationKey) headers.set("Idempotency-Key", creationKey);
    const response = await fetch(endpoint, {
      method: request.method,
      headers,
      body: write ? await request.text() : undefined,
      cache: "no-store",
      redirect: "error",
      signal: request.signal,
    });
    const data = await response.json();
    return Response.json(data, { status: response.status, headers: noStore });
  } catch {
    return Response.json({ error: "The local merchant API is unavailable. Try again." }, { status: 502, headers: noStore });
  }
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const DELETE = proxy;
