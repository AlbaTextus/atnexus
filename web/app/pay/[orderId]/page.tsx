import { notFound } from "next/navigation"
import { AccountShell } from "@/components/account-shell"
import { PaymentCheckout } from "@/components/payment-checkout"
import { requireAccount } from "@/lib/account-session"

export default async function PaymentPage({ params }: { params: Promise<{ orderId: string }> }) {
  const { orderId } = await params
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(orderId)) notFound()
  const user = await requireAccount(`/pay/${orderId}`)
  return <AccountShell initialUser={user}><PaymentCheckout key={orderId} orderId={orderId} /></AccountShell>
}
