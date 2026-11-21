export type DemoScenario = {
  id: string;
  name: string;
  thresholdMinor: number;
  scenarioVersion: number;
  ruleVersion: string;
};

export type DemoMerchant = {
  id: string;
  name: string;
  scenarios: DemoScenario[];
};

export type DemoDirectory = {
  merchants: DemoMerchant[];
  role: "admin" | "merchant";
};

export type DemoTransaction = {
  id: string;
  merchantId: string;
  scenarioId: string;
  scenarioName: string;
  scenarioVersion: number;
  ruleVersion: string;
  source: "checkout" | "simulation";
  product?: string;
  amountMinor: number | null;
  thresholdMinor: number;
  jevStatus: "MOCKED" | "TIMEOUT";
  jevVersion: "mock-jev-v1";
  jevRuleVersion?: string;
  outcome: "LOW_RISK" | "REVIEW" | "UNRESOLVED";
  reason: string;
  ruleHits: string[];
  reviewState: "NONE" | "NEEDS_DECISION" | "RESOLVED";
  createdAt: string;
  disposition?: "cleared" | "rejected";
  decidedBy?: string;
  decidedAt?: string;
};

let sessionToken = "";

export function getMerchantDemoToken() {
  return sessionToken;
}

export function setMerchantDemoToken(value: string) {
  sessionToken = value.trim();
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
    throw new Error(result?.error ?? `Merchant demo request failed (${response.status}).`);
  }
  return result as T;
}

class LocalMerchantPreviewDisabled extends Error {}

export async function loadLocalMerchantDirectory(): Promise<DemoDirectory | null> {
  try {
    return await merchantDemoRequest<DemoDirectory>("", "");
  } catch (error) {
    if (error instanceof LocalMerchantPreviewDisabled) return null;
    throw error;
  }
}
