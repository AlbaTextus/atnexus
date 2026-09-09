import { Check, Clock3, Fingerprint, LockKeyhole } from "lucide-react"
import { creditAmount, orderStatus, walletTime, type PaymentOrder } from "@/lib/wallet"
import { Badge } from "@/components/ui/badge"
import { CardContent, CardHeader } from "@/components/ui/card"

export function PayingInfo({ order, expired }: { order: PaymentOrder; expired: boolean }) {
  const status = expired ? "expired" : order.status
  const StatusIcon = status === "paid" ? Check : status === "pending" ? Clock3 : LockKeyhole
  return <>
    <CardHeader className="checkout-heading">
      <div className="checkout-brand-row"><span className="account-member-brand">ALBATEXTUS</span><Fingerprint className="size-7" aria-hidden="true" /></div>
      <div className="checkout-amount-block">
        <p className="checkout-eyebrow">{status === "paid" ? "已支付积分" : "订单金额"}</p>
        <p className="checkout-amount"><span>{creditAmount(order.amount)}</span></p>
        <Badge variant="outline" className="checkout-status"><StatusIcon aria-hidden="true" />{orderStatus[status]}</Badge>
      </div>
    </CardHeader>
    <CardContent className="checkout-order-details">
      <p className="checkout-order-title">{order.title}</p>
      <dl className="wallet-receipt-facts">
        <div><dt>收款应用</dt><dd>{order.client_name}</dd></div>
        <div><dt>{order.paid_at ? "支付时间" : "有效期至"}</dt><dd>{walletTime(order.paid_at || order.expires_at)}</dd></div>
      </dl>
      <details className="checkout-order-number"><summary>订单编号</summary><span>{order.external_order_id}</span></details>
    </CardContent>
  </>
}
