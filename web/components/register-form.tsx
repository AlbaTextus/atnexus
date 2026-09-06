"use client"

import { useState, type FormEvent } from "react"
import { AlertCircle } from "lucide-react"
import { PasswordInput } from "@/components/password-input"
import { AuthCard } from "@/components/auth-card"
import { AuthFooter } from "@/components/auth-footer"
import { SocialLogin } from "@/components/social-login"
import { AuthVerification, useAuthVerification } from "@/components/auth-verification"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldGroup, FieldLabel, FieldSeparator } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { api, safeReturnTo } from "@/lib/api"
import { authErrorMessage } from "@/lib/auth-errors"

type RegisterFormProps = { returnTo: string | null }

export function RegisterForm({ returnTo }: RegisterFormProps) {
  const verification = useAuthVerification()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [confirmError, setConfirmError] = useState(false)
  const destination = safeReturnTo(returnTo)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy || !verification.ready) return
    setError("")
    setConfirmError(false)
    if (password !== confirm) {
      setConfirmError(true)
      document.getElementById("confirm")?.focus()
      return
    }
    // Match the Go service's UTF-8 byte limits for non-ASCII input.
    const encoder = new TextEncoder()
    const passwordBytes = encoder.encode(password).length
    if (passwordBytes < 12 || passwordBytes > 72) {
      setError("密码过长，请减少中文、表情或字符数量后重试。")
      return
    }
    if (encoder.encode(name.trim()).length > 100) {
      setError("显示名称太长了，请缩短后重试。")
      return
    }
    setBusy(true)
    try {
      await api("/api/v1/users", { method: "POST", body: JSON.stringify({ email: email.trim(), password, display_name: name.trim() }), headers: verification.headers })
      window.location.href = destination
    } catch (reason) {
      setError(authErrorMessage(reason, "暂时无法创建账户，请稍后重试。"))
      verification.reset()
      setBusy(false)
    }
  }

  return (
    <AuthCard
      title="创建你的账户" titleId="register-title" description="注册邮箱，开启你的 AlbaTextus 体验"
      footer={<AuthFooter returnTo={returnTo} from="register" />}
    >
      {error && <Alert id="register-error" variant="destructive" className="mb-5"><AlertCircle aria-hidden="true" /><AlertDescription>{error}</AlertDescription></Alert>}

      <form onSubmit={submit} onChange={() => setError("")} aria-busy={busy} aria-describedby={error ? "register-error" : undefined}>
        <FieldGroup className="gap-5">
          <Field data-disabled={busy}>
            <FieldLabel htmlFor="name" className="sr-only">显示名称（选填）</FieldLabel>
            <Input id="name" name="name" type="text" autoComplete="name" maxLength={100} value={name} onChange={(event) => setName(event.target.value)} disabled={busy} className="h-11 px-3" placeholder="显示名称（选填）" />
          </Field>
          <Field data-disabled={busy}>
            <FieldLabel htmlFor="email" className="sr-only">电子邮箱</FieldLabel>
            <Input id="email" name="email" type="email" autoComplete="username" autoCapitalize="none" spellCheck={false} required maxLength={320} value={email} onChange={(event) => setEmail(event.target.value)} disabled={busy} className="h-11 px-3" placeholder="邮箱" />
          </Field>
          <Field data-disabled={busy}>
            <FieldLabel htmlFor="password" className="sr-only">密码</FieldLabel>
            <PasswordInput id="password" name="password" autoComplete="new-password" required minLength={12} maxLength={72} value={password} onChange={(event) => { setPassword(event.target.value); setConfirmError(false) }} disabled={busy} placeholder="密码（至少 12 个字符）" />
          </Field>
          <Field data-disabled={busy} data-invalid={confirmError}>
            <FieldLabel htmlFor="confirm" className="sr-only">确认密码</FieldLabel>
            <PasswordInput id="confirm" name="confirm" label="确认密码" autoComplete="new-password" required minLength={12} maxLength={72} value={confirm} onChange={(event) => { setConfirm(event.target.value); setConfirmError(false) }} disabled={busy} placeholder="再次输入你的密码" aria-invalid={confirmError || undefined} aria-describedby={confirmError ? "confirm-error" : undefined} />
            {confirmError && <FieldError id="confirm-error">两次输入的密码不一致，请重新确认。</FieldError>}
          </Field>
          <AuthVerification verification={verification} disabled={busy} />
          <Button type="submit" size="lg" disabled={busy || !verification.ready} className="mt-1 h-11 w-full">
            {busy && <Spinner data-icon="inline-start" aria-hidden="true" />}
            {busy ? "正在创建…" : "创建账户"}
          </Button>
        </FieldGroup>
      </form>
      <FieldSeparator className="my-6">或使用第三方账号</FieldSeparator>
      <SocialLogin returnTo={destination} disabled={busy || !verification.ready} onBusyChange={setBusy} verificationHeaders={verification.headers} onAttemptFailed={verification.reset} />
    </AuthCard>
  )
}
