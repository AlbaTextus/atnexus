"use client"

import { useEffect, useState, type FormEvent } from "react"
import { AlertCircle, Plus, ShieldBan, X } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import { imgbedError } from "@/lib/admin/imgbed"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"

export function ImgbedIPBlacklist({
  ips,
  loading,
  onSaved,
}: {
  ips: string[] | null
  loading: boolean
  onSaved: (next: string[]) => void
}) {
  const [draft, setDraft] = useState<string[]>([])
  const [input, setInput] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    if (ips) setDraft(ips)
  }, [ips])

  const dirty = ips !== null && JSON.stringify(draft) !== JSON.stringify(ips)

  function addEntries(event: FormEvent) {
    event.preventDefault()
    if (!input.trim()) return
    const items = input
      .split(/[\s,，]+/)
      .map(s => s.trim())
      .filter(Boolean)
    if (items.length === 0) return
    const next = [...draft]
    for (const item of items) {
      if (!next.includes(item)) next.push(item)
    }
    if (next.length > 10000) {
      setError("IP 黑名单最多支持 10,000 条记录。")
      return
    }
    setError("")
    setDraft(next)
    setInput("")
  }

  function removeEntry(target: string) {
    setDraft(current => current.filter(item => item !== target))
  }

  async function save() {
    if (busy || !dirty) return
    setBusy(true)
    setError("")
    try {
      await api<{ success: boolean }>("/api/v2/manage/cusConfig/blockipList", {
        method: "PUT",
        body: JSON.stringify({ ips: draft }),
      })
      onSaved(draft)
      feedback.success("上传 IP 黑名单已保存")
    } catch (reason) {
      setError(imgbedError(reason))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card aria-busy={loading || busy}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldBan className="size-4 text-muted-foreground" aria-hidden="true" />
          <span>上传 IP 黑名单</span>
        </CardTitle>
        <CardDescription>命中黑名单的客户端 IP 调用 HTTP 上传接口时将被拒绝（403）；支持逗号或空格批量添加。</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <Alert variant="destructive">
            <AlertCircle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {loading && !ips ? (
          <Skeleton className="h-32 w-full" />
        ) : (
          <>
            <form onSubmit={addEntries} className="flex flex-wrap gap-2">
              <Input
                aria-label="添加黑名单 IP"
                className="min-w-0 flex-1"
                placeholder="输入 IPv4 或 IPv6 地址，多个可用逗号分隔"
                disabled={busy}
                value={input}
                onChange={event => setInput(event.target.value)}
              />
              <Button type="submit" variant="outline" disabled={busy || !input.trim()}>
                <Plus data-icon="inline-start" />
                添加
              </Button>
            </form>

            {draft.length === 0 ? (
              <p className="rounded-lg border border-dashed border-border py-6 text-center text-sm text-muted-foreground">
                当前未拦截任何上传 IP
              </p>
            ) : (
              <div className="flex flex-wrap gap-2 rounded-xl border border-border p-3">
                {draft.map(ip => (
                  <Badge key={ip} variant="secondary" className="gap-1.5 py-1 pr-1.5 pl-2.5 font-mono text-xs">
                    <span>{ip}</span>
                    <button
                      type="button"
                      disabled={busy}
                      aria-label={`移除 ${ip}`}
                      className="rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-background hover:text-foreground"
                      onClick={() => removeEntry(ip)}
                    >
                      <X className="size-3" />
                    </button>
                  </Badge>
                ))}
              </div>
            )}

            <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
              <span className="text-xs text-muted-foreground tabular-nums">共 {draft.length} 条记录</span>
              <div className="flex gap-2">
                {dirty && (
                  <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => setDraft(ips || [])}>
                    撤销更改
                  </Button>
                )}
                {draft.length > 0 && (
                  <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => setDraft([])}>
                    清空列表
                  </Button>
                )}
                <Button size="sm" disabled={busy || !dirty} onClick={() => void save()}>
                  {busy && <Spinner data-icon="inline-start" />}
                  保存黑名单
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
