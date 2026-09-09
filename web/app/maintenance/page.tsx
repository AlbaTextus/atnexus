import type { Metadata } from "next"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"
import { readMaintenance } from "@/lib/maintenance"
import { MaintenanceScreen } from "@/components/maintenance-screen"

export const metadata: Metadata = { title: "ALBATEXTUS", robots: { index: false, follow: false } }
export const dynamic = "force-dynamic"

export default async function MaintenancePage() {
  const state = await readMaintenance((await cookies()).toString())
  if (state && (!state.enabled || state.bypass)) redirect(state.bypass ? "/account" : "/")
  return <MaintenanceScreen unavailable={!state} />
}
