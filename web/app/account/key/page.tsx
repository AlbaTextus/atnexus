import { redirectToAccount } from "@/lib/account/account-session"

export default async function AccountKeyPage() {
  return redirectToAccount("/key")
}
