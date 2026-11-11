"use client"

import { useEffect, useRef, useState } from "react"
import { AlertCircle, Check, Copy, KeyRound, RefreshCw } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import { ATHubUsagePanel, type ATHubUsage } from "@/components/athub-usage-panel"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { ConfirmDialog, Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"

type APIKey = {
  admin_disabled: boolean
  id: string; name: string; prefix: string; created_at: string; expires_at: string | null
  revoked_at: string | null; last_used_at: string | null; total_calls: number
}
type KeyResult = { key: APIKey; api_key?: string }
type Call = { id: string; key_id: string | null; method: string; path: string; port: number; status: number | null; duration_ms: number | null; started_at: string }
const keyPath = "/api/v1/account/key"
const usagePath = "/api/v1/account/athub"
const pageSize = 10
const date = (value: string | null) => value ? new Date(value).toLocaleString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }) : "—"
function errorMessage(reason: unknown) {
  if (reason instanceof ApiError) {
    if (reason.status === 401) return "登录已过期，请重新登录。"
    if (reason.code === "athub_admin_disabled") return "API 密钥已被管理员禁用，无法自行重置或启用。"
  }
  return "暂时无法完成操作，请稍后重试。"
}
function keyState(key: APIKey) {
  return key.admin_disabled ? "管理员已禁用" : key.revoked_at ? "已停用" : "已启用"
}

export function AccountKeys() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [usage, setUsage] = useState<ATHubUsage | null>(null)
  const [days, setDays] = useState(7)
  const [revision, setRevision] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [secretOpen, setSecretOpen] = useState(false)
  const [secret, setSecret] = useState("")
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState("")
  const [busy, setBusy] = useState(false)
  const [action, setAction] = useState<"reset" | "disable" | null>(null)
  const [actionError, setActionError] = useState("")
  const [keysLoading, setKeysLoading] = useState(true)
  // Reuse provisioning across Strict Mode effect replay so the one-time secret
  // is handled by the live effect even if the first effect has been cleaned up.
  const provision = useRef<{ revision: number; promise: Promise<KeyResult> } | null>(null)
  const resetButton = useRef<HTMLButtonElement>(null)
  const [calls, setCalls] = useState<Call[]>([])
  const [offset, setOffset] = useState(0)
  const [callsLoading, setCallsLoading] = useState(true)
  const [callsError, setCallsError] = useState("")
  const [hasNext, setHasNext] = useState(false)

  useEffect(() => {
    let active = true
    setKeysLoading(true); setError("")
    if (!provision.current || provision.current.revision !== revision) {
      provision.current = { revision, promise: api<KeyResult>(keyPath, { method: "POST", body: "{}" }) }
    }
    void provision.current.promise.then(result => {
      if (!active) return
      setKeys([result.key])
      if (result.api_key) { setSecret(result.api_key); setCopied(false); setCopyError(""); setSecretOpen(true) }
    }).catch(reason => { if (active) setError(errorMessage(reason)) })
      .finally(() => { if (active) setKeysLoading(false) })
    return () => { active = false }
  }, [revision])

  useEffect(() => {
    let active = true
    setLoading(true)
    void api<ATHubUsage>(`${usagePath}/usage?days=${days}`).then(result => {
      if (active) setUsage(result)
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

  const keyNames = new Map(keys.map(key => [key.id, key.name]))

  function closeSecret(open: boolean) {
    setSecretOpen(open)
    if (!open) { setSecret(""); setCopied(false); setCopyError("") }
  }
  async function copyKey() {
    try { await navigator.clipboard.writeText(secret); setCopied(true); feedback.success("密钥已复制") }
    catch { setCopyError("复制失败，请手动选中并复制密钥。") }
  }
  async function manageKey() {
    if (!action || !keys[0] || busy) return
    setBusy(true); setActionError("")
    try {
      if (action === "reset") {
        const result = await api<KeyResult>(`${keyPath}/reset`, { method: "POST", body: "{}" })
        setKeys([result.key]); setSecret(result.api_key || ""); setCopied(false); setCopyError(""); setSecretOpen(true)
        feedback.success("密钥已重置")
      } else {
        await api(`${keyPath}/${keys[0].id}`, { method: "DELETE" })
        feedback.success("密钥已停用")
      }
      setAction(null); setRevision(value => value + 1)
    } catch (reason) { setActionError(errorMessage(reason)) }
    finally { setBusy(false) }
  }

  return <section className="space-y-7" aria-labelledby="account-keys-title">
    <div className="flex flex-wrap items-center justify-between gap-4">
      <h1 id="account-keys-title" className="text-2xl font-semibold tracking-tight">API 密钥</h1>
      <div className="flex items-center gap-2">
        <Button variant="outline" size="icon" aria-label="刷新密钥和调用统计" disabled={loading || callsLoading || keysLoading || busy} onClick={() => setRevision(value => value + 1)}><RefreshCw /></Button>
      </div>
    </div>
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button variant="link" onClick={() => setRevision(value => value + 1)}>重试</Button></AlertDescription></Alert>}

    <Card>
      <CardHeader><CardTitle>我的密钥</CardTitle></CardHeader>
      <CardContent>
        {keysLoading && !keys.length ? <Skeleton className="h-10 w-full" /> : !keys.length ? <p className="py-6 text-center text-muted-foreground">暂时无法加载密钥</p> :
          keys.map(key => <div key={key.id} className="space-y-5">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div className="flex flex-wrap items-center gap-3"><KeyRound className="size-5 text-muted-foreground" /><code className="text-sm">{key.prefix}••••••••</code><Badge variant={key.admin_disabled || key.revoked_at ? "outline" : "secondary"}>{keyState(key)}</Badge></div>
              <div className="flex gap-2">
                <Button ref={resetButton} variant="outline" disabled={busy || keysLoading || key.admin_disabled} onClick={() => { setAction("reset"); setActionError("") }}><RefreshCw />重置</Button>
                <Button variant="outline" disabled={busy || keysLoading || key.admin_disabled || Boolean(key.revoked_at)} onClick={() => { setAction("disable"); setActionError("") }}>停用</Button>
              </div>
            </div>
            {key.admin_disabled && <p className="text-sm text-muted-foreground">此密钥已被管理员禁用，需由管理员解除限制后才能重置。</p>}
          </div>)}
      </CardContent>
    </Card>

    <ATHubUsagePanel usage={usage} days={days} loading={loading} onDaysChange={setDays} />

    <Card><CardHeader><CardTitle>最近调用</CardTitle></CardHeader><CardContent>
      {callsError ? <Alert variant="destructive"><AlertCircle /><AlertDescription>{callsError}<Button variant="link" onClick={() => setRevision(value => value + 1)}>重试</Button></AlertDescription></Alert> : callsLoading ? <Skeleton className="h-40 w-full" /> : !calls.length ? <p className="py-6 text-center text-muted-foreground">暂无调用记录</p> :
        <div className="overflow-x-auto"><table className="w-full text-left text-sm">
          <thead className="text-xs text-muted-foreground"><tr className="border-b">{["时间", "密钥", "接口", "状态", "耗时"].map(label => <th key={label} scope="col" className="whitespace-nowrap px-3 pb-3 font-normal first:pl-0 last:pr-0">{label}</th>)}</tr></thead>
          <tbody>{calls.map(call => <tr key={call.id} className="border-b last:border-0">
            <td className="whitespace-nowrap py-3 pr-3 text-xs text-muted-foreground">{date(call.started_at)}</td>
            <td className="max-w-36 truncate px-3" title={call.key_id ? keyNames.get(call.key_id) : undefined}>{call.key_id ? keyNames.get(call.key_id) || "历史密钥" : "—"}</td>
            <td className="whitespace-nowrap px-3 font-mono text-xs">{call.method} {call.path}</td>
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

    <Dialog open={secretOpen} onOpenChange={closeSecret}>
      <DialogContent className="sm:max-w-md" finalFocus={resetButton}>
        <DialogHeader><DialogTitle>密钥已生成</DialogTitle>
          <DialogDescription>完整密钥仅显示这一次，请复制并妥善保存。丢失后可重置密钥。</DialogDescription>
        </DialogHeader>
        {copyError && <Alert variant="destructive"><AlertCircle /><AlertDescription>{copyError}</AlertDescription></Alert>}
        <div className="flex items-start gap-2 rounded-lg border bg-muted/40 p-3"><code className="min-w-0 flex-1 select-all break-all text-sm">{secret}</code><Button variant="ghost" size="icon-sm" aria-label={copied ? "已复制密钥" : "复制密钥"} onClick={copyKey}>{copied ? <Check /> : <Copy />}</Button></div>
        <DialogFooter><Button onClick={() => closeSecret(false)}>完成</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <ConfirmDialog open={Boolean(action)} onOpenChange={open => { if (!open) setAction(null) }} title={action === "reset" ? "重置密钥" : "停用密钥"}
      description={action === "reset" ? "重置后旧密钥立即失效，并生成新密钥。历史调用记录会保留。" : "停用后，此密钥的请求将立即失效。刷新页面不会重新启用，可通过重置生成新密钥。"}
      confirmLabel={action === "reset" ? "重置密钥" : "停用密钥"} busy={busy} onConfirm={manageKey}>
      {actionError && <Alert variant="destructive"><AlertCircle /><AlertDescription>{actionError}</AlertDescription></Alert>}
    </ConfirmDialog>
  </section>
}
