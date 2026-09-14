"use client"

import { useFeedback, feedback, redirectWithFeedback } from "@/lib/feedback"

import { useState, type FormEvent } from "react"
import { AlertCircle } from "lucide-react"
import { PasswordInput } from "@/components/password-input"
import { AuthCard } from "@/components/auth-card"
import { AuthFooter } from "@/components/auth-footer"
import { SocialLogin } from "@/components/social-login"
import { AuthVerification, useAuthVerification } from "@/components/auth-verification"
import { EmailCodeField, useEmailCode } from "@/components/email-code-field"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldGroup, FieldLabel, FieldSeparator } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { api, safeReturnTo } from "@/lib/api"
import { authErrorMessage } from "@/lib/auth-errors"
import { profileSetupPath } from "@/lib/profile-setup"

type RegisterFormProps = { returnTo: string | null }

export function RegisterForm({ returnTo }: RegisterFormProps) {
  const verification = useAuthVerification({ requireAgreement: true })
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [submitting, setBusy] = useState(false)
  const emailVerification = useEmailCode(email)
  const busy = submitting || emailVerification.sending
  const [error, setError] = useFeedback("error")
  const [confirmError, setConfirmError] = useState(false)
  const destination = safeReturnTo(returnTo)
  const registrationDisabled = verification.config?.registration_enabled === false

  async function sendCode() {
    if (busy || registrationDisabled || !verification.ready || !verification.config?.email_enabled) return
    const input = document.getElementById("email") as HTMLInputElement | null
    if (!input?.reportValidity()) return
    setError("")
    try { await emailVerification.send("/api/v1/auth/email/code", verification.headers) }
    catch (reason) { setError(authErrorMessage(reason, "验证码发送失败，请稍后重试。")) }
    finally { verification.reset() }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy || registrationDisabled || !verification.ready || !emailVerification.challengeID || !verification.config?.email_enabled) return
    setError("")
    setConfirmError(false)
    if (password !== confirm) {
      setConfirmError(true); feedback.error("两次输入的密码不一致。")
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
    setBusy(true)
    try {
      await api("/api/v1/users", { method: "POST", body: JSON.stringify({ email: email.trim(), password, challenge_id: emailVerification.challengeID, email_code: emailVerification.code }), headers: verification.headers })
      redirectWithFeedback(profileSetupPath(destination), "账户已创建")
    } catch (reason) {
      setError(authErrorMessage(reason, "暂时无法创建账户，请稍后重试。"))
      verification.reset()
      setBusy(false)
    }
  }

  return (
    <AuthCard
      title="创建你的账户" titleId="register-title"
      footer={<AuthFooter returnTo={returnTo} from="register" />}
    >
      {error && <Alert id="register-error" variant="destructive" className="mb-5"><AlertCircle aria-hidden="true" /><AlertDescription>{error}</AlertDescription></Alert>}

      <form onSubmit={submit} onChange={() => setError("")} aria-busy={busy} aria-describedby={error ? "register-error" : undefined}>
        <FieldGroup className="gap-5">
          <Field data-disabled={busy || registrationDisabled}>
            <FieldLabel htmlFor="email" className="sr-only">电子邮箱</FieldLabel>
            <Input id="email" name="email" type="email" autoComplete="username" autoCapitalize="none" spellCheck={false} required maxLength={320} value={email} onChange={(event) => { setEmail(event.target.value); emailVerification.reset() }} disabled={busy || registrationDisabled} className="h-11 px-3" placeholder="邮箱" />
          </Field>
          <Field data-disabled={busy || registrationDisabled}>
            <FieldLabel htmlFor="password" className="sr-only">密码</FieldLabel>
            <PasswordInput id="password" name="password" autoComplete="new-password" required minLength={12} maxLength={72} value={password} onChange={(event) => { setPassword(event.target.value); setConfirmError(false) }} disabled={busy || registrationDisabled} placeholder="密码（至少 12 个字符）" />
          </Field>
          <Field data-disabled={busy || registrationDisabled} data-invalid={confirmError}>
            <FieldLabel htmlFor="confirm" className="sr-only">确认密码</FieldLabel>
            <PasswordInput id="confirm" name="confirm" label="确认密码" autoComplete="new-password" required minLength={12} maxLength={72} value={confirm} onChange={(event) => { setConfirm(event.target.value); setConfirmError(false) }} disabled={busy || registrationDisabled} placeholder="再次输入你的密码" aria-invalid={confirmError || undefined} aria-describedby={confirmError ? "confirm-error" : undefined} />
            {confirmError && <FieldError id="confirm-error">两次输入的密码不一致，请重新确认。</FieldError>}
          </Field>
          <AuthVerification verification={verification} disabled={busy} />
          {registrationDisabled && <Alert><AlertCircle /><AlertDescription>全站注册已关闭，已有账户仍可返回登录。</AlertDescription></Alert>}
          {verification.config && verification.config.registration_enabled && !verification.config.email_enabled && <Alert><AlertCircle /><AlertDescription>邮箱注册暂不可用，请稍后重试或使用第三方账号登录。</AlertDescription></Alert>}
          <EmailCodeField id="register-email-code" verification={emailVerification} disabled={busy || registrationDisabled} sendDisabled={!email || registrationDisabled || !verification.ready || !verification.config?.email_enabled} onSend={sendCode} />
          <Button type="submit" size="lg" disabled={busy || registrationDisabled || !verification.ready || !verification.config?.email_enabled || !emailVerification.challengeID || emailVerification.code.length !== 6} className="mt-1 h-11 w-full">
            {busy && <Spinner data-icon="inline-start" aria-hidden="true" />}
            {busy ? "正在创建…" : "创建账户"}
          </Button>
        </FieldGroup>
      </form>
      <SocialLogin returnTo={destination} disabled={busy || !verification.ready} onBusyChange={setBusy} verificationHeaders={verification.headers} onAttemptFailed={verification.reset} separator={<FieldSeparator className="my-6">或使用第三方账号</FieldSeparator>} />
    </AuthCard>
  )
}
