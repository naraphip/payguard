import { redirect } from "next/navigation";

export default async function JevRulesPage({ searchParams }: { searchParams: Promise<{ merchant?: string }> }) {
  const { merchant } = await searchParams;
  const query = typeof merchant === "string" && merchant ? `?${new URLSearchParams({ merchant })}` : "";
  redirect(`/settings/merchant-rules${query}`);
}
