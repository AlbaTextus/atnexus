// Only resume local pages, and never send the maintenance page back to itself.
export function maintenanceDestination(returnTo: string | null | undefined, bypass = false): string {
  const fallback = bypass ? "/account" : "/"
  if (typeof returnTo !== "string" || !returnTo.startsWith("/") || returnTo.startsWith("//") || /[\\\u0000-\u0020\u007f]/.test(returnTo)) return fallback
  try {
    const pathname = decodeURIComponent(returnTo.split(/[?#]/, 1)[0]).replace(/\/+$/, "")
    if (pathname === "/maintenance" || pathname.startsWith("/maintenance/")) return fallback
    if (pathname.startsWith("//") || /[\\\u0000-\u0020\u007f]/.test(pathname)) return fallback
  } catch { return fallback }
  return returnTo
}

export function maintenancePath(returnTo: string): string {
  return `/maintenance?return_to=${encodeURIComponent(maintenanceDestination(returnTo))}`
}
