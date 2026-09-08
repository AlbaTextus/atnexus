import { Account } from "@/components/account"
import { requireOwnAccount } from "@/lib/account-session"

export default async function SecurityPage({ params }: { params: Promise<{ userId: string }> }) {
  await requireOwnAccount((await params).userId, "/security")
  return <Account />
}
