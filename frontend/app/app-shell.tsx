"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Fragment, useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { ReactNode } from "react";
import {
  Bell, Building2, ChevronDown, ChevronRight,
  ClipboardList, FlaskConical, LayoutDashboard, Menu, PanelLeftClose, PanelLeftOpen, Settings,
  ShieldCheck, ShoppingBag, SlidersHorizontal, Sparkles, Store, X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { PayGuardLogo } from "./payguard-logo";
import { useProductionSession } from "./production-auth";
import type { ProductionRole } from "./production-auth";
import { merchantDetailsHref } from "./settings/merchants/management-contracts";
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
type NavigationGroup = { label: string; items: Destination[] };

const groups: NavigationGroup[] = [
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
    ],
  },
  {
    label: "Decision Engine",
    items: [
      { href: "/settings/system-rules", label: "Platform Rules", Icon: SlidersHorizontal, platformOnly: true },
      { href: "/settings/ai-configuration", label: "AI Configuration", Icon: Sparkles, platformOnly: true },
    ],
  },
  {
    label: "Management",
    items: [
      { href: "/settings/merchants", label: "Merchants", Icon: Building2 },
    ],
  },
  {
    label: "Testing",
    items: [
      { href: "/simulation-lab", label: "Simulation Lab", Icon: FlaskConical },
      { href: "/settings/mock-checkout", label: "Mock Checkout", Icon: ShoppingBag },
    ],
  },
  {
    label: "System",
    items: [
      { href: "/settings", label: "Settings", Icon: Settings },
    ],
  },
];

const destinations = groups.flatMap(group => group.items.map(item => ({ ...item, group: group.label })));
const merchantPath = "/settings/merchants";
const merchantTransactions: Destination = {
  href: `${merchantPath}?view=transactions`, label: "Transactions", Icon: ClipboardList,
};
const merchantRules: Destination = { href: "/settings/merchant-rules", label: "Merchant AI Rules", Icon: Store };
const merchantDashboard: Destination = { href: "/dashboard", label: "Dashboard", Icon: LayoutDashboard };
const merchantSettings: Destination = { href: "/settings", label: "Settings", Icon: Settings };
const directory: Destination = { href: merchantPath, label: "Merchants", Icon: Building2 };
const merchantDestinations = [merchantDashboard, merchantTransactions, merchantRules, merchantSettings];
const merchantGroups = [{ label: "Merchant", items: merchantDestinations }];
const productionMerchantGroups = [{ label: "Merchant", items: [directory, ...merchantDestinations] }];
const productionAdminGroups = groups.map(group => ({
  ...group,
  items: group.items.filter(item => !item.standalone && item.href !== "/settings/mock-checkout")
    .map(item => item.href === "/settings/operations#transactions" ? merchantTransactions : item),
}));
const checkerGroups = [{ label: "Decision Engine", items: [groups[2].items[0]] }];
const roleLabels: Record<ProductionRole, string> = {
  platform_admin: "Platform Admin",
  platform_checker: "Platform Checker",
  merchant: "Merchant",
};

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
  const view = params.get("view");
  return view === "transactions" ? "?view=transactions" : window.location.hash;
}

function getServerHash() {
  return "";
}

function Sidebar({ view, activeHref, collapsed, drawer, onToggle, onNavigate, localMerchantPreview, productionRole }: {
  localMerchantPreview: boolean;
  productionRole?: ProductionRole;
  view: WorkspaceView | null;
  activeHref: string;
  collapsed?: boolean;
  drawer?: boolean;
  onToggle: () => void;
  onNavigate?: () => void;
}) {
  const Action = drawer ? X : collapsed ? PanelLeftOpen : PanelLeftClose;
  const actionLabel = drawer ? "Close navigation" : collapsed ? "Expand sidebar" : "Collapse sidebar";
  const visibleGroups = productionRole === "merchant" ? productionMerchantGroups : productionRole === "platform_checker" ? checkerGroups
    : productionRole === "platform_admin" ? productionAdminGroups : view === "merchant" ? merchantGroups : view === "platform" ? groups : [];
  const home = productionRole === "platform_checker" ? "/settings/system-rules" : productionRole ? merchantPath : "/dashboard";
  return <div className="shell-sidebar-inner">
    <div className="shell-sidebar-brand">
      <Link href={home} className="shell-logo" aria-label="PayGuard dashboard" onClick={onNavigate}>
        <PayGuardLogo compact={collapsed} />
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
  const production = useProductionSession();
  const roleLabel = production ? roleLabels[production.role] : view === "merchant" ? "Merchant" : "Platform Admin";
  const [collapsed, setCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const drawer = useRef<HTMLDialogElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const merchantDetails = pathname.startsWith(`${merchantPath}/`);
  const merchantDestination = pathname === merchantPath || merchantDetails
    ? production && !merchantDetails && hash !== "?view=transactions" ? directory : merchantTransactions
    : merchantDestinations.find(destination => destination.href === pathname);
  const merchantPathAllowed = Boolean(merchantDestination);
  const openMerchantDirectory = view === "platform" && pathname === merchantRules.href;
  const productionHome = production?.role === "platform_checker" ? "/settings/system-rules" : merchantPath;
  const canOpenProductionPage = production?.role === "platform_checker" ? pathname === productionHome :
    production?.role === "merchant"
      ? merchantPathAllowed && (!merchantDetails || pathname === `${merchantPath}/${production.merchantId}`)
      : merchantDetails || productionAdminGroups.some(group => group.items.some(item => item.href === pathname));
  const allowed = production ? canOpenProductionPage :
    (view === "platform" && !openMerchantDirectory) || (view === "merchant" && merchantPathAllowed);
  const activeDestinations = localMerchantPreview || production?.role === "platform_admin"
    ? destinations.filter(destination => !destination.standalone).map(destination =>
        destination.href === "/settings/operations#transactions"
          ? { ...destination, href: merchantTransactions.href }
          : destination)
    : destinations;
  const current = view === "merchant" && merchantDestination ? { ...merchantDestination, group: "Merchant" } : view === "platform"
    ? (merchantDetails || openMerchantDirectory ? destinations.find(destination => destination.href === merchantPath) : undefined)
      ?? activeDestinations.find(destination => destination.href === pathname + hash)
      ?? activeDestinations.find(destination => destination.href === pathname)
    : undefined;

  useEffect(() => {
    if (production) {
      if (!canOpenProductionPage) router.replace(productionHome);
      return;
    }
    if (view === "merchant" && !merchantPathAllowed) router.replace(merchantTransactions.href);
    if (openMerchantDirectory) {
      const merchant = new URLSearchParams(window.location.search).get("merchant");
      router.replace(merchant ? merchantDetailsHref(merchant, "rules") : merchantPath);
    }
  }, [view, merchantPathAllowed, openMerchantDirectory, router, production, canOpenProductionPage, productionHome]);

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
      <Sidebar localMerchantPreview={localMerchantPreview} productionRole={production?.role} view={view} activeHref={current?.href ?? ""} collapsed={collapsed} onToggle={() => setCollapsed(value => !value)} />
    </aside>
    <dialog id="mobile-navigation" aria-label="PayGuard navigation" className="shell-drawer" ref={drawer}
      onClose={handleDrawerClose} onClick={event => {
        if (event.target === event.currentTarget) closeDrawer();
      }}>
      <Sidebar localMerchantPreview={localMerchantPreview} productionRole={production?.role} view={view} activeHref={current?.href ?? ""} drawer onToggle={closeDrawer} onNavigate={closeDrawer} />
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
          {production ? <div className="shell-workspace"><Building2 size={16} aria-hidden="true" />{roleLabel}</div> : <>
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
          </>}
          <details name="payguard-topbar" className="shell-popover">
            <summary className="shell-icon-button" aria-label="Notifications" title="Notifications"><Bell size={17} aria-hidden="true" /></summary>
            <div className="shell-popover-content">
              <p className="font-semibold text-ink">Notifications</p>
              <p className="mt-1">No notifications in this workspace.</p>
            </div>
          </details>
          <details name="payguard-topbar" className="shell-popover">
            <summary className="shell-icon-button shell-user" aria-label="User menu" title="User menu"><span aria-hidden="true">{production?.role === "platform_checker" ? "PC" : view === "merchant" ? "M" : "PA"}</span></summary>
            <div className="shell-popover-content">
              <p className="font-semibold text-ink">{roleLabel}</p>
              {production ? <form className="mt-2" method="post" action="/api/auth/logout">
                <button className="font-semibold text-accent hover:underline" type="submit">Sign out</button>
              </form> : <p className="mt-1">Sandbox workspace view</p>}
            </div>
          </details>
        </div>
      </header>
      <div id="page-content" tabIndex={-1} className="min-w-0">
        {allowed ? <Fragment key={view}>{children}</Fragment> :
          <p role="status" className="px-4 py-8 text-sm text-muted sm:px-8">
            {view === "merchant" ? "Opening your transactions…" : openMerchantDirectory ? "Opening merchants…" : "Loading workspace…"}
          </p>}
      </div>
    </div>
  </div>;
}
