import type { MerchantSetup } from "./settings/merchant-setup/contracts";
import type { CustomSignalDefinition } from "./settings/merchants/custom-signal-setup";

export type DemoMerchant = {
  id: string;
  name: string;
  description?: string;
  createdAt?: string;
  setup?: MerchantSetup;
  policyIntegration?: { products: string[]; emailDomain: boolean; amountMinor?: boolean; customFields?: CustomSignalDefinition[] };
};

export type DemoDirectory = {
  merchants: DemoMerchant[];
  role: "admin" | "merchant";
};

export type DemoTransaction = {
  id: string;
  merchantId: string;
  scenarioId?: string;
  scenarioName?: string;
  scenarioVersion?: number;
  ruleVersion: string;
  source: "checkout" | "simulation";
  product?: string;
  amountMinor: number | null;
  thresholdMinor?: number;
  jevStatus: "MOCKED" | "TIMEOUT" | "LIVE" | "UNAVAILABLE" | "NOT_RUN";
  jevVersion: string;
  jevAnswer?: "LOW_RISK" | "REVIEW";
  jevReason?: string;
  jevDecisionId?: string;
  jevConfidence?: number;
  jevConfigurationVersion?: number;
  jevRuleVersion?: string;
  outcome: "LOW_RISK" | "HIGH_RISK" | "REVIEW" | "UNRESOLVED";
  reason: string;
  ruleHits: string[];
  reviewState: "NONE" | "NEEDS_DECISION" | "RESOLVED";
  createdAt: string;
  disposition?: "cleared" | "rejected";
  decidedBy?: string;
  decidedAt?: string;
  policyDecision?: MerchantPolicyDecision;
};

type MerchantPolicyDecisionPart = {
  action: string;
  matchedRuleIds?: string[];
  reasonCodes?: string[];
  ruleVersions?: Record<string, number>;
  missingSignals?: string[];
  missingRuleIds?: string[];
};

export type MerchantPolicyDecision = {
  platformDecision: MerchantPolicyDecisionPart;
  merchantPolicyDecision: MerchantPolicyDecisionPart;
  jevDecision?: MerchantPolicyDecisionPart;
  finalDecision: { action: string; source: string };
};

export const merchantPolicyProducts = {
  flight_ticket: "Flight ticket",
  hotel_room: "Hotel room",
  physical: "Physical goods",
  digital: "Digital goods",
  payment_gateway: "Gateway payment",
} as const;

export const policyActionLabels: Record<string, string> = {
  BLOCK: "Block", ALLOW: "Allow", REVIEW: "Review", CHALLENGE: "Challenge", NOT_APPLICABLE: "No match",
  UNSUCCESSFUL: "Unresolved", UNRESOLVED: "Unresolved", PASS: "Pass", UNAVAILABLE: "Signal unavailable",
};

export const merchantOutcomeLabels = { LOW_RISK: "Low risk", HIGH_RISK: "High risk", REVIEW: "Review", UNRESOLVED: "Review (unresolved)" } as const;
const jevStatuses = {
  MOCKED: "Mock Jev",
  TIMEOUT: "Jev timed out",
  LIVE: "Live Jev",
  UNAVAILABLE: "Jev unavailable",
  NOT_RUN: "Jev not run",
} satisfies Record<DemoTransaction["jevStatus"], string>;

export function merchantJevStatus(transaction: DemoTransaction) {
  if (transaction.jevStatus === "TIMEOUT" && transaction.jevVersion === "mock-jev-v1") return "Simulated Jev timeout";
  return jevStatuses[transaction.jevStatus];
}

let sessionToken = "";
let sessionRole: DemoDirectory["role"] | null = null;
const sessionEvent = "payguard-merchant-demo-session-change";

export function getMerchantDemoToken() {
  return sessionToken;
}

export function getMerchantDemoRole() {
  return sessionRole;
}

export function subscribeMerchantDemoSession(listener: () => void) {
  window.addEventListener(sessionEvent, listener);
  return () => window.removeEventListener(sessionEvent, listener);
}

export function setMerchantDemoToken(value: string, role: DemoDirectory["role"] | null = null) {
  const token = value.trim();
  const nextRole = token ? role : null;
  if (sessionToken === token && sessionRole === nextRole) return;
  sessionToken = token;
  sessionRole = nextRole;
  window.dispatchEvent(new Event(sessionEvent));
}

export async function merchantDemoRequest<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  const bearer = token.trim();
  if (bearer) headers.set("Authorization", `Bearer ${bearer}`);
  if (init?.body) headers.set("Content-Type", "application/json");
  const base = bearer ? "/api/v1/booking/demo/merchants" : "/api/merchant-preview";
  const response = await fetch(`${base}${path}`, { ...init, headers, cache: "no-store" });
  const result = await response.json().catch(() => null);
  if (!bearer && response.status === 404 && result?.code === "MERCHANT_PREVIEW_DISABLED") {
    throw new LocalMerchantPreviewDisabled();
  }
  if (!response.ok || result === null) {
    throw new MerchantDemoError(response.status, result?.error ?? `Merchant demo request failed (${response.status}).`);
  }
  return result as T;
}

export class MerchantDemoError extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
  }
}

export class LocalMerchantPreviewDisabled extends Error {}

export async function loadLocalMerchantDirectory(): Promise<DemoDirectory | null> {
  try {
    return await merchantDemoRequest<DemoDirectory>("", "");
  } catch (error) {
    if (error instanceof LocalMerchantPreviewDisabled) return null;
    throw error;
  }
}
