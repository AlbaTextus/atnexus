import type { NextRequest } from "next/server"

export const runtime = "nodejs"
export const dynamic = "force-dynamic"

export async function GET(request: NextRequest) {
  const headers = { "Cache-Control": "private, no-store", "X-Data-Source": "wallet" }
  const cookie = request.headers.get("cookie") || ""
  if (!cookie) return Response.json({ error: "login_required" }, { status: 401, headers })
  try {
    const backend = (process.env.OAUTH_BACKEND_URL || "http://localhost:8080").replace(/\/$/, "")
    const upstream = await fetch(`${backend}/api/v1/account/wallet/view?${request.nextUrl.searchParams}`, {
      headers: { Cookie: cookie, "X-AlbaTextus-Client": "account-center", Accept: "application/json" },
      cache: "no-store", redirect: "error", signal: AbortSignal.timeout(15000),
    })
    if (!upstream.headers.get("content-type")?.includes("application/json")) throw new Error("Invalid wallet response")
    const responseHeaders = new Headers(headers)
    responseHeaders.set("Content-Type", "application/json; charset=utf-8")
    const retryAfter = upstream.headers.get("retry-after")
    if (retryAfter) responseHeaders.set("Retry-After", retryAfter)
    return new Response(upstream.body, { status: upstream.status, headers: responseHeaders })
  } catch {
    return Response.json({ error: "wallet_unavailable", error_description: "钱包暂时不可用，请稍后重试。" }, { status: 503, headers })
  }
}
