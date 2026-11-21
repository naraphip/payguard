import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight, Building2, ClipboardCheck } from "lucide-react";

export const metadata: Metadata = { title: "Settings | PayGuard" };

const settings = [
  { href: "/settings/merchant-setup", title: "Merchant Setup", description: "Manage the business model and available booking signals.", Icon: Building2 },
  { href: "/settings/booking-assessment", title: "Scoring Profiles", description: "Generate and approve profiles for booking assessment.", Icon: ClipboardCheck },
];

export default function SettingsPage() {
  return <main className="mx-auto max-w-5xl px-4 pt-7 pb-12 sm:px-8 sm:pt-9">
    <h1 className="text-[30px] font-semibold tracking-tight sm:text-[36px]">Settings</h1>
    <ul className="mt-6 grid gap-3 sm:grid-cols-2">
      {settings.map(({ href, title, description, Icon }) => <li key={href}>
        <Link href={href} className="flex h-full items-center gap-3 rounded-xl border border-line bg-white p-4 shadow-xs hover:border-accent sm:p-5">
          <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-soft text-accent"><Icon size={18} aria-hidden="true" /></span>
          <span className="flex-1">
            <span className="block font-semibold">{title}</span>
            <span className="mt-1 block text-sm text-muted">{description}</span>
          </span>
          <ArrowRight size={17} className="shrink-0 text-accent" aria-hidden="true" />
        </Link>
      </li>)}
    </ul>
  </main>;
}
