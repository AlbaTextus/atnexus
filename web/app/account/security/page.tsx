import { redirectToAccount } from "@/lib/account/account-session"

type SearchParams = { oauth_linked?: string | string[]; oauth_error?: string | string[] }

export default async function SecurityPage({ searchParams }: { searchParams: Promise<SearchParams> }) {
  const { oauth_linked, oauth_error } = await searchParams
  const query = new URLSearchParams()
  if (oauth_linked === "1") query.set("oauth_linked", oauth_linked)
  if (typeof oauth_error === "string" && oauth_error) query.set("oauth_error", oauth_error)
  return redirectToAccount(query.size ? `/security?${query}` : "/security")
}
