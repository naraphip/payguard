const mailboxLocalPart = /^[\p{L}\p{M}\p{N}.!#$%&'*+/=?^_`{|}~-]+$/u;
const mailboxInInstruction = /([\p{L}\p{M}\p{N}.!#$%&'*+/=?^_`{|}~-]+)@[\p{L}\p{M}\p{N}-]+(?:\.[\p{L}\p{M}\p{N}-]+)+/gu;
const englishBlock = /^block[ \t]+(?:email(?:[ \t]+address)?[ \t]+)?(\S+)[ \t]+(?:for[ \t]+)?([1-9]\d{0,5})[ \t]*(h(?:rs?|ours?)?|m(?:ins?|inutes?)?)\.?$/iu;
const thaiBlock = /^บล็อก[ \t]*(?:อีเมล[ \t]+)?(\S+)[ \t]+(?:เป็นเวลา[ \t]*)?([1-9]\d{0,5})[ \t]*(ชั่วโมง|ชม|นาที)\.?$/u;
const maxBlockHours = 365 * 24;
const maxBlockMinutes = maxBlockHours * 60;

type ScenarioEmail = { domain?: string; mailbox?: string };
export type MailboxBlockDuration =
  | { durationMinutes: number; durationHours?: never }
  | { durationHours: number; durationMinutes?: never };

export function parseScenarioEmail(value?: string): ScenarioEmail | null {
  const entered = value?.trim();
  if (!entered) return {};
  const parts = entered.split("@");
  if (parts.length > 2 || !parts[0] || /\s/u.test(entered)) return null;
  const host = parts[parts.length - 1];
  if (!host.includes(".") || host.length > 254 || !/^[\p{L}\p{M}\p{N}.-]+$/u.test(host)) return null;
  let domain: string;
  try {
    domain = new URL(`http://${host}`).hostname.toLowerCase().replace(/\.$/, "");
  } catch {
    return null;
  }
  if (!domain.includes(".") || domain.length > 253 || /^\d+(?:\.\d+){3}$/.test(domain) ||
      domain.split(".").some(label => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))) return null;
  if (parts.length === 1) return { domain };
  const local = parts[0];
  if (local.length > 64 || local === "*" || /^\.|\.\.|\.$/.test(local) || !mailboxLocalPart.test(local)) return null;
  return { domain, mailbox: `${local}@${domain}` };
}

export function containsMailbox(instruction: string): boolean {
  return Array.from(instruction.matchAll(mailboxInInstruction)).some(([, local]) => local !== "*");
}

export function parseMailboxBlock(instruction: string): ({ mailbox: string } & MailboxBlockDuration) | null {
  let command = instruction.trim();
  if ((command.startsWith('"') && command.endsWith('"')) || (command.startsWith("“") && command.endsWith("”"))) {
    command = command.slice(1, -1).trim();
  }
  const match = englishBlock.exec(command) ?? thaiBlock.exec(command);
  if (!match) return null;
  const mailbox = parseScenarioEmail(match[1])?.mailbox;
  if (!mailbox) return null;
  const duration = Number(match[2]);
  const unit = match[3].toLowerCase();
  if (unit.startsWith("m") || unit === "นาที") {
    return duration <= maxBlockMinutes ? { mailbox, durationMinutes: duration } : null;
  }
  return duration <= maxBlockHours ? { mailbox, durationHours: duration } : null;
}

export async function merchantMailboxDigest(merchantId: string, mailbox: string) {
  if (!globalThis.crypto?.subtle) throw new Error("Open PayGuard on HTTPS or localhost to use an exact email address.");
  const canonical = new TextEncoder().encode(`payguard-mailbox-v1:${merchantId.toLowerCase()}:${mailbox}`);
  const digest = await crypto.subtle.digest("SHA-256", canonical);
  return Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, "0")).join("");
}
