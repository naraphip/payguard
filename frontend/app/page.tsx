import { redirect } from "next/navigation";

export default async function HomePage({ searchParams }: {
  searchParams: Promise<{ merchant?: string; scenario?: string }>;
}) {
  const { merchant, scenario } = await searchParams;
  if (typeof merchant === "string" && merchant) {
    const query = new URLSearchParams({ merchant });
    if (typeof scenario === "string" && scenario) query.set("scenario", scenario);
    redirect(`/simulation-lab?${query}`);
  }
  redirect("/dashboard");
}
