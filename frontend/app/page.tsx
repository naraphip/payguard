import { redirect } from "next/navigation";

export default async function HomePage({ searchParams }: {
  searchParams: Promise<{ merchant?: string }>;
}) {
  const { merchant } = await searchParams;
  if (typeof merchant === "string" && merchant) {
    const query = new URLSearchParams({ merchant });
    redirect(`/simulation-lab?${query}#merchant-rules`);
  }
  redirect("/dashboard");
}
