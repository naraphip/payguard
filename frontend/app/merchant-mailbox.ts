const mailboxLocalPart = /^[\p{L}\p{M}\p{N}.!#$%&'*+/=?^_`{|}~-]+$/u;
const mailboxInInstruction = /([\p{L}\p{M}\p{N}.!#$%&'*+/=?^_`{|}~-]+)@[\p{L}\p{M}\p{N}-]+(?:\.[\p{L}\p{M}\p{N}-]+)+/gu;
const englishBlock = /^block[ \t]+(?:email(?:[ \t]+address)?[ \t]+)?(\S+)[ \t]+(?:for[ \t]+)?([1-9]\d{0,5})[ \t]*(h(?:rs?|ours?)?|m(?:ins?|inutes?)?)\.?$/iu;
const thaiBlock = /^บล็อก[ \t]*(?:อีเมล[ \t]+)?(\S+)[ \t]+(?:เป็นเวลา[ \t]*)?([1-9]\d{0,5})[ \t]*(ชั่วโมง|ชม|นาที)\.?$/u;
const maxBlockHours = 365 * 24;
const maxBlockMinutes = maxBlockHours * 60;
const exactMailboxPlaceholder = "[EXACT_EMAIL]";
const rulePrice = /(?:\b(?:price|amount|total)\s*(?:is\s+)?(?:at least|at most|greater than|less than|more than|over|under|equals|[<>]=?|!=|=)|(?:ราคา|ยอด(?:เงิน)?)\s*(?:ตั้งแต่|อย่างน้อย|มากกว่า|เกิน|ไม่เกิน|น้อยกว่า|เท่ากับ|[<>]=?|!=|=))\s*(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?(?![\d.,])/giu;
const rulePriceUnit = /^\s*(?:THB\b|baht\b|satang\b|minor units?\b|บาท|สตางค์|หน่วยย่อย)/iu;

type ScenarioEmail = { domain?: string; mailbox?: string };
export type MerchantEmailShape = { length: number; letters: number; digits: number; transitions: number };
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

export function merchantEmailShape(mailbox: string): MerchantEmailShape | undefined {
  const at = mailbox.indexOf("@");
  if (at < 1) return undefined;
  const localPart = mailbox.slice(0, at);
  if (!localPart || localPart.length > 64 || !/^[\x21-\x7e]+$/.test(localPart)) return undefined;

  const shape = { length: localPart.length, letters: 0, digits: 0, transitions: 0 };
  let previousClass = "";
  for (const character of localPart) {
    const characterClass = /[A-Za-z]/.test(character) ? "letters" : /[0-9]/.test(character) ? "digits" : "";
    if (characterClass) {
      shape[characterClass]++;
      if (previousClass && previousClass !== characterClass) shape.transitions++;
    }
    previousClass = characterClass;
  }
  return shape;
}

export function containsMailbox(instruction: string): boolean {
  return Array.from(instruction.matchAll(mailboxInInstruction)).some(([, local]) => local !== "*");
}

export function maskMailboxInstruction(instruction: string) {
  const matches = Array.from(instruction.matchAll(mailboxInInstruction)).filter(([, local]) => local !== "*");
  if (matches.length !== 1 || instruction.includes(exactMailboxPlaceholder)) return null;
  const match = matches[0];
  const mailbox = parseScenarioEmail(match[0])?.mailbox;
  if (!mailbox || match.index === undefined) return null;
  return {
    mailbox,
    instruction: instruction.slice(0, match.index) + exactMailboxPlaceholder + instruction.slice(match.index + match[0].length),
  };
}

function missingPriceUnits(instruction: string) {
  return Array.from(instruction.matchAll(rulePrice))
    .map(match => match.index + match[0].length)
    .filter(end => !rulePriceUnit.test(instruction.slice(end)));
}

export function needsMailboxPriceUnit(instruction: string) {
  return missingPriceUnits(instruction).length > 0;
}

export function applyMailboxPriceUnit(instruction: string, unit: "THB" | "minor units") {
  const missing = missingPriceUnits(instruction);
  if (missing.length !== 1) return null;
  const [end] = missing;
  return instruction.slice(0, end) + " " + unit + instruction.slice(end);
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
