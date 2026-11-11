"use client"

import { useMemo } from "react"
import { Activity, CheckCircle2, XCircle } from "lucide-react"
import { ATHubUsageChart, type UsagePoint } from "@/components/athub-usage-chart"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, StatCard } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

export type ATHubUsage = {
  total_calls: number; successful_calls: number; failed_calls: number; pending_calls: number
  endpoints: { method: string; path: string; port: number; status: number | null; calls: number }[]
  trend?: UsagePoint[]; from: string; until: string; bucket_seconds: number
}
const number = (value: number) => value.toLocaleString("zh-CN")

export function ATHubUsagePanel({ usage, days, loading, onDaysChange }: {
  usage: ATHubUsage | null
  days: number
  loading: boolean
  onDaysChange: (days: number) => void
}) {
  const endpoints = useMemo(() => {
    const totals = new Map<string, { method: string; path: string; calls: number }>()
    for (const item of usage?.endpoints || []) {
      const id = `${item.method}:${item.path}`
      const total = totals.get(id)
      totals.set(id, { ...item, calls: (total?.calls || 0) + item.calls })
    }
    return Array.from(totals.values()).sort((a, b) => b.calls - a.calls || a.path.localeCompare(b.path))
  }, [usage])

  return <div className="space-y-5" aria-busy={loading}>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h2 className="text-lg font-semibold">调用统计</h2>
      <div className="flex gap-1 rounded-lg bg-muted p-1" role="group" aria-label="统计时间范围">
        {[1, 7, 30].map(value => <Button key={value} size="sm" variant={days === value ? "outline" : "ghost"} aria-pressed={days === value} onClick={() => onDaysChange(value)}>{value === 1 ? "近 24 小时" : `近 ${value} 天`}</Button>)}
      </div>
    </div>
    {loading ? <div className="space-y-5"><div className="grid grid-cols-2 gap-4 lg:grid-cols-4">{[0, 1, 2, 3].map(value => <Skeleton key={value} className="h-28" />)}</div><Skeleton className="h-96 w-full" /></div> : usage && <>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard className="border border-border" label="总调用" value={number(usage.total_calls)} icon={<Activity />} tone="blue" />
        <StatCard className="border border-border" label="成功调用" value={number(usage.successful_calls)} icon={<CheckCircle2 />} tone="green" />
        <StatCard className="border border-border" label="失败调用" value={number(usage.failed_calls)} icon={<XCircle />} tone="orange" />
        <StatCard className="border border-border" label="处理中" value={number(usage.pending_calls)} tone="muted" />
      </div>
      <Card><CardHeader><CardTitle>调用趋势</CardTitle></CardHeader><CardContent>
        {usage.total_calls ? <ATHubUsageChart trend={usage.trend || []} total={usage.total_calls} from={usage.from} until={usage.until} bucketSeconds={usage.bucket_seconds} /> :
          <div className="flex h-72 items-center justify-center text-sm text-muted-foreground">所选时间范围暂无调用记录</div>}
      </CardContent></Card>
      <Card><CardHeader><CardTitle>接口调用</CardTitle></CardHeader><CardContent>
        {!endpoints.length ? <p className="py-6 text-center text-muted-foreground">暂无接口调用</p> : <div className="space-y-4">{endpoints.map(item => <div key={`${item.method}:${item.path}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
          <Badge variant="outline" className="min-w-14 justify-center">{item.method}</Badge>
          <code className="min-w-0 flex-1 break-all text-xs sm:text-sm">{item.path}</code>
          <span className="w-14 text-right tabular-nums">{number(item.calls)}</span>
        </div>)}</div>}
      </CardContent></Card>
    </>}
  </div>
}
