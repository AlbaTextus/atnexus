// Copied from linux-do/credit, Apache-2.0. Only imports adapted for this preview.
"use client"

import { BalanceSummary } from "@/features/credit-reference/components/common/balance/balance-summary"
import { BalanceTable } from "@/features/credit-reference/components/common/balance/balance-table"
import { BalanceStats } from "@/features/credit-reference/components/common/balance/balance-stats"
import { BalanceDistribution } from "@/features/credit-reference/components/common/balance/balance-distribution"
import { useUser } from "@/features/credit-reference/contexts/user-context"

/**
 * 积分余额主页面组件
 *
 * 复用参考项目的积分总览、摘要和活动表，嵌入账户中心钱包。
 */
export function BalanceMain() {
  const { user, loading } = useUser()

  return (
    <div className="flex min-w-0 flex-col gap-8 py-6">
      <div className="grid w-full max-w-[840px] grid-cols-1 items-start gap-4 md:grid-cols-[minmax(0,1.04fr)_minmax(0,1fr)] xl:max-w-[1120px] xl:grid-cols-[minmax(0,420px)_minmax(0,360px)_minmax(0,1fr)]" data-slot="wallet-overview-grid">
        <BalanceSummary />
        <div className="w-full max-w-[360px] rounded-2xl bg-muted/50 p-1"><BalanceStats /></div>
        <div className="w-full max-w-[320px] self-center justify-self-center md:col-span-2 xl:col-span-1">
          <BalanceDistribution available={Number(user?.available_balance || "0")} pending={Number(user?.pending_balance || "0")} loading={loading} />
        </div>
      </div>
      <section aria-labelledby="recent-activity-title">
        <h2 id="recent-activity-title" className="font-semibold mb-2">近期活动</h2>
        <BalanceTable />
      </section>
    </div>
  )
}
