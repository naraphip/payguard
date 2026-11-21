import type { Metadata } from "next";
import { AppShell } from "./app-shell";
import "./globals.css";

export const metadata: Metadata = {
  title: "PayGuard | Fraud Management",
  description: "Monitor transactions, review fraud decisions, and test risk policies.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body><AppShell localMerchantPreview={Boolean(process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim())}>{children}</AppShell></body></html>;
}
