import type { NextRequest } from "next/server";
import { forwardBackend } from "../../backend-proxy";

type Context = { params: Promise<{ action: string }> };

export async function GET(request: NextRequest, { params }: Context) {
  const { action } = await params;
  if (action === "login" || action === "callback" || action === "session") {
    return forwardBackend(request, `/api/auth/${action}`, true);
  }
  return new Response(null, { status: 404 });
}

export async function POST(request: NextRequest, { params }: Context) {
  if ((await params).action === "logout") return forwardBackend(request, "/api/auth/logout", true);
  return new Response(null, { status: 404 });
}
