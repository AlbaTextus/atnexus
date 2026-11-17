import { AccountOverview } from "@/components/account/account-overview"
import { requireOwnAccount } from "@/lib/account/account-session"

export default async function AccountPage({ params }: { params: Promise<{ userId: string }> }) {
  await requireOwnAccount((await params).userId)
  return <AccountOverview />
}
