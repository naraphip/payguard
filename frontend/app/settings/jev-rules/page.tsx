import { connection } from "next/server";
import { redirect } from "next/navigation";

export default async function JevRulesPage() {
  await connection();
  if (process.env.PAYGUARD_LOCAL_MERCHANT_PREVIEW_TOKEN?.trim()) redirect("/settings/merchants?view=rules");
  redirect("/settings/merchant-rules");
}
