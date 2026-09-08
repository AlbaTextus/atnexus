"use client"

import { useFeedback, feedback } from "@/lib/feedback"

import { useState, type FormEvent } from "react"
import { AlertCircle, Check, Save } from "lucide-react"
import { api } from "@/lib/api"
import { isAdminRole, managementError, roleNames } from "@/lib/applications"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { AvatarSettings } from "@/components/avatar-settings"
import { BioEditor } from "@/components/bio-editor"
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
  const [error, setError] = useFeedback("error")
  const [saved, setSaved] = useState(false)
  const dirty = name.trim() !== (user.display_name || "")
  const canEditName = isAdminRole(user.role)
  async function submit(event: FormEvent) {
    event.preventDefault(); if (busy || !canEditName) return
    setBusy(true); setError(""); setSaved(false)
    try { const value = await api<AccountUser>("/api/v1/account/profile", { method: "PATCH", body: JSON.stringify({ display_name: name.trim() }) }); updateUser(value); setName(value.display_name || ""); setSaved(true); feedback.success("个人资料已保存") }
    catch (reason) { setError(managementError(reason)) } finally { setBusy(false) }
  }
  return <div className="profile-editor-page mx-auto flex w-full max-w-3xl flex-col gap-6">
    <div className="center-page-heading"><div><h1>编辑个人资料</h1><p>更新你的头像、个人简介和基本信息。</p></div></div>
    <AvatarSettings />
    <BioEditor />
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
    {saved && <Alert><Check /><AlertDescription role="status">个人资料已保存。</AlertDescription></Alert>}
    <form onSubmit={submit} aria-busy={busy}><Card><CardHeader><CardTitle>基本信息</CardTitle><CardDescription>昵称会同步到使用统一账户的应用。</CardDescription></CardHeader><CardContent><FieldGroup>
      <Field data-disabled={busy}><FieldLabel htmlFor="profile-name">昵称</FieldLabel><Input id="profile-name" value={name} onChange={event => { setName(event.target.value); setSaved(false); setError("") }} required maxLength={100} disabled={busy} readOnly={!canEditName} aria-describedby="profile-name-help" autoComplete="nickname" placeholder="希望大家怎么称呼你" className="h-11" /><FieldDescription id="profile-name-help">{canEditName ? "最多 100 个字符。" : "昵称仅限管理员及以上修改，如需更改请联系管理员。"}</FieldDescription></Field>
      <Field><FieldLabel>账户身份</FieldLabel><div><Badge variant="secondary">{roleNames[user.role] || "用户"}</Badge></div></Field>
    </FieldGroup></CardContent>{canEditName && <CardFooter className="gap-3"><Button type="submit" disabled={busy || !dirty || !name.trim()}>{busy ? <Spinner data-icon="inline-start" aria-hidden="true" /> : <Save data-icon="inline-start" />}保存资料</Button><Button type="button" variant="outline" disabled={busy || !dirty} onClick={() => { setName(user.display_name || ""); setError(""); setSaved(false) }}>取消修改</Button></CardFooter>}</Card></form>
  </div>
}
