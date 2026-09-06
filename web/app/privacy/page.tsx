import type { Metadata } from "next"
import { LegalPage, type LegalPageParams } from "@/components/legal-page"

export const metadata: Metadata = { title: "隐私政策 · AlbaTextus 账户" }

export default async function PrivacyPage({ searchParams }: LegalPageParams) {
  const params = await searchParams
  return <LegalPage slug="privacy" returnTo={params.return_to} from={params.from} />
}
