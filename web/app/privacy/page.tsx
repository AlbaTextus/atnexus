import { redirect } from "next/navigation"
import type { LegalPageParams } from "@/components/legal-page"

export default async function LegacyLegalPage({ searchParams }: LegalPageParams) {
  const params = await searchParams
  const query = new URLSearchParams()
  if (params.return_to) query.set("return_to", params.return_to)
  if (params.from) query.set("from", params.from)
  redirect("/wiki/privacy" + (query.size ? `?${query}` : ""))
}
