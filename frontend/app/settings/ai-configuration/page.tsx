import type { Metadata } from "next";
import { AIConfiguration } from "./configuration";

export const metadata: Metadata = { title: "AI Configuration | PayGuard" };

export default function AIConfigurationPage() {
  return <AIConfiguration />;
}
