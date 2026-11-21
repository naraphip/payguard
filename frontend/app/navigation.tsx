"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Building2, ClipboardCheck, FlaskConical, ShoppingBag } from "lucide-react";
import { PayGuardLogo } from "./payguard-logo";

const destinations = [
  { href: "/", label: "Simulation Lab", Icon: FlaskConical },
  { href: "/settings/mock-checkout", label: "Mock checkout", Icon: ShoppingBag },
  { href: "/settings/merchants", label: "Merchants", Icon: Building2 },
  { href: "/settings/merchant-setup", label: "Merchant setup", Icon: Building2 },
  { href: "/settings/booking-assessment", label: "Booking assessment", Icon: ClipboardCheck },
];

export function AppNavigation() {
  const pathname = usePathname();

  return <header className="border-b border-line bg-white">
    <div className="mx-auto grid max-w-[1440px] grid-cols-[minmax(0,1fr)_auto] items-center gap-x-5 gap-y-2 px-4 py-3 sm:px-7 lg:grid-cols-[auto_minmax(0,1fr)_auto] lg:gap-x-7 lg:px-8">
      <Link href="/" aria-label="PayGuard home" className="col-start-1 row-start-1 w-fit rounded-sm"><PayGuardLogo /></Link>
      <nav aria-label="PayGuard" className="col-span-2 row-start-2 flex flex-wrap items-center gap-1 lg:col-span-1 lg:col-start-2 lg:row-start-1">
        {destinations.map(({ href, label, Icon }) => {
          const active = pathname === href || (href !== "/" && pathname.startsWith(`${href}/`));
          return <Link key={href} href={href} aria-current={active ? "page" : undefined}
            className={`inline-flex min-h-9 items-center gap-1.5 rounded-lg px-2.5 text-xs font-semibold lg:px-2 xl:px-3 xl:text-sm ${active
              ? "bg-soft text-accent ring-1 ring-inset ring-indigo-100"
              : "text-muted hover:bg-paper hover:text-ink"}`}>
            <Icon className="size-4 shrink-0" aria-hidden="true" />{label}
          </Link>;
        })}
      </nav>
      <span className="col-start-2 row-start-1 inline-flex items-center gap-1.5 rounded-full border border-line bg-paper px-2.5 py-1 text-xs font-semibold text-muted lg:col-start-3">
        <span className="size-1.5 rounded-full bg-teal-600" aria-hidden="true" />Sandbox
      </span>
    </div>
  </header>;
}
