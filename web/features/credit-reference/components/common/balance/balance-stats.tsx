"use client"

// Combines the reference IncomeStatsCard / ExpenseStatsCard into one daily chart.
import { useEffect, useState } from "react"
import { motion, useReducedMotion } from "framer-motion"
import { CountingNumber } from "@/features/credit-reference/components/animate-ui/primitives/texts/counting-number"
import { previewRequest } from "@/features/credit-reference/lib/services"
import type { DailyStats } from "@/features/credit-reference/lib/daily-stats"
import { OverviewCard } from "@/features/credit-reference/components/common/home/overview-card"
import { Skeleton } from "@/components/ui/skeleton"

export function BalanceStats() {
  const reducedMotion = useReducedMotion()
  const [stats, setStats] = useState<DailyStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let active = true
    setLoading(true); setError(false)
    previewRequest<DailyStats>("daily-stats").then(value => { if (active) setStats(value) })
      .catch(() => { if (active) setError(true) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [revision])
  const maximum = Math.max(0, ...(stats?.days.map(day => Number(day.income) + Number(day.expense)) ?? []))
  const width = (value: string) => maximum > 0 ? `${Number(value) / maximum * 100}%` : "0%"
  return <motion.section aria-label="7天收支统计" aria-busy={loading} initial={reducedMotion ? false : { opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} whileHover={reducedMotion ? undefined : { y: -2 }} transition={{ duration: reducedMotion ? 0 : 0.3, ease: "easeOut" }}>
    <OverviewCard
      className="gap-4 py-4 [&>[data-slot=card-header]]:px-4 [&>[data-slot=card-content]]:px-4"
      title="7天收支统计"
      loading={loading}
      onRefresh={() => setRevision(v => v + 1)}
      headerExtra={stats && <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 pb-1 text-2xl font-medium tracking-tight tabular-nums">
        <span className="wallet-income" data-testid="income-total" aria-label={`收入 ${stats.total_income}`}>{reducedMotion ? stats.total_income : <CountingNumber number={Number(stats.total_income)} decimalPlaces={0} transition={{ stiffness: 120, damping: 24 }} />}</span>
        <span className="wallet-expense" data-testid="expense-total" aria-label={`支出 ${stats.total_expense}`}>{reducedMotion ? stats.total_expense : <CountingNumber number={Number(stats.total_expense)} decimalPlaces={0} transition={{ stiffness: 120, damping: 24 }} />}</span>
      </div>}
    >
      {error ? <p role="alert" className="flex h-60 items-center justify-center text-xs text-muted-foreground">加载失败，请点击右上角刷新重试。</p> : !stats ? <Skeleton className="h-60 w-full" /> : <div className="hide-scrollbar h-60 w-full overflow-y-auto" tabIndex={0} role="region" aria-label="收支统计明细">
        <ol className="flex flex-col gap-1.5 pr-2" aria-label="每日收入与支出">{stats.days.map((day, index) => <motion.li key={day.date} className="flex flex-col gap-1 rounded-md" initial={reducedMotion ? false : { opacity: 0, y: 3 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: reducedMotion ? 0 : 0.3, delay: reducedMotion ? 0 : index * 0.035 }}>
          <div className="flex items-center justify-between gap-2 text-xs leading-4">
            <time dateTime={day.date} className="text-muted-foreground">{Number(day.date.slice(5, 7))}/{Number(day.date.slice(8))}</time>
            <span className="ml-auto flex items-center gap-2 font-medium tabular-nums"><span className="wallet-income" aria-label={`收入 ${day.income}`}>{day.income}</span><span className="wallet-expense" aria-label={`支出 ${day.expense}`}>{day.expense}</span></span>
          </div>
          <div data-slot="daily-cashflow-track" className="flex h-2 overflow-hidden rounded-full bg-muted/75" aria-hidden="true">
            <motion.div className="wallet-income-bar h-full shrink-0 rounded-l-full" initial={reducedMotion ? false : { width: "0%" }} animate={{ width: width(day.income) }} transition={reducedMotion ? { duration: 0 } : { type: "spring", stiffness: 110, damping: 22, delay: index * 0.035 }} />
            <div className="min-w-0 flex-1" />
            <motion.div className="wallet-expense-bar h-full shrink-0 rounded-r-full" initial={reducedMotion ? false : { width: "0%" }} animate={{ width: width(day.expense) }} transition={reducedMotion ? { duration: 0 } : { type: "spring", stiffness: 110, damping: 22, delay: index * 0.035 }} />
          </div>
        </motion.li>)}</ol>
      </div>}
    </OverviewCard>
  </motion.section>
}
