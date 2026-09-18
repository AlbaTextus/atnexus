"use client"
// Preview boundary: the original table is copied intact. Details read the mock
// record; the preview cannot submit dispute or refund changes to a real payment service.
import type { Order } from "@/features/credit-reference/lib/services"
import { OrderReceiptDialog } from "./order-receipt"

export function OrderDetailDialog({ order }: { order: Order }) {
  return <OrderReceiptDialog order={order} />
}
