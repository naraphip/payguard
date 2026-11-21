import type { NextRequest } from "next/server";
import { forwardBackend } from "../../backend-proxy";

type Context = { params: Promise<{ path: string[] }> };

async function proxy(request: NextRequest, { params }: Context) {
  const { path } = await params;
  const [area, service] = path;
  if (!["booking", "simulation", "enrichment"].includes(area) && !(area === "system" && service === "rules")) {
    return new Response(null, { status: 404 });
  }
  if (path.some(part => !part || part === "." || part === ".." || /[/\\]/.test(part))) {
    return new Response(null, { status: 404 });
  }
  return forwardBackend(request, "/api/v1/" + path.map(encodeURIComponent).join("/"));
}

export { proxy as GET, proxy as POST, proxy as PUT, proxy as DELETE };
