"use client"

import { useEffect, useState, type ReactNode } from "react"
import { Banknote, Check, CircleAlert, CircleCheck, Clock3, Copy, RotateCcw, X } from "lucide-react"
import type { Order } from "@/features/credit-reference/lib/services"
import { Card, CardContent, CardFooter, CardHeader } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/features/credit-reference/components/ui/button"
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from "@/features/credit-reference/components/ui/dialog"
import { formatDateTime } from "@/features/credit-reference/lib/utils"
import { statusConfig, typeConfig } from "./table-filter"

const numberFormat = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 })

export type ReceiptData = {
  number: string
  numberLabel: string
  name: string
  amount: string
  status: string
  statusIcon: typeof CircleCheck
  facts: string[][]
  extraFacts: string[][]
}

export function WalletReceiptCard({ data, title, description, close }: {
  data: ReceiptData; title: ReactNode; description: ReactNode; close: ReactNode
}) {
  const [copyState, setCopyState] = useState<"idle" | "copied" | "error">("idle")
  useEffect(() => {
    if (copyState === "idle") return
    const timer = window.setTimeout(() => setCopyState("idle"), 2000)
    return () => window.clearTimeout(timer)
  }, [copyState])

  async function copyNumber() {
    try {
      await navigator.clipboard.writeText(data.number)
      setCopyState("copied")
    } catch { setCopyState("error") }
  }

  const StatusIcon = data.statusIcon
  return <Card className="account-member-card wallet-receipt">
        <CardHeader className="payment-summary wallet-receipt-heading">
          <div className="wallet-receipt-brand-row">
            <span className="account-member-brand">ALBATEXTUS</span>
            <div className="wallet-receipt-close-slot">{close}</div>
          </div>
          <div className="sr-only">{title}</div>
          {description}
          <p className="wallet-receipt-amount">{data.amount}</p>
          <Badge variant="secondary" className="wallet-receipt-status"><StatusIcon aria-hidden="true" />{data.status}</Badge>
        </CardHeader>
        <CardContent className="wallet-receipt-body">
          <p className="wallet-receipt-order-name">{data.name}</p>
          <dl className="wallet-receipt-facts">{data.facts.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
          <details className="wallet-receipt-more">
            <summary>更多信息</summary>
            <dl className="wallet-receipt-facts">{data.extraFacts.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
          </details>
        </CardContent>
        <Separator className="wallet-receipt-perforation" />
        <CardFooter className="wallet-receipt-footer">
          <div className="flex w-full min-w-0 items-center justify-between gap-3">
            <div className="flex min-w-0 flex-col gap-1"><span className="text-[10px] text-muted-foreground">{data.numberLabel}</span><span className="break-all font-mono text-xs tabular-nums">{data.number}</span></div>
            <Button variant="ghost" size="icon" className="shrink-0 rounded-full" aria-label={`复制${data.numberLabel}`} onClick={copyNumber}>{copyState === "copied" ? <Check /> : <Copy />}</Button>
          </div>
          <div className="flex w-full items-center justify-between gap-2 text-[10px] text-muted-foreground">
            <span role="status">{copyState === "copied" ? "编号已复制" : copyState === "error" ? "复制失败，请手动选择编号复制" : "ALBATEXTUS · 积分记录"}</span>
            <span className="font-semibold" aria-hidden="true">AT</span>
          </div>
        </CardFooter>
      </Card>
}

function orderReceiptData(order: Order): ReceiptData {
  const hasTradeTime = order.status === "success" || order.status === "refund"
  return {
    number: order.order_no, numberLabel: "订单编号", name: order.order_name,
    amount: numberFormat.format(Number(order.amount)), status: statusConfig[order.status]?.label || order.status,
    statusIcon: order.status === "success" ? CircleCheck : order.status === "pending" ? Clock3 : order.status === "refund" ? RotateCcw : CircleAlert,
    facts: [["类型", typeConfig[order.type]?.label || "历史活动"], ["应用", order.app_name || "—"], ["付款方", order.payer_username || "—"], ["收款方", order.payee_username || "—"], ["交易时间", hasTradeTime ? formatDateTime(order.trade_time) : "—"]],
    extraFacts: [["业务单号", order.merchant_order_no || "—"], ["创建时间", formatDateTime(order.created_at)], ["订单过期时间", formatDateTime(order.expires_at)], ["备注", order.remark || "—"]],
  }
}

export function OrderReceiptDialog({ order }: { order: Order }) {
  return <Dialog>
    <DialogTrigger asChild><Button variant="ghost" size="icon" className="size-7" aria-label="查看订单详情" title="查看订单详情"><Banknote aria-hidden="true" /></Button></DialogTrigger>
    <DialogContent className="credit-reference wallet-receipt-dialog" showCloseButton={false}>
      <WalletReceiptCard data={orderReceiptData(order)}
        title={<DialogTitle className="wallet-receipt-title">订单详情</DialogTitle>}
        description={<DialogDescription className="sr-only">积分交易小票，包含订单金额、状态和交易信息。</DialogDescription>}
        close={<DialogClose asChild><Button variant="ghost" size="icon" className="wallet-receipt-close" aria-label="关闭订单详情"><X /></Button></DialogClose>} />
    </DialogContent>
  </Dialog>
}
