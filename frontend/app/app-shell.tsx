"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { ReactNode } from "react";
import {
  Activity, Bell, Building2, ChartNoAxesCombined, ChevronDown, ChevronRight, ClipboardCheck,
  ClipboardList, FlaskConical, LayoutDashboard, Menu, Network, PanelLeftClose, PanelLeftOpen, Settings,
  ShieldCheck, ShoppingBag, SlidersHorizontal, Sparkles, Store, X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { PayGuardLogo } from "./payguard-logo";
import { setWorkspaceView, useWorkspaceView } from "./workspace-view";
import type { WorkspaceView } from "./workspace-view";
import "./app-shell.css";

type Destination = {
  href: string;
  label: string;
  Icon: LucideIcon;
  platformOnly?: boolean;
};

const groups: { label: string; items: Destination[] }[] = [
  {
    label: "Overview",
    items: [
      { href: "/dashboard", label: "Dashboard", Icon: LayoutDashboard },
      { href: "/dashboard#risk-trends", label: "Analytics", Icon: ChartNoAxesCombined },
    ],
  },
  {
    label: "Fraud Operations",
    items: [
      { href: "/settings/operations#transactions", label: "Transactions", Icon: ClipboardList },
      { href: "/settings/operations", label: "Risk Operations", Icon: ShieldCheck },
      { href: "/simulation-lab#fraud-network", label: "Fraud Network", Icon: Network },
    ],
  },
  {
    label: "Decision Engine",
    items: [
      { href: "/settings/system-rules", label: "Platform Rules", Icon: SlidersHorizontal, platformOnly: true },
      { href: "/settings/jev-rules", label: "Jev Rules", Icon: Sparkles },
      { href: "/settings/merchant-rules", label: "Merchant Rules", Icon: Store },
      { href: "/settings/booking-assessment", label: "Scoring Profiles", Icon: ClipboardCheck },
    ],
  },
  {
    label: "Testing & Setup",
    items: [
      { href: "/simulation-lab", label: "Simulation Lab", Icon: FlaskConical },
      { href: "/settings/mock-checkout", label: "Mock Checkout", Icon: ShoppingBag },
      { href: "/settings/merchant-setup", label: "Merchants", Icon: Building2 },
      { href: "/settings", label: "Settings", Icon: Settings },
    ],
  },
];

const destinations = groups.flatMap(group => group.items.map(item => ({ ...item, group: group.label })));

function subscribeToHash(listener: () => void) {
  const navigation = (window as Window & { navigation?: EventTarget }).navigation;
  const onHistoryChange = () => queueMicrotask(listener);
  navigation?.addEventListener("currententrychange", onHistoryChange);
  window.addEventListener("hashchange", listener);
  window.addEventListener("popstate", listener);
  return () => {
    navigation?.removeEventListener("currententrychange", onHistoryChange);
    window.removeEventListener("hashchange", listener);
    window.removeEventListener("popstate", listener);
  };
}

function getHash() {
  return window.location.hash;
}

function getServerHash() {
  return "";
}

function Sidebar({ view, activeHref, collapsed, drawer, onToggle, onNavigate }: {
  view: WorkspaceView | null;
  activeHref: string;
  collapsed?: boolean;
  drawer?: boolean;
  onToggle: () => void;
  onNavigate?: () => void;
}) {
  const Action = drawer ? X : collapsed ? PanelLeftOpen : PanelLeftClose;
  const actionLabel = drawer ? "Close navigation" : collapsed ? "Expand sidebar" : "Collapse sidebar";
  return <div className="shell-sidebar-inner">
    <div className="shell-sidebar-brand">
      <Link href="/dashboard" className="shell-logo" aria-label="PayGuard dashboard" onClick={onNavigate}>
        {collapsed ? <ShieldCheck size={24} aria-hidden="true" /> : <PayGuardLogo />}
      </Link>
      {drawer && <button className="shell-icon-button" type="button" aria-label={actionLabel} onClick={onToggle}>
        <Action size={18} aria-hidden="true" />
      </button>}
    </div>
    <nav aria-label="Primary navigation" className="shell-nav">
      {groups.map(group => <div className="shell-nav-group" key={group.label}>
        <div className="shell-nav-title">{group.label}</div>
        <ul>
          {group.items.filter(item => !item.platformOnly || view === "platform").map(({ href, label, Icon }) =>
            <li key={href}><Link href={href} aria-current={activeHref === href ? "page" : undefined}
              title={collapsed ? label : undefined} className="shell-nav-link" onClick={onNavigate}>
              <Icon size={18} aria-hidden="true" />
              <span>{label}</span>
            </Link></li>
          )}
        </ul>
      </div>)}
    </nav>
    {!drawer && <div className="shell-sidebar-footer">
      <button className="shell-nav-link shell-collapse" type="button" aria-label={actionLabel} aria-expanded={!collapsed}
        title={actionLabel} onClick={onToggle}>
        <Action size={17} aria-hidden="true" />
        <span>Collapse sidebar</span>
      </button>
    </div>}
  </div>;
}

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const hash = useSyncExternalStore(subscribeToHash, getHash, getServerHash);
  const view = useWorkspaceView();
  const [collapsed, setCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const drawer = useRef<HTMLDialogElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const current = destinations.find(destination => destination.href === pathname + hash)
    ?? destinations.find(destination => destination.href === pathname);
  const restricted = current?.platformOnly === true;

  useEffect(() => {
    const desktop = window.matchMedia("(min-width: 1024px)");
    function closeOnDesktop(event: MediaQueryListEvent) {
      if (event.matches) drawer.current?.close();
    }
    desktop.addEventListener("change", closeOnDesktop);
    return () => desktop.removeEventListener("change", closeOnDesktop);
  }, []);

  function showDrawer() {
    drawer.current?.showModal();
    setDrawerOpen(true);
  }

  function closeDrawer() {
    drawer.current?.close();
  }

  function handleDrawerClose() {
    setDrawerOpen(false);
    toggle.current?.focus();
  }

  return <div className="app-shell" data-collapsed={collapsed}>
    <a className="shell-skip-link" href="#page-content">Skip to content</a>
    <aside className="shell-sidebar">
      <Sidebar view={view} activeHref={current?.href ?? ""} collapsed={collapsed} onToggle={() => setCollapsed(value => !value)} />
    </aside>
    <dialog id="mobile-navigation" aria-label="PayGuard navigation" className="shell-drawer" ref={drawer}
      onClose={handleDrawerClose} onClick={event => {
        if (event.target === event.currentTarget) closeDrawer();
      }}>
      <Sidebar view={view} activeHref={current?.href ?? ""} drawer onToggle={closeDrawer} onNavigate={closeDrawer} />
    </dialog>
    <div className="shell-main">
      <header className="shell-topbar">
        <div className="shell-topbar-start">
          <button ref={toggle} className="shell-icon-button shell-menu-button" type="button" aria-label="Open navigation"
            aria-controls="mobile-navigation" aria-expanded={drawerOpen} onClick={showDrawer}>
            <Menu size={19} aria-hidden="true" />
          </button>
          <nav aria-label="Breadcrumb" className="shell-breadcrumb">
            {current && <>
              <span className="shell-breadcrumb-section">{current.group}</span>
              <ChevronRight className="shell-breadcrumb-section" size={14} aria-hidden="true" />
            </>}
            <span aria-current="page">{current?.label ?? "PayGuard"}</span>
          </nav>
        </div>
        <div className="shell-topbar-actions">
          <label className="shell-workspace">
            <Building2 size={16} aria-hidden="true" />
            <span className="shell-workspace-name" aria-hidden="true">{view === "merchant" ? "Merchant" : "Platform workspace"}</span>
            <ChevronDown className="shell-workspace-chevron" size={13} aria-hidden="true" />
            <select aria-label="Sandbox workspace" value={view ?? "platform"} onChange={event => setWorkspaceView(event.target.value === "merchant" ? "merchant" : "platform")}>
              <option value="platform">Platform Admin</option>
              <option value="merchant">Merchant</option>
            </select>
          </label>
          <span className="shell-sandbox">Sandbox</span>
          <details name="payguard-topbar" className="shell-popover">
            <summary className="shell-icon-button" aria-label="Notifications" title="Notifications"><Bell size={17} aria-hidden="true" /></summary>
            <div className="shell-popover-content">
              <p className="font-semibold text-ink">Notifications</p>
              <p className="mt-1">No notifications in this workspace.</p>
            </div>
          </details>
          <details name="payguard-topbar" className="shell-popover">
            <summary className="shell-icon-button shell-user" aria-label="User menu" title="User menu"><span aria-hidden="true">{view === "merchant" ? "M" : "PA"}</span></summary>
            <div className="shell-popover-content">
              <p className="font-semibold text-ink">{view === "merchant" ? "Merchant" : "Platform Admin"}</p>
              <p className="mt-1">Sandbox workspace view</p>
            </div>
          </details>
        </div>
      </header>
      <div id="page-content" tabIndex={-1} className="min-w-0">
        {!restricted || view === "platform" ? children : view === "merchant" ? <main className="mx-auto max-w-5xl px-4 py-10 sm:px-8">
          <div className="rounded-2xl border border-line bg-white p-6">
            <ShieldCheck size={23} className="text-accent" aria-hidden="true" />
            <h1 className="mt-3 text-2xl font-semibold">Platform Rules</h1>
            <p className="mt-2 text-sm text-muted">Platform Rule Settings are available in the Platform Admin workspace.</p>
            <Link href="/dashboard" className="mt-4 inline-flex items-center gap-2 text-sm font-semibold text-accent">
              <Activity size={16} aria-hidden="true" /> Back to Dashboard
            </Link>
          </div>
        </main> : null}
      </div>
    </div>
  </div>;
}
