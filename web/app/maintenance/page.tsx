import type { Metadata } from "next"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"
import { readMaintenance } from "@/lib/maintenance"
import { MaintenanceScreen } from "@/components/maintenance-screen"
import { maintenanceDestination } from "@/lib/maintenance-navigation"

export const metadata: Metadata = { title: "ALBATEXTUS", robots: { index: false, follow: false } }
export const dynamic = "force-dynamic"

export default async function MaintenancePage({ searchParams }: { searchParams: Promise<{ return_to?: string }> }) {
  const { return_to: returnTo } = await searchParams
  const state = await readMaintenance((await cookies()).toString())
  if (state && (!state.enabled || state.bypass)) redirect(maintenanceDestination(returnTo, state.bypass))
  return <MaintenanceScreen unavailable={!state} returnTo={returnTo} />
}
