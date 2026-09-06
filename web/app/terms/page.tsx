import type { Metadata } from "next"
import { LegalPage, type LegalPageParams } from "@/components/legal-page"

export const metadata: Metadata = { title: "用户协议 · AlbaTextus 账户" }

export default async function TermsPage({ searchParams }: LegalPageParams) {
  const params = await searchParams
  return <LegalPage slug="terms" returnTo={params.return_to} from={params.from} />
}
