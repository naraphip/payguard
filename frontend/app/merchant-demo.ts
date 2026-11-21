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
  headers.set("Authorization", `Bearer ${token.trim()}`);
  if (init?.body) headers.set("Content-Type", "application/json");
  const response = await fetch(`/api/v1/booking/demo/merchants${path}`, { ...init, headers, cache: "no-store" });
  const result = await response.json().catch(() => null);
  if (!response.ok || result === null) {
    throw new Error(result?.error ?? `Merchant demo request failed (${response.status}).`);
  }
  return result as T;
}
