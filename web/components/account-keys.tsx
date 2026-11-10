"use client"

import { useEffect, useMemo, useState, type FormEvent } from "react"
import { Activity, AlertCircle, Check, CheckCircle2, Copy, KeyRound, Plus, RefreshCw, XCircle } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import { ATHubUsageChart, portColors, type UsagePoint } from "@/components/athub-usage-chart"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, StatCard } from "@/components/ui/card"
import { ConfirmDialog, Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"

type APIKey = {
  id: string; name: string; prefix: string; created_at: string; expires_at: string | null
  revoked_at: string | null; last_used_at: string | null; total_calls: number
}
type UsageGroup = { method: string; path: string; port: number; status: number | null; calls: number }
type Usage = {
  total_calls: number; successful_calls: number; failed_calls: number; pending_calls: number
  endpoints: UsageGroup[]; trend?: UsagePoint[]; from: string; until: string; bucket_seconds: number
}
type Call = { id: string; key_id: string | null; method: string; path: string; port: number; status: number | null; duration_ms: number | null; started_at: string }
const keyPath = "/api/v1/account/key"
const usagePath = "/api/v1/account/athub"
const pageSize = 10
const number = (value: number) => value.toLocaleString("zh-CN")
const date = (value: string | null) => value ? new Date(value).toLocaleString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }) : "—"
function errorMessage(reason: unknown) {
  if (reason instanceof ApiError) {
    if (reason.status === 401) return "登录已过期，请重新登录。"
    if (reason.code === "api_key_limit") return "最多保留 20 个有效密钥，请先撤销不再使用的密钥。"
  }
  return "暂时无法完成操作，请稍后重试。"
}
function keyState(key: APIKey) {
  return key.revoked_at ? "已撤销" : key.expires_at && Date.parse(key.expires_at) <= Date.now() ? "已过期" : "有效"
}

export function AccountKeys() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [usage, setUsage] = useState<Usage | null>(null)
  const [days, setDays] = useState(7)
  const [revision, setRevision] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [expiry, setExpiry] = useState("")
  const [secret, setSecret] = useState("")
  const [copied, setCopied] = useState(false)
  const [createError, setCreateError] = useState("")
  const [busy, setBusy] = useState(false)
  const [revoke, setRevoke] = useState<APIKey | null>(null)
  const [revokeError, setRevokeError] = useState("")
  const [calls, setCalls] = useState<Call[]>([])
  const [offset, setOffset] = useState(0)
  const [callsLoading, setCallsLoading] = useState(true)
  const [callsError, setCallsError] = useState("")
  const [hasNext, setHasNext] = useState(false)

  useEffect(() => {
    let active = true
    setLoading(true); setError("")
    void Promise.all([
      api<{ keys: APIKey[] }>(keyPath),
      api<Usage>(`${usagePath}/usage?days=${days}`),
    ]).then(([keyData, usageData]) => {
      if (active) { setKeys(keyData.keys); setUsage(usageData) }
    }).catch(reason => { if (active) setError(errorMessage(reason)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [days, revision])

  useEffect(() => {
    let active = true
    setCallsLoading(true); setCallsError("")
    // Request one extra record to determine pagination without a separate count.
    void api<{ calls: Call[] }>(`${usagePath}/calls?limit=${pageSize + 1}&offset=${offset}`).then(data => {
      if (active) { setCalls(data.calls.slice(0, pageSize)); setHasNext(data.calls.length > pageSize) }
    }).catch(reason => { if (active) setCallsError(errorMessage(reason)) })
      .finally(() => { if (active) setCallsLoading(false) })
    return () => { active = false }
  }, [offset, revision])

  const ports = useMemo(() => {
    const totals = new Map<number, number>()
    for (const item of usage?.endpoints || []) totals.set(item.port, (totals.get(item.port) || 0) + item.calls)
    return Array.from(totals, ([port, calls]) => ({ port, calls })).sort((a, b) => b.calls - a.calls || a.port - b.port)
  }, [usage])
  const endpoints = useMemo(() => {
    const totals = new Map<string, { method: string; path: string; port: number; calls: number }>()
    for (const item of usage?.endpoints || []) {
      const id = `${item.port}:${item.method}:${item.path}`
      const total = totals.get(id)
      totals.set(id, { ...item, calls: (total?.calls || 0) + item.calls })
    }
    return Array.from(totals.values()).sort((a, b) => b.calls - a.calls || a.path.localeCompare(b.path))
  }, [usage])
  const keyNames = new Map(keys.map(key => [key.id, key.name]))

  function closeCreate(open: boolean) {
    if (busy) return
    setCreateOpen(open)
    if (!open) { setSecret(""); setName(""); setExpiry(""); setCopied(false); setCreateError("") }
  }
  async function createKey(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    setBusy(true); setCreateError("")
    try {
      const result = await api<{ key: APIKey; api_key: string }>(keyPath, {
        method: "POST", body: JSON.stringify({ name: name.trim(), ...(expiry ? { expires_at: new Date(expiry).toISOString() } : {}) }),
      })
      setSecret(result.api_key); setRevision(value => value + 1)
    } catch (reason) { setCreateError(errorMessage(reason)) }
    finally { setBusy(false) }
  }
  async function copyKey() {
    try { await navigator.clipboard.writeText(secret); setCopied(true); feedback.success("密钥已复制") }
    catch { setCreateError("复制失败，请手动选中并复制密钥。") }
  }
  async function revokeKey() {
    if (!revoke || busy) return
    setBusy(true); setRevokeError("")
    try {
      await api(`${keyPath}/${revoke.id}`, { method: "DELETE" })
      setRevoke(null); setRevision(value => value + 1); feedback.success("密钥已撤销")
    } catch (reason) { setRevokeError(errorMessage(reason)) }
    finally { setBusy(false) }
  }

  return <section className="space-y-7" aria-labelledby="account-keys-title">
    <div className="flex flex-wrap items-center justify-between gap-4">
      <h1 id="account-keys-title" className="text-2xl font-semibold tracking-tight">API 密钥</h1>
      <div className="flex items-center gap-2">
        <Button variant="outline" size="icon" aria-label="刷新密钥和调用统计" disabled={loading || callsLoading} onClick={() => setRevision(value => value + 1)}><RefreshCw /></Button>
        <Button onClick={() => setCreateOpen(true)}><Plus />创建密钥</Button>
      </div>
    </div>
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button variant="link" onClick={() => setRevision(value => value + 1)}>重试</Button></AlertDescription></Alert>}

    <Card>
      <CardHeader><CardTitle>我的密钥</CardTitle></CardHeader>
      <CardContent>
        {loading && !keys.length ? <Skeleton className="h-28 w-full" /> : !keys.length ? <div className="flex flex-col items-center gap-3 py-10 text-muted-foreground"><KeyRound className="size-7" /><p>暂无 API 密钥</p><Button variant="outline" onClick={() => setCreateOpen(true)}>创建密钥</Button></div> :
          <div className="overflow-x-auto"><table className="w-full text-left text-sm">
            <thead className="text-xs text-muted-foreground"><tr className="border-b">{["名称 / Key", "状态", "累计调用", "最近使用", "到期时间", "操作"].map(label => <th key={label} scope="col" className="whitespace-nowrap px-3 pb-3 font-normal first:pl-0 last:pr-0">{label}</th>)}</tr></thead>
            <tbody>{keys.map(key => <tr key={key.id} className="border-b last:border-0">
              <td className="min-w-44 py-4 pr-3"><div className="max-w-64 truncate font-medium" title={key.name}>{key.name}</div><code className="mt-1 block text-xs text-muted-foreground">{key.prefix}••••••••</code></td>
              <td className="px-3"><Badge variant={keyState(key) === "有效" ? "secondary" : "outline"}>{keyState(key)}</Badge></td>
              <td className="px-3 tabular-nums">{number(key.total_calls)}</td>
              <td className="whitespace-nowrap px-3 text-muted-foreground">{date(key.last_used_at)}</td>
              <td className="whitespace-nowrap px-3 text-muted-foreground">{key.expires_at ? date(key.expires_at) : "永不过期"}</td>
              <td className="pl-3 text-right"><Button variant="ghost" size="sm" disabled={Boolean(key.revoked_at)} onClick={() => { setRevoke(key); setRevokeError("") }}>撤销</Button></td>
            </tr>)}</tbody>
          </table></div>}
      </CardContent>
    </Card>

    <div className="space-y-5" aria-busy={loading}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">调用统计</h2>
        <div className="flex gap-1 rounded-lg bg-muted p-1" role="group" aria-label="统计时间范围">
          {[1, 7, 30].map(value => <Button key={value} size="sm" variant={days === value ? "outline" : "ghost"} aria-pressed={days === value} onClick={() => setDays(value)}>{value === 1 ? "近 24 小时" : `近 ${value} 天`}</Button>)}
        </div>
      </div>
      {loading ? <div className="space-y-5"><div className="grid grid-cols-2 gap-4 lg:grid-cols-4">{[0, 1, 2, 3].map(value => <Skeleton key={value} className="h-28" />)}</div><Skeleton className="h-96 w-full" /></div> : usage && <>
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard label="总调用" value={number(usage.total_calls)} icon={<Activity />} tone="blue" />
          <StatCard label="成功调用" value={number(usage.successful_calls)} icon={<CheckCircle2 />} tone="green" />
          <StatCard label="失败调用" value={number(usage.failed_calls)} icon={<XCircle />} tone="orange" />
          <StatCard label="处理中" value={number(usage.pending_calls)} tone="muted" />
        </div>
        <Card><CardHeader><CardTitle>端口调用趋势</CardTitle></CardHeader><CardContent>
          {ports.length ? <ATHubUsageChart trend={usage.trend || []} ports={ports} from={usage.from} until={usage.until} bucketSeconds={usage.bucket_seconds} /> :
            <div className="flex h-72 items-center justify-center text-sm text-muted-foreground">所选时间范围暂无调用记录</div>}
        </CardContent></Card>
        <Card><CardHeader><CardTitle>端口调用排行</CardTitle></CardHeader><CardContent>
          {!ports.length ? <p className="py-6 text-center text-muted-foreground">暂无端口调用</p> : <div className="space-y-5">{ports.map((port, index) => <div key={port.port} className="grid grid-cols-[5rem_1fr_4rem] items-center gap-4 text-sm">
            <span className="font-medium">:{port.port}</span>
            <div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full min-w-1 rounded-full" style={{ width: `${port.calls / ports[0].calls * 100}%`, backgroundColor: portColors[index % portColors.length] }} /></div>
            <span className="text-right tabular-nums">{number(port.calls)}</span>
          </div>)}</div>}
        </CardContent></Card>
        <Card><CardHeader><CardTitle>接口调用</CardTitle></CardHeader><CardContent>
          {!endpoints.length ? <p className="py-6 text-center text-muted-foreground">暂无接口调用</p> : <div className="space-y-4">{endpoints.map(item => <div key={`${item.port}:${item.method}:${item.path}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
            <Badge variant="outline" className="min-w-14 justify-center">{item.method}</Badge>
            <code className="min-w-0 flex-1 break-all text-xs sm:text-sm">{item.path}</code>
            <span className="text-muted-foreground">:{item.port}</span><span className="w-14 text-right tabular-nums">{number(item.calls)}</span>
          </div>)}</div>}
        </CardContent></Card>
      </>}
    </div>

    <Card><CardHeader><CardTitle>最近调用</CardTitle></CardHeader><CardContent>
      {callsError ? <Alert variant="destructive"><AlertCircle /><AlertDescription>{callsError}<Button variant="link" onClick={() => setRevision(value => value + 1)}>重试</Button></AlertDescription></Alert> : callsLoading ? <Skeleton className="h-40 w-full" /> : !calls.length ? <p className="py-6 text-center text-muted-foreground">暂无调用记录</p> :
        <div className="overflow-x-auto"><table className="w-full text-left text-sm">
          <thead className="text-xs text-muted-foreground"><tr className="border-b">{["时间", "密钥", "接口", "端口", "状态", "耗时"].map(label => <th key={label} scope="col" className="whitespace-nowrap px-3 pb-3 font-normal first:pl-0 last:pr-0">{label}</th>)}</tr></thead>
          <tbody>{calls.map(call => <tr key={call.id} className="border-b last:border-0">
            <td className="whitespace-nowrap py-3 pr-3 text-xs text-muted-foreground">{date(call.started_at)}</td>
            <td className="max-w-36 truncate px-3" title={call.key_id ? keyNames.get(call.key_id) : undefined}>{call.key_id ? keyNames.get(call.key_id) || "已删除的密钥" : "—"}</td>
            <td className="whitespace-nowrap px-3 font-mono text-xs">{call.method} {call.path}</td>
            <td className="px-3 tabular-nums">{call.port}</td>
            <td className="px-3"><Badge variant={call.status && call.status >= 400 ? "outline" : "secondary"}>{call.status ?? "处理中"}</Badge></td>
            <td className="whitespace-nowrap pl-3 text-right tabular-nums">{call.duration_ms === null ? "—" : `${call.duration_ms} ms`}</td>
          </tr>)}</tbody>
        </table></div>}
      <div className="mt-4 flex items-center justify-end gap-3">
        <Button variant="outline" size="sm" disabled={callsLoading || offset === 0} onClick={() => setOffset(value => Math.max(0, value - pageSize))}>上一页</Button>
        <span className="text-xs text-muted-foreground" aria-live="polite">第 {offset / pageSize + 1} 页</span>
        <Button variant="outline" size="sm" disabled={callsLoading || !hasNext || offset >= 1000000} onClick={() => setOffset(value => value + pageSize)}>下一页</Button>
      </div>
    </CardContent></Card>

    <Dialog open={createOpen} onOpenChange={closeCreate}>
      <DialogContent className="sm:max-w-md" showCloseButton={!busy} aria-busy={busy}>
        <DialogHeader><DialogTitle>{secret ? "密钥已创建" : "创建 API 密钥"}</DialogTitle>
          {secret && <DialogDescription>完整密钥仅显示这一次，请复制并妥善保存。</DialogDescription>}
        </DialogHeader>
        {createError && <Alert variant="destructive"><AlertCircle /><AlertDescription>{createError}</AlertDescription></Alert>}
        {secret ? <>
          <div className="flex items-start gap-2 rounded-lg border bg-muted/40 p-3"><code className="min-w-0 flex-1 select-all break-all text-sm">{secret}</code><Button variant="ghost" size="icon-sm" aria-label={copied ? "已复制密钥" : "复制密钥"} onClick={copyKey}>{copied ? <Check /> : <Copy />}</Button></div>
          <DialogFooter><Button onClick={() => closeCreate(false)}>完成</Button></DialogFooter>
        </> : <form onSubmit={createKey} className="space-y-5">
          <div className="space-y-2"><label htmlFor="key-name" className="font-medium">名称</label><Input id="key-name" value={name} onChange={event => setName(event.target.value)} maxLength={100} required disabled={busy} placeholder="例如：小说采集" /></div>
          <div className="space-y-2"><label htmlFor="key-expiry" className="font-medium">到期时间（可选）</label><Input id="key-expiry" type="datetime-local" value={expiry} onChange={event => setExpiry(event.target.value)} disabled={busy} min={new Date(Date.now() + 60000).toLocaleString("sv-SE").replace(" ", "T").slice(0, 16)} /></div>
          <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={() => closeCreate(false)}>取消</Button><Button type="submit" disabled={busy || !name.trim()}>{busy && <Spinner />}创建密钥</Button></DialogFooter>
        </form>}
      </DialogContent>
    </Dialog>
    <ConfirmDialog open={Boolean(revoke)} onOpenChange={open => { if (!open) setRevoke(null) }} title="撤销密钥"
      description={`确定撤销「${revoke?.name || ""}」？使用此密钥的请求将立即失效，已有调用记录会保留。`}
      confirmLabel="撤销密钥" busy={busy} onConfirm={revokeKey}>
      {revokeError && <Alert variant="destructive"><AlertCircle /><AlertDescription>{revokeError}</AlertDescription></Alert>}
    </ConfirmDialog>
  </section>
}
