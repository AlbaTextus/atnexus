export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
    this.name = "ApiError"
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: "include",
    ...init,
    headers: { "X-AlbaTextus-Client": "account-center", ...(init?.body ? { "Content-Type": "application/json" } : {}), ...init?.headers },
  })
  const contentType = response.headers.get("content-type") || ""
  const payload = contentType.includes("json") ? await response.json() : null
  if (!response.ok) {
    const message = payload?.error_description || payload?.error || "请求失败"
    throw new ApiError(response.status, payload?.error || "request_failed", message)
  }
  return payload as T
}

export function safeReturnTo(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//") || value.includes("\\")) return "/account"
  return value
}
