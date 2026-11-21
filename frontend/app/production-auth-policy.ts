const loopbackHost = /^(?:127\.0\.0\.1|localhost|\[::1\])(?::\d+)?$/i;
const loopbackAddress = /^(?:127\.0\.0\.1|::(?:ffff:127\.0\.0\.1|1))$/i;

type RequestHeaders = Pick<Headers, "get" | "has">;

export function loopbackHostRequest(headers: RequestHeaders, nextHost?: string) {
  const host = headers.get("host");
  if (!host || !loopbackHost.test(host) || (nextHost !== undefined && !loopbackHost.test(nextHost))) return false;
  if (headers.has("forwarded") || headers.has("cf-connecting-ip") || headers.has("cf-ray")) return false;
  const forwardedHost = headers.get("x-forwarded-host");
  return !forwardedHost || forwardedHost === host;
}

export function loopbackPreviewRequest(headers: RequestHeaders, nextHost?: string) {
  return loopbackHostRequest(headers, nextHost) && ["x-forwarded-for", "x-real-ip"].every(name => {
    const addresses = headers.get(name)?.split(",");
    return !addresses || addresses.every(address => loopbackAddress.test(address.trim()));
  });
}

export function productionOriginMatches(headers: Pick<Headers, "get">, publicOrigin: string) {
  try {
    const origin = new URL(publicOrigin);
    const host = headers.get("host");
    const forwardedHost = headers.get("x-forwarded-host");
    return origin.protocol === "https:" && origin.href === origin.origin + "/" &&
      host === origin.host && (!forwardedHost || forwardedHost === host);
  } catch {
    return false;
  }
}

export function authenticationCookies(header: string | null) {
  return (header ?? "").split(";").map(cookie => cookie.trim()).filter(cookie =>
    cookie.startsWith("__Host-payguard_session=") || cookie.startsWith("__Host-payguard_oidc="),
  ).join("; ");
}
