"use client"

import { useState, type FormEvent } from "react"
import { AlertCircle, Check, KeyRound, LogOut, UserPlus } from "lucide-react"
import { api } from "@/lib/api"
import { roleNames } from "@/lib/applications"
import { type ManagedUser, userManagementError } from "@/lib/users"
import { useAccountUser, useUpdateAccountUser } from "@/components/account-shell"
import { PasswordInput } from "@/components/password-input"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

export type UserAction = { kind: "create" } | { kind: "edit" | "password" | "revoke"; user: ManagedUser }

export function UserManagementDialog({ action, onClose, onSaved }: {
  action: UserAction
  onClose: () => void
  onSaved: (message: string) => void
}) {
  const actor = useAccountUser()
  const updateActor = useUpdateAccountUser()
  const target = action.kind === "create" ? null : action.user
  const self = target?.id === actor.id
  const [name, setName] = useState(target?.display_name || "")
  const [email, setEmail] = useState("")
  const [role, setRole] = useState(target?.role || "USER")
  const [active, setActive] = useState(target?.is_active ?? true)
  const [password, setPassword] = useState("")
  const [confirmation, setConfirmation] = useState("")
  const [error, setError] = useState("")
  const [passwordError, setPasswordError] = useState("")
  const [busy, setBusy] = useState(false)
  const roles = actor.role === "SUPER_ADMIN" ? ["USER", "ADMIN", "SUPER_ADMIN"] : ["USER"]
  const needsPassword = action.kind === "create" || action.kind === "password"
  const changesAccess = target && (target.role !== role || target.is_active !== active)
  const titles = { create: "新建用户", edit: "编辑用户", password: "重置密码", revoke: "撤销登录" }
  const labels = { create: "创建用户", edit: "保存更改", password: "确认重置密码", revoke: "确认撤销登录" }
  const descriptions = {
    create: "创建可使用邮箱与密码登录的账户，请将初始密码通过安全方式告知本人。",
    edit: "管理用户昵称、角色与账户状态。邮箱和第三方登录标识保持原有绑定。",
    password: "设置新密码并撤销此用户的中心会话和 OAuth 授权。请将新密码通过安全方式告知本人。",
    revoke: "撤销此用户的中心会话、OAuth 令牌与待处理授权。用户需要重新登录；密码及通行密钥保留。",
  }
  const ActionIcon = action.kind === "create" ? UserPlus : action.kind === "password" ? KeyRound : action.kind === "revoke" ? LogOut : Check

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy) return
    setError(""); setPasswordError("")
    if (needsPassword) {
      const bytes = new TextEncoder().encode(password).length
      if (bytes < 12 || bytes > 72 || password !== confirmation) {
        setPasswordError(password !== confirmation ? "两次输入的密码不一致。" : "密码需为 12–72 字节；中文等字符会占用多个字节。")
        return
      }
    }
    setBusy(true)
    try {
      if (action.kind === "create") {
        await api<ManagedUser>("/api/v1/admin/users", { method: "POST", body: JSON.stringify({ email, password, display_name: name, role }) })
      } else if (action.kind === "edit") {
        const updated = await api<ManagedUser>(`/api/v1/admin/users/${action.user.id}`, { method: "PATCH", body: JSON.stringify({ display_name: name, role, is_active: active, updated_at: action.user.updated_at }) })
        if (self) updateActor(updated)
      } else {
        await api(`/api/v1/admin/users/${action.user.id}/${action.kind === "password" ? "password" : "revoke-sessions"}`, { method: "POST", body: JSON.stringify({ updated_at: action.user.updated_at, ...(action.kind === "password" ? { new_password: password } : {}) }) })
      }
      onSaved(action.kind === "create" ? "用户已创建。" : action.kind === "edit" ? (changesAccess ? "用户已更新，原有登录与 OAuth 授权已撤销。" : "用户资料已保存。") : action.kind === "password" ? "密码已重置，原有登录与 OAuth 授权已撤销。" : "用户的中心登录与 OAuth 授权已撤销。")
    } catch (reason) { setError(userManagementError(reason)); setBusy(false) }
  }

  return <Dialog open onOpenChange={open => { if (!open && !busy) onClose() }}>
    <DialogContent className="center-dialog max-h-[90dvh] overflow-y-auto sm:max-w-lg" showCloseButton={!busy}>
      <DialogHeader><DialogTitle>{titles[action.kind]}</DialogTitle><DialogDescription>{descriptions[action.kind]}</DialogDescription></DialogHeader>
      {target && <p className="break-all text-sm text-muted-foreground">{target.display_name || "未设置昵称"} · {target.email}</p>}
      <form onSubmit={submit} className="flex flex-col gap-5">
        {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
        <FieldGroup>
          {action.kind === "create" && <Field data-disabled={busy}><FieldLabel htmlFor="managed-email">邮箱</FieldLabel><Input id="managed-email" type="email" autoComplete="off" required maxLength={320} disabled={busy} value={email} onChange={event => setEmail(event.target.value)} placeholder="name@example.com" /></Field>}
          {(action.kind === "create" || action.kind === "edit") && <>
            <Field data-disabled={busy}><FieldLabel htmlFor="managed-name">昵称</FieldLabel><Input id="managed-name" maxLength={100} disabled={busy} value={name} onChange={event => setName(event.target.value)} placeholder="可选，最多 100 个字符" /></Field>
            <Field data-disabled={busy || self}><FieldLabel id="managed-role-label">用户角色</FieldLabel>{roles.length > 1 ? <ToggleGroup aria-labelledby="managed-role-label" variant="outline" spacing={0} value={[role]} disabled={busy || self} onValueChange={values => { if (values[0]) setRole(values[0]) }}><ToggleGroupItem value="USER">普通用户</ToggleGroupItem><ToggleGroupItem value="ADMIN">管理员</ToggleGroupItem><ToggleGroupItem value="SUPER_ADMIN">超级管理员</ToggleGroupItem></ToggleGroup> : <p className="text-sm">{roleNames[role]}</p>}<FieldDescription>{self ? "不能修改自己的角色或停用自己的账户。" : actor.role === "SUPER_ADMIN" ? "管理员可管理普通用户；超级管理员可管理所有角色。" : "管理员只能创建和管理普通用户，角色调整由超级管理员完成。"}</FieldDescription></Field>
            {action.kind === "edit" && <Field data-disabled={busy || self}><FieldLabel id="managed-status-label">账户状态</FieldLabel><ToggleGroup aria-labelledby="managed-status-label" variant="outline" spacing={0} value={[active ? "active" : "inactive"]} disabled={busy || self} onValueChange={values => { if (values[0]) setActive(values[0] === "active") }}><ToggleGroupItem value="active">启用</ToggleGroupItem><ToggleGroupItem value="inactive">停用</ToggleGroupItem></ToggleGroup><FieldDescription>停用后，邮箱、第三方账号与通行密钥均无法登录。</FieldDescription></Field>}
          </>}
          {needsPassword && <>
            <Field data-disabled={busy} data-invalid={Boolean(passwordError)}><FieldLabel htmlFor="managed-password">{action.kind === "create" ? "初始密码" : "新密码"}</FieldLabel><PasswordInput id="managed-password" label="新密码" required autoComplete="new-password" disabled={busy} aria-invalid={Boolean(passwordError)} aria-describedby="managed-password-help" value={password} onChange={event => { setPassword(event.target.value); setPasswordError("") }} /><FieldDescription id="managed-password-help">12–72 字节，建议使用随机生成的长密码。</FieldDescription></Field>
            <Field data-disabled={busy} data-invalid={Boolean(passwordError)}><FieldLabel htmlFor="managed-confirmation">确认密码</FieldLabel><PasswordInput id="managed-confirmation" label="确认密码" required autoComplete="new-password" disabled={busy} aria-invalid={Boolean(passwordError)} aria-describedby={passwordError ? "managed-password-error" : undefined} value={confirmation} onChange={event => { setConfirmation(event.target.value); setPasswordError("") }} />{passwordError && <FieldDescription id="managed-password-error" role="alert">{passwordError}</FieldDescription>}</Field>
          </>}
        </FieldGroup>
        {(changesAccess || action.kind === "password" || action.kind === "revoke") && <Alert><AlertCircle /><AlertDescription>{changesAccess ? "保存角色或状态变更将撤销该用户已有的中心登录与 OAuth 授权。" : "操作完成后，该用户需要重新登录。"}子系统已建立的本地会话由子系统处理。</AlertDescription></Alert>}
        <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={onClose}>取消</Button><Button type="submit" variant={action.kind === "password" || action.kind === "revoke" || !active ? "destructive" : "default"} disabled={busy}>{busy ? <Spinner data-icon="inline-start" /> : <ActionIcon data-icon="inline-start" />}{busy ? "正在保存…" : labels[action.kind]}</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
}
