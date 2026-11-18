import { collectLoginDevice } from "@/lib/auth/login-device"
import { clearStoredContent } from "@/lib/content-cache"
import { maintenancePath } from "@/lib/maintenance/maintenance-navigation"

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
    this.name = "ApiError"
  }
}

function waitForRetry(signal: AbortSignal | null | undefined, delay: number) {
  return new Promise<void>((resolve, reject) => {
    signal?.throwIfAborted()
    const finish = () => {
      signal?.removeEventListener("abort", abort)
      resolve()
    }
    const abort = () => {
      clearTimeout(timer)
      reject(signal?.reason)
    }
    const timer = setTimeout(finish, delay)
    signal?.addEventListener("abort", abort, { once: true })
  })
}

async function request(path: string, init?: RequestInit) {
  const readOnly = ["GET", "HEAD"].includes((init?.method || "GET").toUpperCase())
  for (let attempt = 0; ; attempt++) {
    const canRetry = readOnly && attempt < 2
    try {
      const response = await fetch(path, {
        credentials: "include",
        ...init,
        headers: { "X-AlbaTextus-Client": "account-center", ...(init?.body && !(init.body instanceof FormData) ? { "Content-Type": "application/json" } : {}), ...init?.headers },
      })
      const contentType = response.headers.get("content-type") || ""
      const json = contentType.includes("json")
      const payload = response.status !== 204 && init?.method?.toUpperCase() !== "HEAD" && json ? await response.json() : null
      // A gateway failure may be an empty/plain-text 500 from Next's proxy.
      // Authentication, maintenance and application errors stay authoritative.
      const transient = [502, 503, 504].includes(response.status) || (response.status === 500 && !json)
      if (!canRetry || !transient || payload?.error === "maintenance") return { response, payload }
      if (!response.bodyUsed) await response.body?.cancel()
    } catch (reason) {
      if (!canRetry || init?.signal?.aborted || !(reason instanceof TypeError)) throw reason
    }
    await waitForRetry(init?.signal, attempt === 0 ? 250 : 750)
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  if (init?.method === "POST" && (path === "/api/v1/sessions" || path === "/api/v1/users" || path === "/api/v1/passkeys/login/finish" || /^\/api\/v1\/auth\/providers\/[^/]+\/start$/.test(path))) {
    await collectLoginDevice()
  }
  const { response, payload } = await request(path, init)
  if (!response.ok) {
    if (payload?.error === "maintenance" && typeof window !== "undefined" && !window.location.pathname.startsWith("/maintenance")) {
      window.location.replace(maintenancePath(window.location.pathname + window.location.search + window.location.hash))
    }
    // A request that outlives the server's deadline is answered by the shared
    // module wrapper, which has no description to give.
    const message = payload?.error_description || (payload?.error === "server_timeout" ? "服务器处理超时，请稍后重试。" : payload?.error) || "请求失败"
    throw new ApiError(response.status, payload?.error || "request_failed", message)
  }
  if (init?.method === "DELETE" && path === "/api/v1/sessions/current") clearStoredContent()
  if (init?.method === "POST" && (path === "/api/v4/account/athub/submissions" || path.startsWith("/api/v4/admin/athub/reviews/") || /^\/api\/v4\/account\/athub\/novels\/\d+\/illustrations$/.test(path)) ||
    init?.method === "PUT" && /^\/api\/v4\/account\/athub\/novels\/\d+(\/illustration-layout)?$/.test(path) ||
    init?.method === "DELETE" && /^\/api\/v4\/account\/athub\/novels\/\d+\/illustrations\/\d+$/.test(path)) {
    clearStoredContent()
    if (typeof window !== "undefined") window.dispatchEvent(new Event("atnexus:content-changed"))
  }
  return payload as T
}

export function safeReturnTo(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//") || value.includes("\\")) return "/account"
  return value
}
