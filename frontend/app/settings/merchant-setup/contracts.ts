export const merchantSetupStorageKey = "payguard:merchant-setup";

export type BookingSignal = "occurredAt" | "amountMinor";
export type BusinessModel = "ota" | "ecommerce";

export type MerchantSetup = {
  schemaVersion: 1;
  businessModel: BusinessModel;
  stage: "booking";
  availableSignals: BookingSignal[];
};

export function isBusinessModel(value: unknown): value is BusinessModel {
  return value === "ota" || value === "ecommerce";
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function validBookingSignals(value: unknown): value is BookingSignal[] {
  return Array.isArray(value) && value.includes("occurredAt") &&
    value.every(signal => signal === "occurredAt" || signal === "amountMinor") &&
    value.length === new Set(value).size;
}

export function isMerchantSetup(value: unknown): value is MerchantSetup {
  return isRecord(value) && value.schemaVersion === 1 && isBusinessModel(value.businessModel) &&
    value.stage === "booking" && validBookingSignals(value.availableSignals);
}
