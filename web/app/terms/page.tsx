import { redirect } from "next/navigation"
import { wikiUrl } from "@/lib/wiki"

type LegalPageParams = { searchParams: Promise<{ return_to?: string; from?: string }> }

/** Policy text is published on the standalone Wiki; return links keep pointing back to this account centre. */
export default async function LegacyLegalPage({ searchParams }: LegalPageParams) {
  const params = await searchParams
  const query = new URLSearchParams()
  if (params.return_to) query.set("return_to", params.return_to)
  if (params.from) query.set("from", params.from)
  redirect(wikiUrl("/terms") + (query.size ? `?${query}` : ""))
}
