import assert from "node:assert/strict";
import test from "node:test";
import { containsMailbox, merchantMailboxDigest, parseMailboxBlock, parseScenarioEmail } from "../app/merchant-mailbox.ts";

test("interprets a standalone exact block in English or Thai", () => {
  const expected = { mailbox: "buyer@example.test", durationHours: 12 };
  for (const command of [
    "Block email buyer@Example.TEST 12 h.",
    '"Block email buyer@example.test 12 h."',
    "“Block email buyer@example.test for 12 hours”",
    "บล็อกอีเมล buyer@example.test เป็นเวลา 12 ชั่วโมง",
  ]) {
    assert.deepEqual(parseMailboxBlock(command), expected);
  }
});

test("leaves compound, mismatched and invalid commands out of exact blocking", () => {
  for (const command of [
    '"Block email buyer@example.test for 12 hours" for hotels only',
    "“Block email buyer@example.test for 12 hours and review other bookings”",
    "“Block email buyer@example.test for 12 hours\"",
    "Block email buyer@example.test for 8761 hours",
    "Block email *@example.test for 12 hours",
  ]) {
    assert.equal(parseMailboxBlock(command), null);
  }
  assert.equal(containsMailbox("Review flight bookings by buyer@example.test"), true);
  assert.equal(containsMailbox("Block flight bookings from *@example.test"), false);
});

test("uses matching evidence for the same mailbox and separates other mailboxes and merchants", async () => {
  const command = parseMailboxBlock("Block email buyer+events@EXAMPLE.TEST 12 h.");
  const evidence = parseScenarioEmail("  buyer+events@example.test. ");
  assert.deepEqual(evidence, { domain: "example.test", mailbox: "buyer+events@example.test" });
  assert.deepEqual(parseScenarioEmail("EXAMPLE.TEST"), { domain: "example.test" });
  const merchant = "1A56CC97-ED81-4B6C-939C-51EB08978F4A";
  const reference = await merchantMailboxDigest(merchant, command.mailbox);
  assert.match(reference, /^[0-9a-f]{64}$/);
  assert.equal(await merchantMailboxDigest(merchant.toLowerCase(), evidence.mailbox), reference);
  assert.notEqual(await merchantMailboxDigest(merchant, "buyer@example.test"), reference);
  assert.notEqual(await merchantMailboxDigest("7bd5864e-852b-11ee-b9d1-0242ac120002", evidence.mailbox), reference);
});
