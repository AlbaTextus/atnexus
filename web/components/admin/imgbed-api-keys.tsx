"use client"

import { useState, type FormEvent } from "react"
import { AlertCircle, Check, Copy, KeyRound, Pencil, Plus, PowerOff } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import {
  formatDateTime,
  imageKeyPermissions,
  imgbedError,
  permissionShortLabels,
  type CreatedImageApiKey,
  type ImageApiKey,
  type ImageKeyPermission,
} from "@/lib/admin/imgbed"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"

type KeyFormState = {
  mode: "create" | "edit"
  tokenId?: string
  name: string
  permissions: ImageKeyPermission[]
  expiresAtLocal: string
  disabled: boolean
  autoDelete: boolean
}

function toDatetimeLocal(iso: string | null): string {
  if (!iso) return ""
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function ImgbedApiKeys({
  keys,
  loading,
  onChanged,
}: {
  keys: ImageApiKey[] | null
  loading: boolean
  onChanged: () => void
}) {
  const [form, setForm] = useState<KeyFormState | null>(null)
  const [createdToken, setCreatedToken] = useState<CreatedImageApiKey | null>(null)
  const [copied, setCopied] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [dialogError, setDialogError] = useState("")

  function togglePermission(permission: ImageKeyPermission, checked: boolean) {
    setForm(prev => {
      if (!prev) return prev
      const next = checked
        ? prev.permissions.includes(permission) ? prev.permissions : [...prev.permissions, permission]
        : prev.permissions.filter(p => p !== permission)
      return { ...prev, permissions: next }
    })
  }

  async function submitForm(event: FormEvent) {
    event.preventDefault()
    if (!form || busy) return
    if (form.mode === "create" && !form.name.trim()) {
      setDialogError("请填写 Key 名称。")
      return
    }
    if (form.permissions.length === 0) {
      setDialogError("请至少选择一项权限。")
      return
    }
    let expiresAt: string | null = null
    if (form.expiresAtLocal.trim()) {
      const parsed = new Date(form.expiresAtLocal)
      if (Number.isNaN(parsed.getTime()) || parsed.getTime() <= Date.now()) {
        setDialogError("过期时间必须晚于当前时间，或留空表示永不过期。")
        return
      }
      expiresAt = parsed.toISOString()
    }

    setBusy(true)
    setDialogError("")
    setError("")
    try {
      if (form.mode === "create") {
        const result = await api<CreatedImageApiKey>("/api/v1/admin/image/keys", {
          method: "POST",
          body: JSON.stringify({
            name: form.name.trim(),
            permissions: form.permissions,
            expiresAt,
            autoDelete: form.autoDelete,
          }),
        })
        setCreatedToken(result)
        setCopied(false)
        setForm(null)
        feedback.success(`已创建图床 Key「${result.name}」`)
        onChanged()
      } else {
        await api<{ success: boolean }>("/api/v1/admin/image/keys", {
          method: "PUT",
          body: JSON.stringify({
            tokenId: form.tokenId,
            permissions: form.permissions,
            expiresAt,
            disabled: form.disabled,
            autoDelete: form.autoDelete,
          }),
        })
        setForm(null)
        feedback.success(`图床 Key「${form.name}」已更新`)
        onChanged()
      }
    } catch (reason) {
      setDialogError(imgbedError(reason))
    } finally {
      setBusy(false)
    }
  }

  async function disableKey(item: ImageApiKey) {
    if (busy || item.disabled) return
    setBusy(true)
    setError("")
    try {
      await api<{ success: boolean }>(`/api/v1/admin/image/keys?id=${encodeURIComponent(item.id)}`, {
        method: "DELETE",
      })
      feedback.success(`已停用图床 Key「${item.name}」`)
      onChanged()
    } catch (reason) {
      setError(imgbedError(reason))
    } finally {
      setBusy(false)
    }
  }

  async function copyToken(token: string) {
    try {
      await navigator.clipboard.writeText(token)
      setCopied(true)
      feedback.success("已复制明文 Token")
    } catch {
      feedback.error("复制失败，请手动选中复制")
    }
  }

  return (
    <Card aria-busy={loading || busy}>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="size-4 text-muted-foreground" aria-hidden="true" />
            <span>图床外部 API Key</span>
          </CardTitle>
          <CardDescription>用于外部脚本或独立服务通过 Authorization: Bearer 调用图床 /api/v2 接口；创建后仅展示一次明文。</CardDescription>
        </div>
        <Button
          size="sm"
          disabled={loading || busy}
          onClick={() => {
            setDialogError("")
            setForm({
              mode: "create",
              name: "",
              permissions: ["upload", "list"],
              expiresAtLocal: "",
              disabled: false,
              autoDelete: false,
            })
          }}
        >
          <Plus data-icon="inline-start" />
          新建 API Key
        </Button>
      </CardHeader>

      <CardContent className="space-y-4">
        {error && (
          <Alert variant="destructive">
            <AlertCircle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {createdToken && (
          <div className="space-y-2.5 rounded-xl border border-emerald-500/40 bg-emerald-500/5 p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm font-medium">新创建的 Key 凭据：{createdToken.name}</p>
              <Button type="button" variant="ghost" size="sm" onClick={() => setCreatedToken(null)}>
                我已保存
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">此明文 Token 仅在创建时显示一次，请立即复制妥善保存。</p>
            <div className="flex flex-wrap items-center gap-2">
              <code className="min-w-0 flex-1 break-all rounded-lg border border-border bg-background px-3 py-2 font-mono text-xs">
                {createdToken.token}
              </code>
              <Button type="button" variant="outline" size="sm" onClick={() => void copyToken(createdToken.token)}>
                {copied ? <Check data-icon="inline-start" /> : <Copy data-icon="inline-start" />}
                {copied ? "已复制" : "复制"}
              </Button>
            </div>
          </div>
        )}

        {loading && !keys ? (
          <Skeleton className="h-36 w-full" />
        ) : !keys?.length ? (
          <p className="rounded-lg border border-dashed border-border py-8 text-center text-sm text-muted-foreground">
            暂无外部图床 API Key
          </p>
        ) : (
          <div className="space-y-2.5">
            {keys.map(item => {
              const expired = Boolean(item.expiresAt && new Date(item.expiresAt).getTime() <= Date.now())
              return (
                <div key={item.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/80 px-4 py-3">
                  <div className="min-w-0 flex-1 space-y-1.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{item.name}</span>
                      {item.disabled ? (
                        <Badge variant="outline">已停用</Badge>
                      ) : expired ? (
                        <Badge variant="destructive">已过期</Badge>
                      ) : (
                        <Badge variant="success">启用中</Badge>
                      )}
                      {item.permissions.map(p => (
                        <Badge key={p} variant="secondary">
                          {permissionShortLabels[p] ?? p}
                        </Badge>
                      ))}
                      {item.autoDelete && <Badge variant="outline">自动清理</Badge>}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      创建于 {formatDateTime(item.createdAt)} ·{" "}
                      {item.expiresAt ? `过期于 ${formatDateTime(item.expiresAt)}` : "永不过期"}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      onClick={() => {
                        setDialogError("")
                        setForm({
                          mode: "edit",
                          tokenId: item.id,
                          name: item.name,
                          permissions: item.permissions,
                          expiresAtLocal: toDatetimeLocal(item.expiresAt),
                          disabled: item.disabled,
                          autoDelete: item.autoDelete,
                        })
                      }}
                    >
                      <Pencil data-icon="inline-start" />
                      编辑
                    </Button>
                    {!item.disabled && (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy}
                        onClick={() => void disableKey(item)}
                      >
                        <PowerOff data-icon="inline-start" />
                        停用
                      </Button>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>

      <Dialog open={Boolean(form)} onOpenChange={open => { if (!open && !busy) setForm(null) }}>
        <DialogContent className="center-dialog sm:max-w-md" showCloseButton={!busy}>
          <DialogHeader>
            <DialogTitle>{form?.mode === "create" ? "新建图床 API Key" : `编辑 Key · ${form?.name}`}</DialogTitle>
            <DialogDescription>按需分配上传、列表、删除与管理权限；管理权限仅应授予受信任的服务。</DialogDescription>
          </DialogHeader>
          {form && (
            <form onSubmit={event => void submitForm(event)} className="space-y-4">
              {dialogError && (
                <Alert variant="destructive">
                  <AlertCircle />
                  <AlertDescription>{dialogError}</AlertDescription>
                </Alert>
              )}
              <FieldGroup className="gap-4">
                {form.mode === "create" && (
                  <Field>
                    <FieldLabel htmlFor="imgbed-key-name">Key 名称</FieldLabel>
                    <Input
                      id="imgbed-key-name"
                      required
                      maxLength={200}
                      placeholder="例如：图床同步脚本"
                      disabled={busy}
                      value={form.name}
                      onChange={event => setForm(prev => prev ? { ...prev, name: event.target.value } : prev)}
                    />
                  </Field>
                )}

                <div className="space-y-2">
                  <span className="text-sm font-medium">权限范围</span>
                  <div className="space-y-2 rounded-xl border border-border p-3">
                    {imageKeyPermissions.map(item => (
                      <label key={item.value} className="flex cursor-pointer items-start gap-3">
                        <Checkbox
                          checked={form.permissions.includes(item.value)}
                          disabled={busy}
                          onCheckedChange={checked => togglePermission(item.value, Boolean(checked))}
                        />
                        <div className="min-w-0 text-sm">
                          <div className="font-medium leading-none">{item.label}</div>
                          <p className="mt-1 text-xs text-muted-foreground">{item.description}</p>
                        </div>
                      </label>
                    ))}
                  </div>
                </div>

                <Field>
                  <FieldLabel htmlFor="imgbed-key-expires">过期时间（留空表示永不过期）</FieldLabel>
                  <Input
                    id="imgbed-key-expires"
                    type="datetime-local"
                    disabled={busy}
                    value={form.expiresAtLocal}
                    onChange={event => setForm(prev => prev ? { ...prev, expiresAtLocal: event.target.value } : prev)}
                  />
                </Field>

                {form.mode === "edit" && (
                  <Field orientation="horizontal">
                    <FieldContent>
                      <FieldLabel htmlFor="imgbed-key-disabled">停用此 Key</FieldLabel>
                      <FieldDescription>停用后使用该 Key 的外部调用将被拒绝。</FieldDescription>
                    </FieldContent>
                    <Switch
                      id="imgbed-key-disabled"
                      checked={form.disabled}
                      disabled={busy}
                      onCheckedChange={checked => setForm(prev => prev ? { ...prev, disabled: checked } : prev)}
                    />
                  </Field>
                )}
              </FieldGroup>
              <DialogFooter>
                <Button type="button" variant="outline" disabled={busy} onClick={() => setForm(null)}>取消</Button>
                <Button type="submit" disabled={busy}>
                  {busy && <Spinner data-icon="inline-start" />}
                  {form.mode === "create" ? "创建 Key" : "保存更改"}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>
    </Card>
  )
}
