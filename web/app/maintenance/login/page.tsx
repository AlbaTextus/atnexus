import type { Metadata } from "next"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"
import { AuthLayout } from "@/components/auth-layout"
import { LoginForm } from "@/components/login-form"
import { readMaintenance } from "@/lib/maintenance"

export const metadata: Metadata = { title: "超级管理员登录 · ALBATEXTUS", robots: { index: false, follow: false } }

export default async function MaintenanceLoginPage({ searchParams }: { searchParams: Promise<{ oauth_error?: string }> }) {
  const params = await searchParams
  const state = await readMaintenance((await cookies()).toString())
  if (state?.bypass) redirect("/account")
  if (state && !state.enabled) redirect("/login")
  return <AuthLayout appearance="white" background="identity"><LoginForm returnTo="/account" oauthError={params.oauth_error} maintenance /></AuthLayout>
}
