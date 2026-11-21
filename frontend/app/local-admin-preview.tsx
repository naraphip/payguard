"use client";

import { createContext, useContext, type ReactNode } from "react";
import { useWorkspaceView } from "./workspace-view";

const LocalAdminPreview = createContext(false);

export function LocalPreviewProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  return <LocalAdminPreview.Provider value={enabled}>{children}</LocalAdminPreview.Provider>;
}

export function useLocalAdminPreview() {
  const enabled = useContext(LocalAdminPreview);
  const view = useWorkspaceView();
  return enabled && view === "platform" && typeof window !== "undefined" &&
    ["localhost", "127.0.0.1", "[::1]"].includes(window.location.hostname);
}

// Only administrator screens select this transport. The server supplies its local credential.
export function adminRulePath(path: string, local: boolean, token: string) {
  return local && !token ? path.replace(/^\/api\/v1\//, "/api/merchant-preview/admin/") : path;
}
