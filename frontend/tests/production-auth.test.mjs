import assert from "node:assert/strict";
import test from "node:test";
import { authenticationCookies, loopbackHostRequest, loopbackPreviewRequest, productionOriginMatches } from "../app/production-auth-policy.ts";

test("production requests match a configured HTTPS origin and forwarding identifies the same host", () => {
  const origin = "https://payguard.example.test";
  assert.equal(productionOriginMatches(new Headers({ host: "payguard.example.test" }), origin), true);
  assert.equal(productionOriginMatches(new Headers({ host: "payguard.example.test", "x-forwarded-host": "payguard.example.test" }), origin), true);
  assert.equal(productionOriginMatches(new Headers({ host: "localhost:3000" }), origin), false);
  assert.equal(productionOriginMatches(new Headers({ host: "payguard.example.test" }), "http://payguard.example.test"), false);
});

test("local preview accepts a loopback connection and is unavailable through a public host or proxy", () => {
  const host = "127.0.0.1:3000";
  assert.equal(loopbackPreviewRequest(new Headers({ host, "x-forwarded-for": "127.0.0.1" }), "localhost:3000"), true);
  assert.equal(loopbackPreviewRequest(new Headers({ host: "payguard.example.test" })), false);
  assert.equal(loopbackPreviewRequest(new Headers({ host, "x-forwarded-for": "192.0.2.15" })), false);
  assert.equal(loopbackPreviewRequest(new Headers({ host, "cf-ray": "fixture" })), false);
});

test("ordinary sandbox requests can be delivered to a loopback port through a Docker bridge", () => {
  const headers = new Headers({ host: "127.0.0.1:3000", "x-forwarded-for": "172.18.0.1" });
  assert.equal(loopbackHostRequest(headers), true);
  assert.equal(loopbackPreviewRequest(headers), false);
});

test("backend transport forwards only the PayGuard authentication cookies", () => {
  const cookie = "__Host-payguard_session=opaque; __Host-payguard_oidc=flow";
  assert.equal(authenticationCookies(`theme=light; ${cookie}; appCookie=other-app`), cookie);
  assert.equal(authenticationCookies(null), "");
});
