import { AccountKeys } from "@/components/account/account-keys"
import { requireOwnAccount } from "@/lib/account/account-session"

export default async function AccountKeyPage({ params }: { params: Promise<{ userId: string }> }) {
  await requireOwnAccount((await params).userId, "/key")
  return <AccountKeys />
}
