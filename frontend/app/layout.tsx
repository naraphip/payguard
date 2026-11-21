import type { Metadata } from "next";
import { AppNavigation } from "./navigation";
import "./globals.css";

export const metadata: Metadata = {
  title: "Simulation Lab | PayGuard",
  description: "Run synthetic checkout scenarios and inspect fraud decision evidence.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body><AppNavigation />{children}</body></html>;
}
