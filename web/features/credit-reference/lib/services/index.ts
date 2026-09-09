// Wallet data adapter. The same-origin route forwards only the account session.
import type { TransactionListResponse, TransactionQueryParams } from "./transaction/types"
export * from "./transaction/types"

export async function previewRequest<T>(resource: string, params: Record<string, string> = {}): Promise<T> {
  const query = new URLSearchParams({ resource, ...params })
  const response = await fetch(`/account/wallet/data?${query}`, { cache: "no-store" })
  if (!response.ok) {
    const payload = await response.json().catch(() => null)
    throw new Error(payload?.error_description || (response.status === 401 ? "登录已失效，请重新登录。" : "钱包数据加载失败，请稍后重试。"))
  }
  return response.json()
}

const services = {
  transaction: {
    getTransactions(params: TransactionQueryParams): Promise<TransactionListResponse> {
      const query: Record<string, string> = {}
      for (const [key, value] of Object.entries(params)) {
        if (value !== undefined) query[key] = Array.isArray(value) ? value.join(",") : String(value)
      }
      return previewRequest("transactions", query)
    },
  },
}
export default services
