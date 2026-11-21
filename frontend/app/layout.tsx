import type { Metadata } from "next";
import "@fontsource/ibm-plex-sans-thai/400.css";
import "@fontsource/ibm-plex-sans-thai/500.css";
import "@fontsource/ibm-plex-sans-thai/600.css";
import "@fontsource/ibm-plex-sans-thai/700.css";
import { AppShell } from "./app-shell";
import { LocalPreviewProvider } from "./local-admin-preview";
import "./globals.css";

// The loopback preview is enabled by the server environment when it starts.
export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "PayGuard | Fraud Management",
  description: "Monitor transactions, review fraud decisions, and test risk policies.",
  icons: {
    icon: [
      { url: "/brand/payguard-app-icon-light.svg?v=4", type: "image/svg+xml", sizes: "any" },
      { url: "/brand/payguard-app-icon-dark.svg?v=4", type: "image/svg+xml", sizes: "any", media: "(prefers-color-scheme: dark)" },
    ],
  },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const local = Boolean(process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim());
  return <html lang="en"><body><LocalPreviewProvider enabled={local}><AppShell localMerchantPreview={local}>{children}</AppShell></LocalPreviewProvider></body></html>;
}
