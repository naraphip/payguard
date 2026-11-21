import type { Metadata } from "next";
import { headers } from "next/headers";
import "@fontsource/ibm-plex-sans-thai/400.css";
import "@fontsource/ibm-plex-sans-thai/500.css";
import "@fontsource/ibm-plex-sans-thai/600.css";
import "@fontsource/ibm-plex-sans-thai/700.css";
import { AppShell } from "./app-shell";
import { LoginPanel } from "./login-panel";
import { LocalPreviewProvider } from "./local-admin-preview";
import { ProductionAuthenticationProvider } from "./production-auth";
import { localPreviewAllowed, productionAuthenticationEnabled, productionHostAllowed, productionSession, sandboxHostAllowed } from "./production-auth-server";
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

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const requestHeaders = await headers();
  if (productionAuthenticationEnabled()) {
    const auth = productionHostAllowed(requestHeaders)
      ? await productionSession(requestHeaders.get("cookie")) : { session: null, unavailable: true };
    return <html lang="en" data-payguard-auth="oidc"><body>
      {auth.session
        ? <ProductionAuthenticationProvider session={auth.session}><AppShell localMerchantPreview={false}>{children}</AppShell></ProductionAuthenticationProvider>
        : <LoginPanel unavailable={auth.unavailable} />}
    </body></html>;
  }
  if (!sandboxHostAllowed(requestHeaders)) return <html lang="en"><body><LoginPanel unavailable /></body></html>;
  const local = localPreviewAllowed(requestHeaders);
  return <html lang="en"><body><LocalPreviewProvider enabled={local}><AppShell localMerchantPreview={local}>{children}</AppShell></LocalPreviewProvider></body></html>;
}
