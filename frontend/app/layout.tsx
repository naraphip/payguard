import type { Metadata } from "next";
import { AppShell } from "./app-shell";
import { LocalPreviewProvider } from "./local-admin-preview";
import "./globals.css";

// The loopback preview is enabled by the server environment when it starts.
export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "PayGuard | Fraud Management",
  description: "Monitor transactions, review fraud decisions, and test risk policies.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const local = Boolean(process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim());
  return <html lang="en"><body><LocalPreviewProvider enabled={local}><AppShell localMerchantPreview={local}>{children}</AppShell></LocalPreviewProvider></body></html>;
}
