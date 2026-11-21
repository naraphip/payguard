"use client";

import { useSyncExternalStore } from "react";
import { getMerchantDemoRole, subscribeMerchantDemoSession } from "./merchant-demo";

export type WorkspaceView = "platform" | "merchant";

const preferenceKey = "payguard-workspace-view";
const changeEvent = "payguard-workspace-view-change";
let inMemoryView: WorkspaceView | null = null;

function subscribe(listener: () => void) {
  const unsubscribeSession = subscribeMerchantDemoSession(listener);
  window.addEventListener("storage", listener);
  window.addEventListener(changeEvent, listener);
  return () => {
    unsubscribeSession();
    window.removeEventListener("storage", listener);
    window.removeEventListener(changeEvent, listener);
  };
}

function getWorkspaceView() {
  if (getMerchantDemoRole() === "merchant") return "merchant";
  if (inMemoryView) return inMemoryView;
  try {
    return window.localStorage.getItem(preferenceKey) === "merchant" ? "merchant" : "platform";
  } catch {
    return "platform";
  }
}

function getServerWorkspaceView() {
  return null;
}

export function useWorkspaceView() {
  return useSyncExternalStore(subscribe, getWorkspaceView, getServerWorkspaceView);
}

export function useMerchantDemoRole() {
  return useSyncExternalStore(subscribeMerchantDemoSession, getMerchantDemoRole, getServerWorkspaceView);
}

export function setWorkspaceView(view: WorkspaceView) {
  try {
    window.localStorage.setItem(preferenceKey, view);
    inMemoryView = null;
  } catch {
    inMemoryView = view;
  }
  window.dispatchEvent(new Event(changeEvent));
}
