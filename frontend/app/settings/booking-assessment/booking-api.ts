import type { Product } from "../../types";
import type { MerchantSetup } from "../merchant-setup/contracts";

const endpoint = "/api/v1/booking";

export type Proposal = {
  setup: MerchantSetup;
  candidate: { schemaVersion: 1; stage: "booking"; signals: string[] };
};

export type SDKStatus = "observed" | "unavailable";
export type JevMode = "mock" | "timeout";

export type BookingAttempt = {
  event: {
    eventId: string;
    schemaVersion: 1;
    stage: "booking";
    occurredAt: string;
    amountMinor?: number;
    product?: Product;
    sdkStatus?: SDKStatus;
  };
  jevMode: JevMode;
};

export type BookingReceipt = {
  eventId: string;
  receivedAt: string;
  product?: Product;
  sdkStatus?: SDKStatus | "unknown";
};

export type Assessment = {
  eventId: string;
  assessedAt?: string;
  profileVersion: string;
  ruleVersion: string;
  baseline: { state: string; version: string | null; sampleCount?: number };
  features: { name: string; value: number | string | null; presence: string; source: string }[];
  jev: { status: string; answer: string; modelVersion: string; rubricVersion: string; question: string };
  outcome: string;
  handoff: string;
  ruleHits: string[];
};

export async function bookingRequest<T>(path: string, token: string, body: unknown, idempotencyKey?: string): Promise<T> {
  const headers = new Headers({ "Content-Type": "application/json", Authorization: `Bearer ${token}` });
  if (idempotencyKey) headers.set("Idempotency-Key", idempotencyKey);
  const response = await fetch(`${endpoint}${path}`, { method: "POST", headers, body: JSON.stringify(body) });
  if (!response.ok) {
    const problem = await response.json().catch(() => null);
    throw new Error(typeof problem?.error === "string" ? problem.error : `Booking API returned HTTP ${response.status}.`);
  }
  return response.json() as Promise<T>;
}

export async function submitBookingAssessment(token: string, attempt: BookingAttempt) {
  const booking = await bookingRequest<BookingReceipt>("/events", token, attempt.event, attempt.event.eventId);
  const assessment = await bookingRequest<Assessment>(`/events/${booking.eventId}/assessment`, token, { jevMode: attempt.jevMode });
  return { booking, assessment };
}
