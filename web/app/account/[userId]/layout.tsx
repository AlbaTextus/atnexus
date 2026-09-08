import type { ReactNode } from "react"
import { AccountShell } from "@/components/account-shell"
import { requireOwnAccount } from "@/lib/account-session"

export default async function PrivateAccountLayout({ children, params }: { children: ReactNode; params: Promise<{ userId: string }> }) {
  const { userId } = await params
  const user = await requireOwnAccount(userId)
  return <AccountShell initialUser={user}>{children}</AccountShell>
}
