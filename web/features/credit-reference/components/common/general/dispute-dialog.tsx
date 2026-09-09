"use client"
// Preview boundary: the original table is copied intact. Details read the mock
// record; dispute/refund controls cannot submit to a real payment service.
import type { Order } from "@/features/credit-reference/lib/services"
import { Button } from "@/features/credit-reference/components/ui/button"
import { OrderReceiptDialog } from "./order-receipt"

export function OrderDetailDialog({ order }: { order: Order }) {
  return <OrderReceiptDialog order={order} />
}
function PreviewAction({ label }: { label: string }) { return <Button variant="ghost" size="sm" className="h-6 px-2 text-[11px]" disabled title="模拟数据不支持交易变更">{label}</Button> }
export function CreateDisputeDialog(_: { order: Order }) { return <PreviewAction label="争议" /> }
export function ViewDisputeHistoryDialog(_: { order: Order; viewer: string }) { return <PreviewAction label="争议记录" /> }
export function RefundReviewDialog(_: { order: Order }) { return <PreviewAction label="处理" /> }
export function CancelDisputeDialog(_: { order: Order }) { return <PreviewAction label="取消争议" /> }
