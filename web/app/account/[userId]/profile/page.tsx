import { AccountProfile } from "@/components/account-profile"
import { requireOwnAccount } from "@/lib/account-session"

export default async function ProfilePage({ params }: { params: Promise<{ userId: string }> }) {
  await requireOwnAccount((await params).userId, "/profile")
  return <AccountProfile />
}
