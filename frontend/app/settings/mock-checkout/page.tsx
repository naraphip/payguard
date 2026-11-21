import type { Metadata } from "next";
import { MockCheckout } from "./mock-checkout";

export const metadata: Metadata = { title: "Mock Checkout | PayGuard" };

export default function MockCheckoutPage() {
  return <main className="mx-auto max-w-[1180px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header className="mb-6">
      <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Mock Checkout</h1>
      <p className="mt-2 text-sm leading-relaxed text-slate-600">Test a synthetic order using the selected merchant’s active rules.</p>
    </header>
    <MockCheckout />
  </main>;
}
