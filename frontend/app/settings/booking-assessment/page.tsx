import type { Metadata } from "next";
import { BookingAssessmentDemo } from "./booking-assessment-demo";

export const metadata: Metadata = { title: "Booking assessment | PayGuard" };

export default function BookingAssessmentPage() {
  return <main className="mx-auto max-w-[1080px] px-4 pt-7 pb-12 sm:px-7 sm:pt-9 lg:px-10">
    <header className="mb-6">
      <h1 className="text-[30px] leading-tight font-semibold tracking-tight sm:text-[36px]">Booking assessment</h1>
      <p className="mt-2 text-sm leading-relaxed text-slate-600">Approve a scoring profile and inspect the evidence for a synthetic booking.</p>
    </header>
    <BookingAssessmentDemo />
  </main>;
}
