import { Account } from "@/components/account/account"
import { requireOwnAccount } from "@/lib/account/account-session"

export default async function SecurityPage({ params }: { params: Promise<{ userId: string }> }) {
  await requireOwnAccount((await params).userId, "/security")
  return <Account />
}
