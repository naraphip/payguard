import type { Product } from "../../types";
import type { Assessment, BookingReceipt, SDKStatus } from "../booking-assessment/booking-api";

export const products: Record<Product, string> = {
  flight_ticket: "Flight ticket",
  hotel_room: "Hotel room",
  physical: "Physical goods",
  digital: "Digital goods",
};

export const outcomes = {
  LOW_RISK: "Low risk",
  REVIEW: "Review",
  UNRESOLVED: "Unresolved",
};

export const holdStates = {
  OPEN: "Open",
  CLAIMED: "Claimed",
  ESCALATED: "Escalated",
  RESOLVED: "Resolved",
};

type Outcome = keyof typeof outcomes;
type HoldState = keyof typeof holdStates;

export type Actor = { merchantId: string; actorId: string; role: "merchant" | "analyst" | "viewer" };
export type Hold = {
  state: HoldState;
  dueAt: string;
  assignedActor?: string;
  resolution?: "cleared" | "rejected";
  escalatedAt?: string;
  resolvedAt?: string;
  overdue: boolean;
};
export type Transaction = {
  eventId: string;
  product?: Product;
  receivedAt: string;
  assessedAt?: string;
  outcome?: Outcome;
  amountMinor?: number;
  sdkStatus: SDKStatus | "unknown";
  hold?: Hold;
};
export type Queue = {
  transactions: Transaction[];
  kpis: {
    total: number;
    assessed: number;
    manualReview: number;
    openHolds: number;
    overdueHolds: number;
    averageAssessmentLatencyMs: number | null;
    latestReceivedAt: string | null;
    freshnessSeconds: number | null;
  };
  actor: Actor;
};
export type Detail = {
  booking: BookingReceipt;
  assessment: Assessment | null;
  contact: { state: "MASKED"; email: string } | { state: "UNAVAILABLE" };
  hold?: Hold;
  actor: Actor;
};
export type Reveal = { email: string; auditId: number };
export type Filters = { product: string; outcome: string; hold: string };

const operationsEndpoint = "/api/v1/booking/ops/transactions";

export async function operationsRequest<T>(path: string, token: string, options?: { signal?: AbortSignal; data?: unknown; write?: boolean }) {
  const headers = new Headers({ Authorization: `Bearer ${token}` });
  if (options?.data !== undefined) headers.set("Content-Type", "application/json");
  const response = await fetch(`${operationsEndpoint}${path}`, {
    method: options?.write ? "POST" : "GET",
    headers,
    signal: options?.signal,
    body: options?.data === undefined ? undefined : JSON.stringify(options.data),
    cache: "no-store",
  });
  if (!response.ok) {
    const problem = await response.json().catch(() => null);
    throw new Error(typeof problem?.error === "string" ? problem.error : `Operations API returned HTTP ${response.status}.`);
  }
  return response.json() as Promise<T>;
}
