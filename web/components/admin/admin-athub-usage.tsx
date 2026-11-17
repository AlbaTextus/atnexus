"use client"

import { useEffect, useState } from "react"
import { AlertCircle, RefreshCw } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/admin/applications"
import { ATHubUsagePanel, type ATHubUsage } from "@/components/account/athub-usage-panel"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { UserAvatar } from "@/components/shared/user-avatar"

type RankedUser = {
  user_id: string; display_name: string | null; avatar_url: string | null
  total_calls: number; successful_calls: number; failed_calls: number; pending_calls: number; last_used_at: string | null
}
type Dashboard = { usage: ATHubUsage; users: RankedUser[]; total_users: number }
const pageSize = 10
const number = (value: number) => value.toLocaleString("zh-CN")
const date = (value: string | null) => value ? new Date(value).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }) : "—"

function Pagination({ label, offset, hasNext, loading, onChange }: {
  label: string; offset: number; hasNext: boolean; loading: boolean; onChange: (offset: number) => void
}) {
  return <nav className="mt-4 flex items-center justify-end gap-3" aria-label={label}>
    <Button variant="outline" size="sm" disabled={loading || offset === 0} onClick={() => onChange(Math.max(0, offset - pageSize))}>上一页</Button>
    <span className="text-xs text-muted-foreground" aria-live="polite">第 {offset / pageSize + 1} 页</span>
    <Button variant="outline" size="sm" disabled={loading || !hasNext || offset >= 1000000} onClick={() => onChange(offset + pageSize)}>下一页</Button>
  </nav>
}

export function AdminATHubUsage() {
  const [dashboard, setDashboard] = useState<Dashboard | null>(null)
  const [days, setDays] = useState(7)
  const [rankOffset, setRankOffset] = useState(0)
  const [revision, setRevision] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  useEffect(() => {
    let active = true
    setLoading(true); setError(""); setDashboard(null)
    void api<Dashboard>(`/api/v1/admin/athub/dashboard?days=${days}&rank_offset=${rankOffset}`)
      .then(result => { if (active) setDashboard(result) })
      .catch(reason => { if (active) setError(managementError(reason)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [days, rankOffset, revision])

  function changeDays(value: number) {
    setDays(value); setRankOffset(0)
  }

  return <section className="space-y-7" aria-labelledby="admin-usage-title">
    <div className="flex flex-wrap items-center justify-between gap-4">
      <h1 id="admin-usage-title" className="text-2xl font-semibold tracking-tight">全站调用</h1>
      <Button variant="outline" size="icon" aria-label="刷新全站调用统计" disabled={loading} onClick={() => setRevision(value => value + 1)}><RefreshCw /></Button>
    </div>
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button variant="link" onClick={() => setRevision(value => value + 1)}>重试</Button></AlertDescription></Alert>}
    <ATHubUsagePanel usage={dashboard?.usage || null} days={days} loading={loading} onDaysChange={changeDays} />
    <Card aria-busy={loading}><CardHeader><CardTitle>用户调用排行榜</CardTitle></CardHeader><CardContent>
      {loading ? <Skeleton className="h-64 w-full" /> : dashboard && <>
        {!dashboard.users.length ? <p className="py-6 text-center text-muted-foreground">所选时间范围暂无用户调用</p> :
          <div className="overflow-x-auto"><table className="w-full text-left text-sm">
            <thead className="text-xs text-muted-foreground"><tr className="border-b">{["排名", "用户", "总调用", "成功", "失败", "处理中", "最近调用"].map(label => <th key={label} scope="col" className="whitespace-nowrap px-3 pb-3 font-normal first:pl-0 last:pr-0">{label}</th>)}</tr></thead>
            <tbody>{dashboard.users.map((user, index) => <tr key={user.user_id} className="border-b last:border-0">
              <td className="py-3 pr-3 tabular-nums">{number(rankOffset + index + 1)}</td>
              <td className="px-3"><div className="flex min-w-28 items-center gap-3"><UserAvatar src={user.avatar_url} name={user.display_name} decorative className="size-8 shrink-0" /><span className="font-medium">{user.display_name || "用户"}</span></div></td>
              <td className="px-3 font-medium tabular-nums">{number(user.total_calls)}</td>
              <td className="px-3 tabular-nums">{number(user.successful_calls)}</td>
              <td className="px-3 tabular-nums">{number(user.failed_calls)}</td>
              <td className="px-3 tabular-nums">{number(user.pending_calls)}</td>
              <td className="whitespace-nowrap pl-3 text-xs text-muted-foreground">{date(user.last_used_at)}</td>
            </tr>)}</tbody>
          </table></div>}
        <Pagination label="用户排行榜分页" offset={rankOffset} hasNext={rankOffset + pageSize < dashboard.total_users} loading={loading} onChange={setRankOffset} />
      </>}
    </CardContent></Card>
  </section>
}
