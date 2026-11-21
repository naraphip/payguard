import type { Metadata } from "next";
import { MockCheckout } from "./mock-checkout";

export const metadata: Metadata = { title: "Mock checkout | PayGuard" };

export default function MockCheckoutPage() {
  return <main className="mx-auto max-w-[1180px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header className="mb-6">
      <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Mock checkout</h1>
      <p className="mt-2 text-sm leading-relaxed text-slate-600">Submit a synthetic travel or e-commerce order and inspect its booking assessment.</p>
    </header>
    <MockCheckout />
  </main>;
}
