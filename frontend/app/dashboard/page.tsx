import type { Metadata } from "next";
import Link from "next/link";
import {
  AlertTriangle, ArrowRight, CheckCircle2, ClipboardClock, Clock3, CreditCard,
} from "lucide-react";

export const metadata: Metadata = { title: "Dashboard | PayGuard" };

const metrics = [
  { label: "Transactions", value: "12,450", Icon: CreditCard, color: "bg-indigo-50 text-indigo-700" },
  { label: "Fraud Alerts", value: "247", Icon: AlertTriangle, color: "bg-rose-50 text-rose-700" },
  { label: "Under Review", value: "36", Icon: ClipboardClock, color: "bg-amber-50 text-amber-700" },
  { label: "Approval Rate", value: "96.8%", Icon: CheckCircle2, color: "bg-emerald-50 text-emerald-700" },
];

const days = [
  { label: "Mon", allowed: 52, flagged: 18 },
  { label: "Tue", allowed: 65, flagged: 25 },
  { label: "Wed", allowed: 60, flagged: 21 },
  { label: "Thu", allowed: 76, flagged: 32 },
  { label: "Fri", allowed: 73, flagged: 26 },
  { label: "Sat", allowed: 89, flagged: 38 },
  { label: "Sun", allowed: 80, flagged: 29 },
];

const queue = [
  { id: "#1024", reason: "Booking velocity", age: "5 min ago" },
  { id: "#1019", reason: "Linked device", age: "12 min ago" },
  { id: "#1016", reason: "Payment risk", age: "23 min ago" },
];

const transactions = [
  { id: "#1024", type: "Travel booking", reason: "Booking velocity", action: "REVIEW", time: "10:42", color: "bg-amber-50 text-amber-800" },
  { id: "#1023", type: "Store checkout", reason: "Routine checkout", action: "ALLOW", time: "10:39", color: "bg-emerald-50 text-emerald-700" },
  { id: "#1022", type: "Gateway authorization", reason: "Payment verification", action: "CHALLENGE", time: "10:34", color: "bg-indigo-50 text-indigo-700" },
];

const panel = "min-w-0 overflow-hidden rounded-2xl border border-line bg-white shadow-xs";
const dashboardLink = "inline-flex shrink-0 items-center gap-1.5 rounded text-sm font-semibold text-accent hover:underline";

function RiskActivity() {
  return <section id="risk-trends" aria-labelledby="risk-activity-heading" className={`${panel} p-4 sm:p-6`}>
    <div className="flex flex-wrap items-start justify-between gap-x-5 gap-y-3">
      <div>
        <h2 id="risk-activity-heading" className="text-lg font-semibold tracking-tight">Risk Activity</h2>
        <p className="mt-0.5 text-sm text-muted">Sample decisions over 7 days</p>
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1.5 text-xs font-medium text-muted" aria-hidden="true">
        <span className="flex items-center gap-1.5"><i className="size-2 rounded-full bg-[#4658d6]" /> Allowed</span>
        <span className="flex items-center gap-1.5"><i className="size-2 rounded-full bg-[#aebaf8]" /> Flagged</span>
      </div>
    </div>
    <div role="img" aria-label="Sample weekly decisions, peaking on Saturday at 89 allowed and 38 flagged."
      className="mt-6 flex items-stretch gap-2.5 sm:gap-4">
      <div className="flex h-44 flex-col justify-between pb-0.5 text-right text-[11px] tabular-nums text-muted" aria-hidden="true">
        {["100", "75", "50", "25", "0"].map(value => <span key={value}>{value}</span>)}
      </div>
      <div className="min-w-0 flex-1" aria-hidden="true">
        <div className="relative grid h-44 grid-cols-7 items-end border-b border-[#e2e8f4]">
          <div className="pointer-events-none absolute inset-0 flex flex-col justify-between">
            {[0, 1, 2, 3].map(line => <span key={line} className="border-t border-dashed border-[#e2e8f4]" />)}
            <span />
          </div>
          {days.map(day => <div key={day.label} className="relative mx-auto flex h-full w-full max-w-[56px] items-end justify-center gap-1 px-0.5 pb-px sm:gap-1.5">
            <span className="w-2.5 rounded-t-sm bg-[#4658d6] sm:w-4" style={{ height: `${day.allowed}%` }} />
            <span className="w-2.5 rounded-t-sm bg-[#aebaf8] sm:w-4" style={{ height: `${day.flagged}%` }} />
          </div>)}
        </div>
        <div className="mt-2 grid grid-cols-7 text-center text-[11px] text-muted">
          {days.map(day => <span key={day.label}>{day.label}</span>)}
        </div>
      </div>
    </div>
  </section>;
}

export default function DashboardPage() {
  return <main className="mx-auto max-w-[1440px] space-y-6 px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-9">
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="text-[30px] leading-tight font-semibold tracking-[-0.045em] sm:text-[38px]">Fraud Overview</h1>
        <p className="mt-1.5 text-sm text-muted sm:text-[15px]">Transaction monitoring &amp; risk decisions</p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <span className="rounded-md border border-line bg-white px-2.5 py-1.5 text-xs font-medium text-muted">Sample data</span>
        <Link href="/settings/operations#transactions"
          className="inline-flex items-center gap-2 rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white hover:bg-indigo-800">
          <CreditCard size={16} aria-hidden="true" /> Transactions
        </Link>
      </div>
    </header>

    <dl aria-label="Sample fraud metrics" className="grid grid-cols-2 gap-3 xl:grid-cols-4">
      {metrics.map(({ label, value, Icon, color }) => <div key={label} className={`${panel} p-3.5 sm:p-5`}>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <dt className="text-xs font-medium text-muted sm:text-sm">{label}</dt>
          <span className={`hidden size-8 shrink-0 place-items-center rounded-lg sm:grid ${color}`}>
            <Icon size={16} aria-hidden="true" />
          </span>
        </div>
        <dd className="mt-2 text-[26px] leading-none font-semibold tracking-tight tabular-nums sm:mt-3 sm:text-[31px]">{value}</dd>
      </div>)}
    </dl>

    <div className="grid gap-5 min-[1300px]:grid-cols-[minmax(0,1.75fr)_minmax(280px,1fr)]">
      <RiskActivity />
      <section aria-labelledby="hold-queue-heading" className={panel}>
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 pt-4 pb-3 sm:px-6 sm:pt-6">
          <div>
            <h2 id="hold-queue-heading" className="text-lg font-semibold tracking-tight">HOLD Queue</h2>
            <p className="mt-0.5 text-sm text-muted">Sample cases awaiting review</p>
          </div>
          <Link href="/settings/operations" className={dashboardLink}>Open queue <ArrowRight size={15} aria-hidden="true" /></Link>
        </div>
        <ul className="divide-y divide-line px-4 pb-3 sm:px-6">
          {queue.map(item => <li key={item.id} className="flex flex-wrap items-center gap-x-2.5 gap-y-1 py-2.5 sm:py-3">
            <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-amber-50 text-amber-700"><Clock3 size={16} aria-hidden="true" /></span>
            <div className="min-w-0 flex-1">
              <p className="whitespace-nowrap text-sm font-semibold">Transaction {item.id}</p>
              <p className="text-xs text-muted">{item.reason}</p>
            </div>
            <span className="text-xs whitespace-nowrap text-muted">{item.age}</span>
          </li>)}
        </ul>
      </section>
    </div>

    <section aria-labelledby="recent-transactions-heading" className={panel}>
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-4 sm:px-6 sm:py-5">
        <div>
          <h2 id="recent-transactions-heading" className="text-lg font-semibold tracking-tight">Recent Transactions</h2>
          <p className="mt-0.5 text-sm text-muted">Sample risk decisions</p>
        </div>
        <Link href="/settings/operations#transactions" className={dashboardLink}>View transactions <ArrowRight size={15} aria-hidden="true" /></Link>
      </div>
      <table className="w-full table-fixed border-t border-line text-left text-sm">
        <thead className="bg-[#f8faff] text-xs font-medium text-muted">
          <tr>
            <th scope="col" className="w-[60%] px-4 py-2.5 sm:w-[36%] sm:px-6">Transaction</th>
            <th scope="col" className="hidden py-2.5 sm:table-cell">Signal</th>
            <th scope="col" className="w-[40%] pr-4 sm:w-[23%]">Decision</th>
            <th scope="col" className="hidden w-[15%] pr-6 text-right sm:table-cell">Time</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line">
          {transactions.map(item => <tr key={item.id}>
            <th scope="row" className="px-4 py-2.5 text-left sm:px-6 sm:py-3">
              <span className="block font-semibold">Transaction {item.id}</span>
              <span className="block text-xs font-normal text-muted">{item.type}</span>
            </th>
            <td className="hidden py-2.5 text-muted sm:table-cell">{item.reason}</td>
            <td className="pr-4"><span className={`inline-block rounded-full px-2 py-1 text-[10px] font-bold sm:text-[11px] ${item.color}`}>{item.action}</span></td>
            <td className="hidden pr-6 text-right text-sm tabular-nums text-muted sm:table-cell">{item.time}</td>
          </tr>)}
        </tbody>
      </table>
    </section>
  </main>;
}
