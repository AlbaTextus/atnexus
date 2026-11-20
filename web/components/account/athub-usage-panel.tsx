"use client"

import { useMemo } from "react"
import { Activity, CheckCircle2, XCircle } from "lucide-react"
import { ATHubUsageChart, type UsagePoint } from "@/components/account/athub-usage-chart"
import { APIRequestCode } from "@/components/account/api-request-code"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, StatCard } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"

type CallQuota = { total_call_limit: number | null; remaining_calls: number | null }
export type ATHubUsage = {
  total_calls: number; successful_calls: number; failed_calls: number; pending_calls: number
  endpoints: { method: string; path: string; status: number | null; calls: number }[]
  trend?: UsagePoint[]; from: string; until: string; bucket_seconds: number
  quota?: CallQuota
}
const number = (value: number) => value.toLocaleString("zh-CN")

function RemainingCallsCard({ quota }: { quota?: CallQuota }) {
  const unlimited = quota?.total_call_limit === null
  const remaining = quota?.remaining_calls ?? 0
  const percent = !quota ? 0 : unlimited ? 100 : quota.total_call_limit! > 0
    ? Math.min(100, Math.max(0, remaining / quota.total_call_limit! * 100)) : 0
  const label = !quota ? "暂时无法加载额度" : unlimited ? "不限次数" : `剩余 ${number(remaining)} 次`
  const display = !quota ? "—" : percent > 0 && percent < 1 ? "<1%" : percent > 99 && percent < 100 ? ">99%" : `${Math.round(percent)}%`
  const color = percent > 50 ? "#31976c" : percent >= 20 ? "#d8a22b" : "#d65050"

  return <Card className="flex-row items-center justify-between gap-4 border border-border px-4">
    <span className="text-sm text-muted-foreground">剩余次数</span>
    <TooltipProvider><Tooltip>
      <TooltipTrigger render={<div />} tabIndex={0}
        className="relative size-24 shrink-0 rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        aria-label={`剩余调用额度：${label}`}>
        <svg viewBox="0 0 100 100" className="size-full" role="progressbar" aria-label="剩余调用额度"
          aria-valuemin={0} aria-valuemax={100} aria-valuenow={quota ? percent : undefined} aria-valuetext={label}>
          <circle cx="50" cy="50" r="41" fill="none" stroke={quota && percent === 0 ? color : "var(--muted)"}
            strokeOpacity={quota && percent === 0 ? 0.25 : 1} strokeWidth="7" />
          {percent > 0 && <circle cx="50" cy="50" r="41" fill="none" stroke={color} strokeWidth="7"
            strokeLinecap="round" pathLength="100" strokeDasharray={`${percent} 100`} transform="rotate(-90 50 50)" />}
        </svg>
        <span aria-hidden="true" className="pointer-events-none absolute inset-0 flex items-center justify-center text-xl font-semibold tabular-nums">{display}</span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip></TooltipProvider>
  </Card>
}

export function ATHubUsagePanel({ usage, days, loading, onDaysChange, showQuota = false }: {
  usage: ATHubUsage | null
  days: number
  loading: boolean
  onDaysChange: (days: number) => void
  showQuota?: boolean
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
    {loading ? <div className="space-y-5"><div className={`grid gap-4 ${showQuota ? "grid-cols-4" : "grid-cols-3"}`}>{Array.from({ length: showQuota ? 4 : 3 }, (_, value) => <Skeleton key={value} className="h-32" />)}</div><Skeleton className="h-96 w-full" /></div> : usage && <>
      <div className={`grid gap-4 ${showQuota ? "grid-cols-4" : "grid-cols-3"}`}>
        <StatCard className="border border-border" label="总调用" value={number(usage.total_calls)} icon={<Activity />} tone="blue" />
        <StatCard className="border border-border" label="成功调用" value={number(usage.successful_calls)} icon={<CheckCircle2 />} tone="green" />
        <StatCard className="border border-border" label="失败调用" value={number(usage.failed_calls)} icon={<XCircle />} tone="orange" />
        {showQuota && <RemainingCallsCard quota={usage.quota} />}
      </div>
      <Card><CardHeader><CardTitle>调用趋势</CardTitle></CardHeader><CardContent>
        {usage.total_calls ? <ATHubUsageChart trend={usage.trend || []} total={usage.total_calls} from={usage.from} until={usage.until} bucketSeconds={usage.bucket_seconds} /> :
          <div className="flex h-72 items-center justify-center text-sm text-muted-foreground">所选时间范围暂无调用记录</div>}
      </CardContent></Card>
      <Card><CardHeader><CardTitle>接口调用</CardTitle></CardHeader><CardContent>
        {!endpoints.length ? <p className="py-6 text-center text-muted-foreground">暂无接口调用</p> : <div className="space-y-4">{endpoints.map(item => <div key={`${item.method}:${item.path}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
          <APIRequestCode method={item.method} path={item.path} className="min-w-0 flex-1" />
          <span className="w-14 text-right tabular-nums">{number(item.calls)}</span>
        </div>)}</div>}
      </CardContent></Card>
    </>}
  </div>
}
