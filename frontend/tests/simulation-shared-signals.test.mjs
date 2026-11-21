import assert from "node:assert/strict";
import test from "node:test";
import { findSharedSignals } from "../app/simulation-shared-signals.ts";

function decision(id, at, account, ...signals) {
  return { event: { id, at, entities: [{ kind: "account", key: account }, ...signals] } };
}

const ip = { kind: "ip", key: "shared-ip" };
const device = { kind: "device", key: "shared-device" };
const token = { kind: "token", key: "shared-token" };

test("uses the engine's event order and includes earlier evidence outside the 60-second counter", () => {
  const earlier = decision("event-a", 5, "first", ip);
  const tieBefore = decision("event-b", 100, "second", ip);
  const selected = decision("event-c", 100, "selected", ip);
  const tieAfter = decision("event-d", 100, "third", ip);
  const later = decision("event-e", 101, "fourth", ip);

  assert.deepEqual(findSharedSignals(selected.event, [later, tieAfter, earlier, selected, tieBefore]), [
    { kind: "ip", eventIds: ["event-b", "event-a"] },
  ]);
});

test("shows the latest matching event for each other account and ignores retries", () => {
  const checkout = decision("checkout", 10, "peer", ip, device);
  const payment = decision("payment", 11, "peer", ip, token);
  const ownCheckout = decision("own-checkout", 12, "selected", ip, device, token);
  const tieToken = decision("other-token", 12, "another-peer", token);
  const selected = decision("selected", 13, "selected", ip, device, token);

  assert.deepEqual(findSharedSignals(selected.event, [checkout, payment, ownCheckout, tieToken, payment, selected]), [
    { kind: "device", eventIds: ["checkout"] },
    { kind: "ip", eventIds: ["payment"] },
    { kind: "token", eventIds: ["other-token", "payment"] },
  ]);
});

test("matches the signal type as well as its key and uses only the current run", () => {
  const selected = decision("selected", 10, "selected", ip, device);
  const peer = decision("peer", 8, "peer", ip);
  const wrongType = decision("other-kind", 8, "other-kind", { kind: "device", key: ip.key });
  const differentKey = decision("other-key", 8, "other-key", { kind: "ip", key: "different-ip" });

  assert.deepEqual(findSharedSignals(selected.event, [peer, selected]), [{ kind: "ip", eventIds: ["peer"] }]);
  assert.deepEqual(findSharedSignals(selected.event, [wrongType, differentKey, selected]), []);
});
