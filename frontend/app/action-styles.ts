const buttonBase = "inline-flex min-h-10 items-center justify-center gap-2 rounded-lg border px-3.5 py-2 text-sm font-semibold leading-tight transition-colors disabled:cursor-not-allowed disabled:border-slate-200 disabled:bg-slate-100 disabled:text-slate-600 disabled:shadow-none";

export const actionButton = {
  primary: `${buttonBase} border-transparent bg-accent text-white shadow-sm hover:bg-[#082452]`,
  secondary: `${buttonBase} border-slate-300 bg-white text-slate-800 hover:border-slate-400 hover:bg-slate-50`,
  positive: `${buttonBase} border-transparent bg-emerald-700 text-white shadow-sm hover:bg-emerald-800`,
  warning: `${buttonBase} border-amber-300 bg-amber-50 text-amber-950 hover:border-amber-500 hover:bg-amber-100`,
  danger: `${buttonBase} border-transparent bg-red-700 text-white shadow-sm hover:bg-red-800`,
  dangerOutline: `${buttonBase} border-red-300 bg-white text-red-800 hover:border-red-400 hover:bg-red-50`,
} as const;

const badgeTone = {
  neutral: "border-slate-200 bg-slate-100 text-slate-800",
  info: "border-blue-200 bg-blue-50 text-blue-900",
  positive: "border-emerald-200 bg-emerald-50 text-emerald-900",
  warning: "border-amber-200 bg-amber-50 text-amber-950",
  danger: "border-red-200 bg-red-50 text-red-900",
} as const;

export function stateBadge(tone: keyof typeof badgeTone) {
  return `inline-flex w-fit items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold leading-tight ${badgeTone[tone]}`;
}

export function ruleActionTone(action: string): keyof typeof badgeTone {
  switch (action) {
    case "BLOCK": return "danger";
    case "REVIEW": return "warning";
    case "CHALLENGE": return "info";
    case "ALLOW": return "positive";
    default: return "neutral";
  }
}
