import type { DemoMerchant, DemoTransaction } from "../../merchant-demo";

export type MerchantDirectoryItem = DemoMerchant & {
  transactionCount: number;
  underReview: number;
  ruleCount: number;
  lastActivity?: string;
};

export type PageInfo = { total: number; page: number; limit: number };

export type MerchantDirectoryPage = PageInfo & { role: "admin"; merchants: MerchantDirectoryItem[] };
export type MerchantLedgerPage = PageInfo & { transactions: DemoTransaction[] };

export type MerchantProfile = {
  businessName: string;
  businessModel?: string;
  products: string[];
  stages: string[];
  signals: string[];
  scoringProfileVersion?: string;
};

export type MerchantDetails = {
  merchant: DemoMerchant;
  profile: MerchantProfile;
  metrics: {
    windowDays: number;
    totalTransactions: number;
    allowed: number;
    challenged: number;
    underReview: number;
    blocked: number;
    activePolicies: number;
  };
  recentTransactions: DemoTransaction[];
};

export type MerchantAuditEvent = {
  id: string;
  action: string;
  actor: string;
  resourceId: string;
  previousVersion?: number;
  version?: number;
  occurredAt: string;
  reason?: string;
};

export const merchantTabs = {
  overview: "Overview",
  transactions: "Transactions",
  rules: "AI Rule",
  profiles: "Advanced Risk Configuration",
  integrations: "Integrations",
  audit: "Audit Logs",
} as const;

export type MerchantTab = keyof typeof merchantTabs | "platform-drafts";

export function asMerchantTab(value?: string): MerchantTab {
  if (value === "jev") return "rules";
  if (value === "platform") return "platform-drafts";
  if (value === "platform-drafts" || (value && Object.hasOwn(merchantTabs, value))) return value as MerchantTab;
  return "overview";
}

export function merchantDetailsHref(merchant: string, tab: MerchantTab = "overview", transaction?: string) {
  const params = new URLSearchParams();
  if (tab !== "overview") params.set("tab", tab);
  if (transaction) params.set("transaction", transaction);
  const query = params.toString();
  return `/settings/merchants/${encodeURIComponent(merchant)}${query ? `?${query}` : ""}`;
}

const dateTime = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" });
const dateTimeWithSeconds = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "medium" });
export function merchantDate(value?: string, withSeconds = false) {
  if (!value) return "Unavailable";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Unavailable" : (withSeconds ? dateTimeWithSeconds : dateTime).format(date);
}
