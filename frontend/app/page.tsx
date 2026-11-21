import { redirect } from "next/navigation";
import { productionAuthenticationEnabled } from "./production-auth-server";

export default async function HomePage({ searchParams }: {
  searchParams: Promise<{ merchant?: string }>;
}) {
  if (productionAuthenticationEnabled()) redirect("/settings/merchants");
  const { merchant } = await searchParams;
  if (typeof merchant === "string" && merchant) {
    const query = new URLSearchParams({ merchant });
    redirect(`/simulation-lab?${query}#merchant-rules`);
  }
  redirect("/dashboard");
}
