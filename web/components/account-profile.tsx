"use client"

import { useState, type FormEvent } from "react"
import { AlertCircle, Check, Save } from "lucide-react"
import { api } from "@/lib/api"
import { managementError, roleNames } from "@/lib/applications"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { AvatarSettings } from "@/components/avatar-settings"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

export function AccountProfile() {
  const user = useAccountUser()
  const updateUser = useUpdateAccountUser()
  const [name, setName] = useState(user.display_name || "")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [saved, setSaved] = useState(false)
  const dirty = name.trim() !== (user.display_name || "")
  async function submit(event: FormEvent) {
    event.preventDefault(); if (busy) return
    setBusy(true); setError(""); setSaved(false)
    try { const value = await api<AccountUser>("/api/v1/account/profile", { method: "PATCH", body: JSON.stringify({ display_name: name.trim() }) }); updateUser(value); setName(value.display_name || ""); setSaved(true) }
    catch (reason) { setError(managementError(reason)) } finally { setBusy(false) }
  }
  return <div className="flex max-w-3xl flex-col gap-6">
    <div className="center-page-heading"><div><p className="center-eyebrow">PERSONAL PROFILE</p><h1>个人资料</h1><p>完善你的个人信息，让每个应用更好地认识你。</p></div></div>
    <AvatarSettings />
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
    {saved && <Alert><Check /><AlertDescription role="status">个人资料已保存。</AlertDescription></Alert>}
    <form onSubmit={submit} aria-busy={busy}><Card><CardHeader><CardTitle>基本信息</CardTitle><CardDescription>昵称会同步到使用统一账户的应用。</CardDescription></CardHeader><CardContent><FieldGroup>
      <Field data-disabled={busy}><FieldLabel htmlFor="profile-name">昵称</FieldLabel><Input id="profile-name" value={name} onChange={event => { setName(event.target.value); setSaved(false); setError("") }} required maxLength={100} disabled={busy} autoComplete="nickname" placeholder="希望大家怎么称呼你" className="h-11" /><FieldDescription>最多 100 个字符。</FieldDescription></Field>
      <Field><FieldLabel htmlFor="profile-email">{user.email_is_alias ? "登录标识" : "电子邮箱"}</FieldLabel><Input id="profile-email" value={user.email} readOnly className="h-11" /><FieldDescription>{user.email_is_alias ? "这是第三方账户的专属登录标识，不是收件邮箱。设置密码后可用它登录，也可继续使用 LINUX DO。" : "用于登录与账户识别。如需更改，请联系管理员。"}</FieldDescription></Field>
      <Field><FieldLabel>账户身份</FieldLabel><div><Badge variant="secondary">{roleNames[user.role] || "用户"}</Badge></div></Field>
    </FieldGroup></CardContent><CardFooter className="gap-3"><Button type="submit" disabled={busy || !dirty || !name.trim()}>{busy ? <Spinner data-icon="inline-start" aria-hidden="true" /> : <Save data-icon="inline-start" />}保存资料</Button><Button type="button" variant="outline" disabled={busy || !dirty} onClick={() => { setName(user.display_name || ""); setError(""); setSaved(false) }}>取消修改</Button></CardFooter></Card></form>
  </div>
}
