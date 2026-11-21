import { connection } from "next/server";
import { redirect } from "next/navigation";
import { JevRulesPanel } from "./panel";

export default async function JevRulesPage() {
  await connection();
  if (process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim()) redirect("/settings/merchants?view=jev");
  return <JevRulesPanel />;
}
