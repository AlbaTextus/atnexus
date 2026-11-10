"use client"

import { useEffect, useState } from "react"
import { AlertCircle, KeyRound } from "lucide-react"
import { api } from "@/lib/api"
import { userManagementError, type ManagedUser } from "@/lib/users"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"

type KeyAccess = {
  admin_disabled: boolean
  account_active: boolean
  updated_at: string
  key: { prefix: string; revoked_at: string | null; total_calls: number } | null
}

export function ManagedUserKey({ user, onSaved, onLockChange }: {
  user: ManagedUser
  onSaved: (message: string) => void
  onLockChange: (locked: boolean) => void
}) {
  const [access, setAccess] = useState<KeyAccess | null>(null)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  const [reload, setReload] = useState(0)
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const path = `/api/v1/admin/users/${user.id}/athub-key`

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError("")
    void api<KeyAccess>(path, { signal: controller.signal }).then(value => {
      if (!controller.signal.aborted) setAccess(value)
    }).catch(reason => { if (!controller.signal.aborted) setError(userManagementError(reason)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [path, user.updated_at, reload])

  useEffect(() => {
    onLockChange(confirm || busy)
    return () => onLockChange(false)
  }, [confirm, busy, onLockChange])

  async function save() {
    if (!access || busy) return
    setBusy(true); setError("")
    const disabled = !access.admin_disabled
    try {
      const value = await api<KeyAccess>(path, {
        method: "PUT", body: JSON.stringify({ disabled, updated_at: access.updated_at }),
      })
      setAccess(value); setConfirm(false)
      onSaved(disabled ? "API Key 已被管理员禁用，用户无法自行恢复。" : "已解除 API Key 限制，用户可重置密钥。")
    } catch (reason) { setError(userManagementError(reason)) }
    finally { setBusy(false) }
  }

  return <section className="managed-user-panel space-y-4" aria-label="API 密钥管理">
    <h3 className="flex items-center gap-2 font-semibold"><KeyRound className="size-4" />API 密钥</h3>
    {loading ? <Skeleton className="h-16 w-full" /> : access && <>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={access.admin_disabled || !access.account_active ? "outline" : "secondary"}>
            {!access.account_active ? "账户已封禁" : access.admin_disabled ? "管理员已禁用" : access.key?.revoked_at ? "用户已停用" : access.key ? "已启用" : "未生成"}
          </Badge>
          {access.key && <code className="text-sm">{access.key.prefix}••••••••</code>}
        </div>
        <Button variant="outline" disabled={busy || (!access.account_active && access.admin_disabled)} onClick={() => { setError(""); setConfirm(true) }}>
          {access.admin_disabled ? "解除 Key 禁用" : "禁用 API Key"}
        </Button>
      </div>
      {!access.account_active ? <p className="text-sm text-muted-foreground">账户封禁已同步禁用 Key。需先解封账户，再由管理员解除 Key 限制。</p>
        : access.admin_disabled && <p className="text-sm text-muted-foreground">用户无法通过自动生成或重置恢复使用。解除限制后，用户需重置密钥。</p>}
    </>}
    {error && !confirm && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button variant="link" onClick={() => setReload(value => value + 1)}>重新加载</Button></AlertDescription></Alert>}
    <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={access?.admin_disabled ? "解除 Key 禁用" : "禁用 API Key"}
      description={access?.admin_disabled ? "解除后，用户可以重置密钥。已停用的旧密钥不会自动恢复。" : "禁用后，该用户的 Key 将立即失效，用户无法通过自动生成或重置自行恢复。历史调用记录会保留。"}
      confirmLabel={access?.admin_disabled ? "解除禁用" : "确认禁用"} onConfirm={save} busy={busy}
      variant={access?.admin_disabled ? "default" : "destructive"}>
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
    </ConfirmDialog>
  </section>
}
