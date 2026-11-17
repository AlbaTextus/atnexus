"use client"

import { useState, type FormEvent } from "react"
import { AlertCircle, Mail } from "lucide-react"
import { api } from "@/lib/api"
import { authErrorMessage } from "@/lib/auth/auth-errors"
import { feedback, useFeedback } from "@/lib/feedback"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account/account-shell"
import { EmailCodeField, useEmailCode } from "@/components/auth/email-code-field"
import { PasswordInput } from "@/components/shared/password-input"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

export function AccountEmail() {
  const user = useAccountUser()
  const updateUser = useUpdateAccountUser()
  const [email, setEmail] = useState(user.email_is_alias ? "" : user.email)
  const [password, setPassword] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useFeedback("error")
  const verification = useEmailCode(email)
  const busy = submitting || verification.sending
  const unchanged = email.trim().toLowerCase() === user.email
  const verified = !user.email_is_alias && user.email_verified
  const action = user.email_is_alias ? "绑定邮箱" : unchanged ? "验证邮箱" : "更换邮箱"

  async function sendCode() {
    if (busy) return
    const input = document.getElementById("security-email") as HTMLInputElement | null
    if (!input?.reportValidity()) return
    setError("")
    try { await verification.send("/api/v1/account/email/code") }
    catch (reason) { setError(authErrorMessage(reason, "验证码发送失败，请稍后重试。")) }
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy || !verification.challengeID) return
    setError(""); setSubmitting(true)
    try {
      const updated = await api<AccountUser>("/api/v1/account/email", { method: "POST", body: JSON.stringify({ email: email.trim(), code: verification.code, challenge_id: verification.challengeID, current_password: password }) })
      updateUser(updated); setEmail(updated.email); setPassword(""); verification.reset()
      feedback.success(unchanged ? "邮箱已验证" : "邮箱已更新，其他设备需重新登录")
    } catch (reason) { setError(authErrorMessage(reason, "邮箱验证失败，请稍后重试。")) }
    finally { setSubmitting(false) }
  }

  return <form onSubmit={submit} aria-busy={busy}><Card className="border border-border">
    <CardHeader>
      <CardTitle className="flex items-center gap-2"><Mail size={18} className="shrink-0 text-blue-600 dark:text-blue-400" aria-hidden="true" />电子邮箱<Badge variant={verified ? "secondary" : "outline"}>{verified ? "已验证" : user.email_is_alias ? "未绑定" : "待验证"}</Badge></CardTitle>
      <CardDescription>{user.email_is_alias ? "绑定可收件邮箱后，可使用它作为登录邮箱，第三方登录仍可继续使用。" : "验证邮箱所有权，或更换用于登录的邮箱。"}</CardDescription>
    </CardHeader>
    <CardContent><FieldGroup>
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
      {user.email_is_alias && <Field><FieldLabel htmlFor="email-login-alias">当前登录标识</FieldLabel><Input id="email-login-alias" readOnly value={user.email} /><FieldDescription>这是第三方账户的专属标识，不能收件。</FieldDescription></Field>}
      <Field data-disabled={busy}>
        <FieldLabel htmlFor="security-email">{user.email_is_alias ? "绑定邮箱" : "电子邮箱"}</FieldLabel>
        <Input id="security-email" type="email" autoComplete="email" autoCapitalize="none" spellCheck={false} required maxLength={320} value={email} onChange={event => { setEmail(event.target.value); verification.reset(); setError("") }} disabled={busy} className="h-11" aria-describedby="security-email-help" />
        <FieldDescription id="security-email-help">{verified && unchanged ? "当前邮箱已验证，填写新邮箱即可更换。" : "验证码将发往此邮箱；完成更换后，其他设备和应用需要重新授权登录。"}</FieldDescription>
      </Field>
      {(!verified || !unchanged) && <>
        <EmailCodeField id="account-email-code" verification={verification} disabled={busy} sendDisabled={!email} onSend={sendCode} />
        {user.has_password && <Field data-disabled={busy}><FieldLabel htmlFor="email-current-password">当前密码</FieldLabel><PasswordInput id="email-current-password" label="当前密码" autoComplete="current-password" required value={password} onChange={event => setPassword(event.target.value)} disabled={busy} /></Field>}
      </>}
    </FieldGroup></CardContent>
    {(!verified || !unchanged) && <CardFooter><Button type="submit" disabled={busy || !verification.challengeID || verification.code.length !== 6 || (user.has_password && !password)}>{submitting && <Spinner data-icon="inline-start" />}{action}</Button></CardFooter>}
  </Card></form>
}
