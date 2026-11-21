import Link from "next/link";
import { Building2, ClipboardCheck, FlaskConical, ShoppingBag, SlidersHorizontal } from "lucide-react";

const links = [
  { href: "/", label: "Simulation Lab", Icon: FlaskConical },
  { href: "/settings/mock-checkout", label: "Mock checkout", Icon: ShoppingBag },
  { href: "/settings/merchant-setup", label: "Merchant setup", Icon: Building2 },
  { href: "/settings/booking-assessment", label: "Booking assessment", Icon: ClipboardCheck },
  { href: "/settings/system-rules", label: "Platform Rules", Icon: SlidersHorizontal },
] as const;

export function SettingsNavigation({ current }: { current: (typeof links)[number]["href"] }) {
  return <nav aria-label="PayGuard" className="flex flex-wrap gap-1 md:flex-col">
    {links.map(({ href, label, Icon }) => <Link
      key={href}
      href={href}
      aria-current={href === current ? "page" : undefined}
      className={`flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition ${href === current
        ? "bg-indigo-50 font-semibold text-indigo-800 ring-1 ring-inset ring-indigo-100"
        : "font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900"}`}
    ><Icon className="size-4" aria-hidden="true" />{label}</Link>)}
  </nav>;
}
