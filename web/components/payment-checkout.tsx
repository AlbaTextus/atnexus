"use client"

import Link from "next/link"
import { useEffect, useState, type FormEvent } from "react"
import { AlertCircle, ArrowLeft, ArrowRight, Check, LockKeyhole, RefreshCw, ShieldCheck } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { feedback, useFeedback } from "@/lib/feedback"
import { creditAmount, orderStatus, type PaymentOrder, type Wallet } from "@/lib/wallet"
import { useAccountUser } from "@/components/account-shell"
import { PayingInfo } from "@/components/wallet/paying-info"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardFooter } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { UserAvatar } from "@/components/user-avatar"
import { Separator } from "@/components/ui/separator"
import "@/styles/payment-checkout.css"

export function PaymentCheckout({ orderId }: { orderId: string }) {
  const user = useAccountUser()
  const [order, setOrder] = useState<PaymentOrder | null>(null)
  const [wallet, setWallet] = useState<Wallet | null>(null)
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(true)
  const [revision, setRevision] = useState(0)
  const [now, setNow] = useState(() => Date.now())
  const [error, setError] = useFeedback("error")
  const walletHref = `/account/${user.numeric_id}/wallet`
  const endpoint = `/api/v1/account/wallet/orders/${encodeURIComponent(orderId)}`
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([api<PaymentOrder>(endpoint), api<Wallet>("/api/v1/account/wallet")]).then(([order, wallet]) => {
      if (!cancelled) { setOrder(order); setWallet(wallet); setNow(Date.now()) }
    }).catch(reason => { if (!cancelled) setError(managementError(reason)) }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [endpoint, revision, setError])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  const expired = Boolean(order && order.status === "pending" && new Date(order.expires_at).getTime() <= now)
  const pending = order?.status === "pending" && !expired
  async function pay(event: FormEvent) {
    event.preventDefault()
    if (busy || loading || !pending) return
    setBusy(true); setError("")
    try {
      const paid = await api<PaymentOrder>(endpoint + "/pay", { method: "POST" })
      setOrder(paid); feedback.success("支付成功")
    } catch (reason) { setError(managementError(reason)); setRevision(v => v + 1) } finally { setBusy(false) }
  }
  async function cancel() {
    if (busy || !pending) return
    setBusy(true); setError("")
    try { await api(endpoint + "/cancel", { method: "POST" }); feedback.success("订单已取消") }
    catch (reason) { setError(managementError(reason)) }
    finally { setBusy(false); setRevision(v => v + 1) }
  }
  const payerName = user.display_name || `用户 ${user.numeric_id}`
  return <div className="checkout-page">
    <h1 className="sr-only">支付订单</h1>
    <div className="checkout-toolbar">
      <Button variant="ghost" size="sm" nativeButton={false} render={<Link href={walletHref} />}><ArrowLeft data-icon="inline-start" />返回钱包</Button>
      <Button variant="ghost" size="icon" aria-label="刷新订单" disabled={busy || loading} onClick={() => { setError(""); setRevision(v => v + 1) }}><RefreshCw /></Button>
    </div>
    {loading && !order ? <div className="checkout-ticket-shadow" role="status" aria-label="正在加载支付订单">
      <Card className="account-member-card wallet-receipt checkout-ticket checkout-loading">
        <div className="checkout-heading">
          <div className="checkout-brand-row"><span className="account-member-brand">ALBATEXTUS</span><ShieldCheck className="checkout-loading-mark" aria-hidden="true" /></div>
          <div className="checkout-amount-block">
            <p className="checkout-eyebrow">正在准备订单信息<span className="checkout-loading-dots" aria-hidden="true"><i /><i /><i /></span></p>
            <Skeleton className="h-14 w-40" aria-hidden="true" />
            <Skeleton className="h-5 w-20" aria-hidden="true" />
          </div>
        </div>
        <div className="checkout-order-details" aria-hidden="true">
          <Skeleton className="mx-auto h-5 w-3/5" />
          <div className="flex justify-between gap-4"><Skeleton className="h-3 w-16" /><Skeleton className="h-3 w-28" /></div>
          <div className="flex justify-between gap-4"><Skeleton className="h-3 w-20" /><Skeleton className="h-3 w-24" /></div>
        </div>
        <Separator className="checkout-perforation" />
        <CardContent className="checkout-payment" aria-hidden="true">
          <div className="checkout-payer"><Skeleton className="size-10 shrink-0 rounded-full" /><div className="flex flex-1 flex-col gap-3"><Skeleton className="h-3 w-16" /><Skeleton className="h-3 w-28" /></div></div>
          <Skeleton className="h-12 w-full" />
        </CardContent>
        <CardFooter className="checkout-footer"><ShieldCheck className="size-3.5" aria-hidden="true" /><span>ALBATEXTUS · 安全支付</span><span className="checkout-monogram" aria-hidden="true">AT</span></CardFooter>
      </Card>
    </div> : order && wallet ? <div className="checkout-ticket-shadow checkout-ready">
      <Card className="account-member-card wallet-receipt checkout-ticket">
        <PayingInfo order={order} expired={expired} />
        <Separator className="checkout-perforation" />
        <CardContent className="checkout-payment">
          {pending && <div className="checkout-payer">
            <UserAvatar src={user.avatar_url} name={payerName} className="size-10 shrink-0" />
            <div className="checkout-payer-identity"><span className="checkout-eyebrow">付款账户</span><span className="checkout-payer-name">{payerName}</span></div>
          </div>}
          {error && <Alert variant="destructive"><AlertCircle /><AlertDescription role="alert">{error}</AlertDescription></Alert>}
          {order.status === "paid" ? <div key="paid" className="checkout-state checkout-complete checkout-paid">
            <Check className="size-7" aria-hidden="true" /><h2>支付成功</h2><p role="status">积分已支付，可以返回原应用继续操作。</p>
            <Button nativeButton={false} render={<Link href={walletHref} />} size="lg" className="w-full">查看钱包<ArrowRight data-icon="inline-end" /></Button>
          </div> : !pending ? <div key="closed" className="checkout-state checkout-complete">
            <LockKeyhole className="size-6" aria-hidden="true" /><h2>{expired ? "订单已过期" : orderStatus[order.status]}</h2><p role="status">此订单已关闭，无法继续支付。</p>
            <Button nativeButton={false} render={<Link href={walletHref} />} variant="outline" size="lg" className="w-full">返回钱包</Button>
          </div> : <form onSubmit={pay} aria-busy={busy} className="checkout-state">
            <Button type="submit" size="lg" disabled={busy || loading} className="w-full">{busy ? <Spinner data-icon="inline-start" /> : <Check data-icon="inline-start" />}{busy ? "正在支付…" : `确认支付 ${creditAmount(order.amount)} 积分`}</Button>
          </form>}
        </CardContent>
        <CardFooter className="checkout-footer"><ShieldCheck className="size-3.5" aria-hidden="true" /><span>ALBATEXTUS · 安全支付</span><span className="checkout-monogram" aria-hidden="true">AT</span></CardFooter>
      </Card>
    </div> : error && <Alert variant="destructive"><AlertCircle /><AlertDescription role="alert">{error}</AlertDescription></Alert>}
    {pending && <Button type="button" variant="ghost" size="sm" className="checkout-cancel" disabled={busy || loading} onClick={cancel}>取消订单</Button>}
  </div>
}
