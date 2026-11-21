export const rulesEndpoint = "/api/v1/system/rules";

export const stages = {
  ANY: "All stages",
  BOOKING: "Booking",
  PRE_PAYMENT: "Pre-payment",
  CHECKOUT: "Checkout",
  PAYMENT: "Payment",
  PAYMENT_STAGES: "Payment stages",
};

export const actions = { REVIEW: "Review", CHALLENGE: "Challenge" };

export const operators = {
  gte: "≥ At least",
  gt: "> Greater than",
  lte: "≤ At most",
  lt: "< Less than",
  eq: "= Equals",
  ne: "≠ Does not equal",
};

export type Stage = keyof typeof stages;
export type Action = keyof typeof actions;
export type Operator = keyof typeof operators;

export type Condition = {
  signal: string;
  operator: Operator;
  value: number | boolean;
};

export type RuleInput = {
  name: string;
  description: string;
  stage: Stage;
  action: Action;
  conditions: Condition[];
};

export type Rule = RuleInput & {
  id: string;
  status: "ACTIVE" | "DRAFT";
  version: number;
  updatedAt?: string;
  code?: string;
};

export type Signal = {
  id: string;
  label: string;
  type: "number" | "boolean";
  stages: Stage[];
};

export type PreviewRun = {
  id: string;
  seed: number;
  product: string;
  scenario: string;
  createdAt: string;
};

export type RuleCatalogue = {
  rules: Rule[];
  signals: Signal[];
  runs: PreviewRun[];
  ruleVersion: string;
};

export type Preview = {
  runId: string;
  ruleVersion: string;
  eligible: number;
  matched: number;
  skipped: number;
  matchedEventIds: string[];
};

export async function ruleRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  const body = await response.json().catch(() => null);
  if (!response.ok || body === null) {
    throw new Error(body?.error ?? "The rules service is unavailable. Try again.");
  }
  return body as T;
}

export const primaryButton = "inline-flex h-9 items-center justify-center gap-2 rounded-lg bg-indigo-600 px-3.5 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-55";
export const outlineButton = "inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 shadow-xs transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-55";
export const fieldControl = "h-9 min-w-0 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-900 shadow-xs disabled:bg-slate-50";

export function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeZone: "UTC" }).format(new Date(value));
}

export type RuleTransport = <T>(suffix: string, init?: RequestInit) => Promise<T>;

export const globalRuleTransport: RuleTransport = (suffix, init) =>
  ruleRequest(rulesEndpoint + suffix, init);
