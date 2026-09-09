// Adapted from linux-do/credit frontend/components/common/home/data-panel.tsx,
// Apache-2.0. Adapted to a compact income/expense summary with the wallet adapter.
"use client"

import * as React from "react"
import { Card, CardHeader, CardTitle, CardContent } from "../../ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "../../ui/button"
import { useUser } from "../../../contexts/user-context"
import { previewRequest } from "../../../lib/services"
import type { DailyStats } from "../../../lib/daily-stats"

const numberFormat = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 })

export function BalanceTrend() {
  const { user, loading: userLoading } = useUser()
  const [stats, setStats] = React.useState<DailyStats | null>(null)
  const [error, setError] = React.useState(false)
  const [revision, setRevision] = React.useState(0)

  React.useEffect(() => {
    if (!user) return
    let active = true
    previewRequest<DailyStats>("daily-stats")
      .then(data => { if (active) setStats(data) })
      .catch(() => { if (active) setError(true) })
    return () => { active = false }
  }, [user, revision])

  return (
    <Card className="min-w-0 gap-4" aria-label="近7天积分收支" aria-busy={!error && (!stats || userLoading)}>
      <CardHeader>
        <CardTitle><h3>近 7 天收支</h3></CardTitle>
      </CardHeader>
      <CardContent>
        {error ? (
          <div className="flex flex-wrap items-center justify-between gap-3" role="alert">
            <p className="text-xs text-muted-foreground">收支数据加载失败</p>
            <Button variant="outline" size="sm" onClick={() => { setError(false); setRevision(value => value + 1) }}>重试</Button>
          </div>
        ) : (
          <dl className="grid grid-cols-2 gap-4">
            {[
              { label: "收入", value: stats?.total_income, className: "wallet-income" },
              { label: "支出", value: stats?.total_expense, className: "wallet-expense" },
            ].map(item => (
              <div key={item.label} className="flex min-w-0 flex-col gap-2">
                <dt className="text-xs text-muted-foreground">{item.label}</dt>
                <dd className={item.className}>
                  {!stats || userLoading ? <Skeleton className="h-8 w-20" /> : (
                    <span className="break-all text-2xl font-semibold tabular-nums">{numberFormat.format(Number(item.value || "0"))}</span>
                  )}
                </dd>
              </div>
            ))}
          </dl>
        )}
      </CardContent>
    </Card>
  )
}
