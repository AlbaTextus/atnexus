"use client"

import { useEffect, useState, type FormEvent } from "react"
import { AlertCircle } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { feedback } from "@/lib/feedback"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

type KeyLimit = { role: string; total_call_limit: number | null; updated_at: string }
const levels = [
  { role: "USER", label: "LT1" },
  { role: "LEVEL_2", label: "LT2" },
  { role: "ADMIN", label: "LT3" },
  { role: "SUPER_ADMIN", label: "LT4" },
]

export function AdminKeyLimits() {
  const [limits, setLimits] = useState<KeyLimit[] | null>(null)
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [pending, setPending] = useState<string | null>(null)
  const [error, setError] = useState("")
  const [retry, setRetry] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setError("")
    api<{ limits: KeyLimit[] }>("/api/v1/admin/athub/key-limits", { signal: controller.signal })
      .then(result => {
        if (controller.signal.aborted) return
        setLimits(result.limits)
        setDrafts(Object.fromEntries(result.limits.map(limit => [limit.role, limit.total_call_limit?.toString() ?? ""])))
      })
      .catch(reason => { if (!controller.signal.aborted) setError(managementError(reason)) })
    return () => controller.abort()
  }, [retry])

  async function save(event: FormEvent, role: string, label: string) {
    event.preventDefault()
    if (pending) return
    const draft = drafts[role] ?? ""
    const value = draft === "" ? null : Number(draft)
    if (value !== null && (!/^\d+$/.test(draft) || !Number.isSafeInteger(value) || value < 0)) {
      setError("总调用上限需为非负整数，留空表示不限次。")
      return
    }
    setPending(role)
    setError("")
    try {
      const updated = await api<KeyLimit>("/api/v1/admin/athub/key-limits", {
        method: "PUT",
        body: JSON.stringify({ role, total_call_limit: value }),
      })
      setLimits(current => current?.map(limit => limit.role === role ? updated : limit) ?? null)
      setDrafts(current => ({ ...current, [role]: updated.total_call_limit?.toString() ?? "" }))
      feedback.success(`${label} Key 调用限制已保存`)
    } catch (reason) {
      setError(managementError(reason))
    } finally {
      setPending(null)
    }
  }

  return <Card>
    <CardHeader>
      <CardTitle>Key 总调用限制</CardTitle>
      <CardDescription id="key-limits-description">按每把 Key 的累计调用次数限制，成功、失败和处理中均计入，不按天重置。留空表示不限次，0 表示不允许调用。</CardDescription>
    </CardHeader>
    <CardContent className="space-y-5">
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}{!limits && <Button variant="link" onClick={() => setRetry(value => value + 1)}>重试</Button>}</AlertDescription></Alert>}
      {!limits ? !error && <Skeleton className="h-56 w-full" aria-label="正在加载 Key 调用限制" /> :
        levels.map(({ role, label }) => {
          const setting = limits.find(limit => limit.role === role)
          const changed = setting && (drafts[role] ?? "") !== (setting.total_call_limit?.toString() ?? "")
          return <form key={role} onSubmit={event => void save(event, role, label)} className="flex flex-col gap-3 border-b pb-5 last:border-0 last:pb-0 sm:flex-row sm:items-end">
            <Field className="flex-none sm:w-[calc(50%-5rem)]">
              <FieldLabel htmlFor={`key-limit-${role}`}>{label} Key 总调用上限</FieldLabel>
              <Input id={`key-limit-${role}`} type="text" inputMode="numeric" pattern="[0-9]*" placeholder="不限次"
                aria-describedby="key-limits-description" value={drafts[role] ?? ""} disabled={Boolean(pending) || !setting}
                onChange={event => setDrafts(current => ({ ...current, [role]: event.target.value }))} />
            </Field>
            <Button type="button" variant="outline" disabled={Boolean(pending) || !setting || drafts[role] === ""} onClick={() => setDrafts(current => ({ ...current, [role]: "" }))}>不限次</Button>
            <Button type="submit" disabled={Boolean(pending) || !changed}>{pending === role ? "保存中…" : "保存"}</Button>
          </form>
        })}
    </CardContent>
  </Card>
}
