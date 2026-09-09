// Copied from linux-do/credit, Apache-2.0. Only imports adapted for this preview.
"use client"

import { BalanceSummary } from "@/features/credit-reference/components/common/balance/balance-summary"
import { BalanceTable } from "@/features/credit-reference/components/common/balance/balance-table"

/**
 * 积分余额主页面组件
 *
 * 复用参考项目的积分总览、摘要和活动表，嵌入账户中心钱包。
 */
export function BalanceMain() {
  return (
    <div className="flex min-w-0 flex-col gap-8 py-6">
      <div className="flex w-full justify-center" data-slot="wallet-overview-grid">
        <BalanceSummary />
      </div>
      <BalanceTable />
    </div>
  )
}
