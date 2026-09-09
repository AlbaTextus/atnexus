export type Wallet = { balance: string; pending_balance: string }
export type WalletEntry = { id: string; order_id: string | null; amount: string; balance_after: string; kind: "payment" | "receipt" | "adjustment"; description: string; created_at: string }
export type PaymentOrder = {
  id: string; client_id: string; client_name: string; external_order_id: string
  payer_id: string; payee_id: string; title: string; amount: string
  status: "pending" | "paid" | "cancelled" | "expired"
  created_at: string; expires_at: string; paid_at: string | null
}
export const orderStatus = { pending: "待支付", paid: "已支付", cancelled: "已取消", expired: "已过期" }
export function creditAmount(amount: string) { return amount.replace(/\B(?=(\d{3})+(?!\d))/g, ",") }
export function walletTime(value: string) { return new Date(value).toLocaleString("zh-CN", { hour12: false }) }
