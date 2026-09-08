import { AccountOverview } from "@/components/account-overview"
import { requireOwnAccount } from "@/lib/account-session"

export default async function AccountPage({ params }: { params: Promise<{ userId: string }> }) {
  await requireOwnAccount((await params).userId)
  return <AccountOverview />
}
