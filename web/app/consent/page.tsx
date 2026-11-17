import type { Metadata } from "next"
import { ConsentScreen } from "@/components/auth/consent-screen"

export const metadata: Metadata = { title: "ALBATEXTUS", robots: { index: false, follow: false }, referrer: "no-referrer" }

export default async function ConsentPage({ searchParams }: { searchParams: Promise<{ request?: string }> }) {
  const params = await searchParams
  return <ConsentScreen requestToken={params.request || ""} />
}
