import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Simulation Lab | PayGuard",
  description: "Run synthetic checkout scenarios and inspect fraud decision evidence.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}
