import { NextRequest, NextResponse } from "next/server"
import { readMaintenance } from "@/lib/maintenance/maintenance"
import { maintenanceDestination } from "@/lib/maintenance/maintenance-navigation"

export async function proxy(request: NextRequest) {
  const path = request.nextUrl.pathname
  // Go applies the same rule to APIs and all OAuth protocol endpoints.
  if (path.startsWith("/api/") || path.startsWith("/.well-known/") || path === "/healthz") return NextResponse.next()
  if (path === "/maintenance" || path === "/maintenance/login") return NextResponse.next()
  const state = await readMaintenance(request.headers.get("cookie") || "")
  if (!state || (state.enabled && !state.bypass)) {
    if (path.startsWith("/oauth/") && request.method !== "GET") return NextResponse.next()
    const destination = request.nextUrl.clone()
    const returnTo = request.nextUrl.pathname + request.nextUrl.search
    destination.pathname = "/maintenance"
    destination.search = ""
    destination.searchParams.set("return_to", maintenanceDestination(returnTo))
    const response = NextResponse.redirect(destination, 307)
    response.headers.set("Cache-Control", "no-store")
    response.headers.set("Retry-After", "15")
    return response
  }
  const response = NextResponse.next()
  response.headers.set("Cache-Control", "private, no-store")
  return response
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|brands/|providers/).*)"],
}
