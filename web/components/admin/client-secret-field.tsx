"use client"

import { useFeedback, feedback } from "@/lib/feedback"

import { useEffect, useId, useRef, useState } from "react"
import { Check, Copy, Eye, EyeOff, RotateCw } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/admin/applications"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"

export function ClientSecretField({ applicationId, available, busy, onRotate }: {
  applicationId: string
  available: boolean
  busy: boolean
  onRotate: () => void
}) {
  const id = useId()
  const [secret, setSecret] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useFeedback("error")
  const request = useRef<AbortController | null>(null)
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    request.current?.abort()
    if (copyTimer.current) clearTimeout(copyTimer.current)
  }, [])

  async function readSecret(copy: boolean) {
    if (loading || busy || !available) return
    if (!copy && secret !== null) { setSecret(null); setError(""); return }
    const controller = new AbortController()
    request.current = controller
    setLoading(true); setError("")
    try {
      const result = await api<{ client_secret: string }>(`/api/v1/applications/${encodeURIComponent(applicationId)}/secret`, { cache: "no-store", signal: controller.signal })
      if (controller.signal.aborted) return
      if (copy) {
        try {
          await navigator.clipboard.writeText(result.client_secret)
          if (controller.signal.aborted) return
          setCopied(true); feedback.success("密钥已复制")
          if (copyTimer.current) clearTimeout(copyTimer.current)
          copyTimer.current = setTimeout(() => setCopied(false), 2000)
        } catch { setError("复制失败，请显示密钥后选中内容手动复制。") }
      } else setSecret(result.client_secret)
    } catch (reason) {
      if (!controller.signal.aborted) setError(managementError(reason))
    } finally {
      if (!controller.signal.aborted) setLoading(false)
    }
  }

  return <Field>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <FieldLabel htmlFor={id}>Client Secret</FieldLabel>
      <Button type="button" variant="outline" size="sm" disabled={busy || loading} onClick={onRotate}><RotateCw data-icon="inline-start" />重置密钥</Button>
    </div>
    <InputGroup className="h-11" aria-busy={loading}>
      <InputGroupInput id={id} aria-describedby={`${id}-help`} type={secret === null ? "password" : "text"} value={secret ?? ""} placeholder="••••••••••••••••••••••••" readOnly autoComplete="off" spellCheck={false} onFocus={event => event.currentTarget.select()} />
      <InputGroupAddon align="inline-end">
        <InputGroupButton size="icon-sm" aria-label={secret === null ? "显示密钥" : "隐藏密钥"} aria-controls={id} aria-pressed={secret !== null} disabled={busy || loading || !available} onClick={() => void readSecret(false)}>{loading ? <Spinner aria-hidden="true" /> : secret === null ? <Eye /> : <EyeOff />}</InputGroupButton>
        <InputGroupButton size="icon-sm" aria-label="复制 Client Secret" disabled={busy || loading || !available} onClick={() => void readSecret(true)}>{copied ? <Check /> : <Copy />}</InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
    <FieldDescription id={`${id}-help`}>{available ? "点击眼睛查看密钥，也可以直接复制。" : "旧密钥没有可查看的副本。重置后可随时查看，请同步更新子系统配置。"}</FieldDescription>
    <span className="sr-only" role="status">{loading ? "正在读取密钥" : copied ? "Client Secret 已复制" : ""}</span>
    {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
  </Field>
}
