import { redirect } from "next/navigation";
import { productionAuthenticationEnabled } from "../production-auth-server";

// The root layout validates the session and chooses the initial workspace.
export default function LoginPage() {
  if (!productionAuthenticationEnabled()) redirect("/dashboard");
  return null;
}
