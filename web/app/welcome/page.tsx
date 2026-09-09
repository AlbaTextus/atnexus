import type { Metadata } from "next"
import { redirect } from "next/navigation"
import { AuthLayout } from "@/components/auth-layout"
import { ProfileSetupForm } from "@/components/profile-setup-form"
import { requireAccount } from "@/lib/account-session"
import { afterProfileSetup, profileSetupPath } from "@/lib/profile-setup"

export const metadata: Metadata = { title: "ALBATEXTUS", robots: { index: false, follow: false } }

export default async function WelcomePage({ searchParams }: { searchParams: Promise<{ return_to?: string }> }) {
  const { return_to } = await searchParams
  const destination = afterProfileSetup(return_to || null)
  const user = await requireAccount(profileSetupPath(destination), true)
  if (!user.profile_setup_required) redirect(destination)
  return <AuthLayout><ProfileSetupForm initialName={user.display_name || ""} destination={destination} /></AuthLayout>
}
