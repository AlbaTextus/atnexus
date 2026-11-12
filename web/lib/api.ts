import { collectLoginDevice } from "@/lib/login-device"
import { clearStoredContent } from "@/lib/content-cache"

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
    this.name = "ApiError"
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  if (init?.method === "POST" && (path === "/api/v1/sessions" || path === "/api/v1/users" || path === "/api/v1/passkeys/login/finish" || /^\/api\/v1\/auth\/providers\/[^/]+\/start$/.test(path))) {
    await collectLoginDevice()
  }
  const response = await fetch(path, {
    credentials: "include",
    ...init,
    headers: { "X-AlbaTextus-Client": "account-center", ...(init?.body && !(init.body instanceof FormData) ? { "Content-Type": "application/json" } : {}), ...init?.headers },
  })
  const contentType = response.headers.get("content-type") || ""
  const payload = contentType.includes("json") ? await response.json() : null
  if (!response.ok) {
    if (payload?.error === "maintenance" && typeof window !== "undefined" && !window.location.pathname.startsWith("/maintenance")) {
      window.location.replace("/maintenance")
    }
    const message = payload?.error_description || payload?.error || "请求失败"
    throw new ApiError(response.status, payload?.error || "request_failed", message)
  }
  if (init?.method === "DELETE" && path === "/api/v1/sessions/current") clearStoredContent()
  if (init?.method === "POST" && (path === "/api/v1/account/athub/submissions" || path.startsWith("/api/v1/admin/athub/reviews/"))) {
    clearStoredContent()
    if (typeof window !== "undefined") window.dispatchEvent(new Event("atnexus:content-changed"))
  }
  return payload as T
}

export function safeReturnTo(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//") || value.includes("\\")) return "/account"
  return value
}
