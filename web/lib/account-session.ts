import "server-only"

import { cache } from "react"
import { cookies } from "next/headers"
import { notFound, redirect } from "next/navigation"
import type { AccountUser } from "@/components/account-shell"
import { profileSetupPath } from "@/lib/profile-setup"

const currentAccount = cache(async (): Promise<AccountUser | null> => {
  const cookieJar = await cookies()
  const backend = process.env.OAUTH_BACKEND_URL || "http://localhost:8080"
  const response = await fetch(`${backend}/api/v1/user`, {
    headers: { Cookie: cookieJar.toString() }, cache: "no-store", redirect: "error",
    signal: AbortSignal.timeout(10000),
  })
  if (response.status === 401) return null
  if (response.status === 433 || response.status === 503) redirect("/maintenance")
  if (!response.ok) throw new Error("暂时无法加载账户，请稍后重试。")
  return response.json()
})

export async function requireAccount(returnTo = "/account", allowIncomplete = false) {
  const user = await currentAccount()
  if (!user) redirect(`/login?return_to=${encodeURIComponent(returnTo)}`)
  if (user.profile_setup_required && !allowIncomplete) redirect(profileSetupPath(returnTo))
  return user
}

export async function requireOwnAccount(userId: string, suffix = "") {
  if (!/^[1-9]\d*$/.test(userId)) notFound()
  const user = await requireAccount(`/account/${userId}${suffix}`)
  // Never resolve a profile from the URL alone, including for administrators.
  if (String(user.numeric_id) !== userId) notFound()
  return user
}

export async function redirectToAccount(suffix = "") {
  const user = await requireAccount(`/account${suffix}`)
  redirect(`/account/${user.numeric_id}${suffix}`)
}
