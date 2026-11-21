"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Fragment, useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { ReactNode } from "react";
import {
  Bell, Building2, ChevronDown, ChevronRight, ClipboardCheck,
  ClipboardList, FlaskConical, LayoutDashboard, Menu, Network, PanelLeftClose, PanelLeftOpen, Settings,
  ShieldCheck, ShoppingBag, SlidersHorizontal, Sparkles, Store, X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { PayGuardLogo } from "./payguard-logo";
import { setWorkspaceView, useMerchantDemoRole, useWorkspaceView } from "./workspace-view";
import type { WorkspaceView } from "./workspace-view";
import "./app-shell.css";

type Destination = {
  href: string;
  label: string;
  Icon: LucideIcon;
  platformOnly?: boolean;
  standalone?: boolean;
};

const groups: { label: string; items: Destination[] }[] = [
  {
    label: "Overview",
    items: [
      { href: "/dashboard", label: "Dashboard", Icon: LayoutDashboard },
    ],
  },
  {
    label: "Fraud Operations",
    items: [
      { href: "/settings/operations#transactions", label: "Transactions", Icon: ClipboardList },
      { href: "/settings/operations", label: "Risk Operations", Icon: ShieldCheck, standalone: true },
      { href: "/simulation-lab#fraud-network", label: "Fraud Network", Icon: Network },
    ],
  },
  {
    label: "Decision Engine",
    items: [
      { href: "/settings/system-rules", label: "Platform Rules", Icon: SlidersHorizontal, platformOnly: true, standalone: true },
      { href: "/settings/jev-rules", label: "Jev Rules", Icon: Sparkles, standalone: true },
      { href: "/settings/merchant-rules", label: "Merchant Rules", Icon: Store, standalone: true },
      { href: "/settings/booking-assessment", label: "Scoring Profiles", Icon: ClipboardCheck },
    ],
  },
  {
    label: "Testing & Setup",
    items: [
      { href: "/simulation-lab", label: "Simulation Lab", Icon: FlaskConical },
      { href: "/settings/mock-checkout", label: "Mock Checkout", Icon: ShoppingBag },
      { href: "/settings/merchants", label: "Merchants", Icon: Building2 },
      { href: "/settings", label: "Settings", Icon: Settings },
    ],
  },
];

const destinations = groups.flatMap(group => group.items.map(item => ({ ...item, group: group.label })));
const merchantPath = "/settings/merchants";
const merchantTransactions: Destination = {
  href: `${merchantPath}?view=transactions`, label: "Transactions", Icon: ClipboardList,
};
const merchantGroups = [{ label: "Merchant", items: [merchantTransactions] }];

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
  const params = new URLSearchParams(window.location.search);
  return params.get("view") === "transactions" ? "?view=transactions" : window.location.hash;
}

function getServerHash() {
  return "";
}

function Sidebar({ view, activeHref, collapsed, drawer, onToggle, onNavigate, localMerchantPreview }: {
  localMerchantPreview: boolean;
  view: WorkspaceView | null;
  activeHref: string;
  collapsed?: boolean;
  drawer?: boolean;
  onToggle: () => void;
  onNavigate?: () => void;
}) {
  const Action = drawer ? X : collapsed ? PanelLeftOpen : PanelLeftClose;
  const actionLabel = drawer ? "Close navigation" : collapsed ? "Expand sidebar" : "Collapse sidebar";
  const visibleGroups = view === "merchant" ? merchantGroups : view === "platform" ? groups : [];
  const home = view === "merchant" ? merchantTransactions.href : "/dashboard";
  return <div className="shell-sidebar-inner">
    <div className="shell-sidebar-brand">
      <Link href={home} className="shell-logo" aria-label={view === "merchant" ? "PayGuard transactions" : "PayGuard dashboard"} onClick={onNavigate}>
        {collapsed ? <ShieldCheck size={24} aria-hidden="true" /> : <PayGuardLogo />}
      </Link>
      {drawer && <button className="shell-icon-button" type="button" aria-label={actionLabel} onClick={onToggle}>
        <Action size={18} aria-hidden="true" />
      </button>}
    </div>
    <nav aria-label="Primary navigation" className="shell-nav">
      {visibleGroups.map(group => <div className="shell-nav-group" key={group.label}>
        <div className="shell-nav-title">{group.label}</div>
        <ul>
          {group.items.filter(item => (!item.platformOnly || view === "platform") && !(localMerchantPreview && item.standalone)).map(({ href, label, Icon }) =>
            <li key={href}><Link href={localMerchantPreview && href === "/settings/operations#transactions" ? "/settings/merchants?view=transactions" : href} aria-current={activeHref === (localMerchantPreview && href === "/settings/operations#transactions" ? merchantTransactions.href : href) ? "page" : undefined}
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

export function AppShell({ children, localMerchantPreview }: { children: ReactNode; localMerchantPreview: boolean }) {
  const pathname = usePathname();
  const router = useRouter();
  const hash = useSyncExternalStore(subscribeToHash, getHash, getServerHash);
  const view = useWorkspaceView();
  const verifiedRole = useMerchantDemoRole();
  const [collapsed, setCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const drawer = useRef<HTMLDialogElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const allowed = view === "platform" || (view === "merchant" && pathname === merchantPath);
  const activeDestinations = localMerchantPreview
    ? destinations.filter(destination => !destination.standalone).map(destination =>
        destination.href === "/settings/operations#transactions"
          ? { ...destination, href: merchantTransactions.href }
          : destination)
    : destinations;
  const current = view === "merchant" ? { ...merchantTransactions, group: "Merchant" } : view === "platform"
    ? activeDestinations.find(destination => destination.href === pathname + hash)
      ?? activeDestinations.find(destination => destination.href === pathname)
    : undefined;

  useEffect(() => {
    if (view === "merchant" && pathname !== merchantPath) router.replace(merchantTransactions.href);
  }, [view, pathname, router]);

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
      <Sidebar localMerchantPreview={localMerchantPreview} view={view} activeHref={current?.href ?? ""} collapsed={collapsed} onToggle={() => setCollapsed(value => !value)} />
    </aside>
    <dialog id="mobile-navigation" aria-label="PayGuard navigation" className="shell-drawer" ref={drawer}
      onClose={handleDrawerClose} onClick={event => {
        if (event.target === event.currentTarget) closeDrawer();
      }}>
      <Sidebar localMerchantPreview={localMerchantPreview} view={view} activeHref={current?.href ?? ""} drawer onToggle={closeDrawer} onNavigate={closeDrawer} />
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
            <select aria-label="Sandbox workspace" value={view ?? "platform"} disabled={!view || verifiedRole === "merchant"} onChange={event => setWorkspaceView(event.target.value === "merchant" ? "merchant" : "platform")}>
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
        {allowed ? <Fragment key={view}>{children}</Fragment> :
          <p role="status" className="px-4 py-8 text-sm text-muted sm:px-8">
            {view === "merchant" ? "Opening your transactions…" : "Loading workspace…"}
          </p>}
      </div>
    </div>
  </div>;
}
