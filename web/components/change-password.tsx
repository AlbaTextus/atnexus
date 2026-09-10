"use client"

import { useFeedback, feedback } from "@/lib/feedback"

import { useState, type FormEvent } from "react"
import { AlertCircle, Check, KeyRound } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { PasswordInput } from "@/components/password-input"
import { useAccountUser } from "@/components/account-shell"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"

export function ChangePassword() {
  const user = useAccountUser()
  const hasPassword = user.has_password !== false
  const [current, setCurrent] = useState("")
  const [password, setPassword] = useState("")
  const [confirmation, setConfirmation] = useState("")
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useFeedback("error")
  const [invalid, setInvalid] = useState("")
  function resetError() { setError(""); setInvalid("") }
  async function submit(event: FormEvent) {
    event.preventDefault(); if (busy) return
    resetError()
    if (password !== confirmation) { setError("两次输入的新密码不一致。"); setInvalid("confirmation"); return }
    if (password === current) { setError("新密码不能与当前密码相同。"); setInvalid("password"); return }
    const length = new TextEncoder().encode(password).length
    if (length < 12 || length > 72) { setError("新密码需为 12–72 字节；中文等字符会占用多个字节。"); setInvalid("password"); return }
    setBusy(true)
    try { await api("/api/v1/account/password", { method: "POST", body: JSON.stringify({ current_password: current, new_password: password }) }); setCurrent(""); setPassword(""); setConfirmation(""); setDone(true); feedback.success("密码已更新，请重新登录") }
    catch (reason) { setError(managementError(reason)); if (reason instanceof ApiError && reason.code === "incorrect_password") setInvalid("current") } finally { setBusy(false) }
  }
  return <form onSubmit={submit} aria-busy={busy}><Card className="border border-border">
    <CardHeader><CardTitle className="flex items-center gap-2"><KeyRound size={18} />登录密码</CardTitle><CardDescription>{hasPassword ? "修改密码需要验证当前密码，完成后需重新登录。" : user.email_is_alias ? "设置密码后，也可以使用个人资料中的登录标识登录。" : "你目前使用第三方账号登录。设置密码后，也可使用邮箱登录。"}</CardDescription></CardHeader>
    <CardContent>{done ? <Alert><Check /><AlertDescription role="status">密码已更新，原有登录会话和应用授权已失效。请使用新密码重新登录。</AlertDescription></Alert> : <FieldGroup>
      {error && <Alert variant="destructive" id="password-change-error"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
      {hasPassword && <Field data-disabled={busy} data-invalid={invalid === "current"}><FieldLabel htmlFor="current-password">当前密码</FieldLabel><PasswordInput id="current-password" label="当前密码" autoComplete="current-password" required value={current} onChange={event => { setCurrent(event.target.value); resetError() }} disabled={busy} aria-invalid={invalid === "current"} aria-describedby={invalid === "current" ? "password-change-error" : undefined} /></Field>}
      <Field data-disabled={busy} data-invalid={invalid === "password"}><FieldLabel htmlFor="new-password">新密码</FieldLabel><PasswordInput id="new-password" label="新密码" autoComplete="new-password" required maxLength={72} value={password} onChange={event => { setPassword(event.target.value); resetError() }} disabled={busy} aria-invalid={invalid === "password"} aria-describedby={invalid === "password" ? "password-change-error" : "new-password-help"} /><FieldDescription id="new-password-help">建议使用至少 12 位的英文、数字与符号组合。</FieldDescription></Field>
      <Field data-disabled={busy} data-invalid={invalid === "confirmation"}><FieldLabel htmlFor="confirm-password">确认新密码</FieldLabel><PasswordInput id="confirm-password" label="确认新密码" autoComplete="new-password" required maxLength={72} value={confirmation} onChange={event => { setConfirmation(event.target.value); resetError() }} disabled={busy} aria-invalid={invalid === "confirmation"} aria-describedby={invalid === "confirmation" ? "password-change-error" : undefined} /></Field>
    </FieldGroup>}</CardContent>
    <CardFooter>{done ? <Button type="button" nativeButton={false} role="link" render={<a href="/login?return_to=%2Faccount%2Fsecurity" />}>重新登录</Button> : <Button type="submit" disabled={busy || (hasPassword && !current) || !password || !confirmation}>{busy && <Spinner data-icon="inline-start" aria-hidden="true" />}{hasPassword ? "更新密码" : "设置密码"}</Button>}</CardFooter>
  </Card></form>
}
