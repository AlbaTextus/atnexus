export type MaintenanceStatus = { enabled: boolean; bypass: boolean; updated_at: string }

// Called on the server with its configured backend, never through the web proxy.
export async function readMaintenance(cookie: string): Promise<MaintenanceStatus | null> {
  try {
    const backend = process.env.OAUTH_BACKEND_URL || "http://localhost:8010"
    const response = await fetch(`${backend}/api/v1/maintenance`, {
      headers: { Cookie: cookie }, cache: "no-store", redirect: "error", signal: AbortSignal.timeout(5000),
    })
    if (!response.ok) return null
    const state = await response.json()
    if (typeof state.enabled !== "boolean" || typeof state.bypass !== "boolean") return null
    return state
  } catch { return null }
}
