"use client"

import { useEffect, useState, type FormEvent } from "react"
import { AlertCircle, KeyRound } from "lucide-react"
import { browserSupportsWebAuthn, startAuthentication } from "@simplewebauthn/browser"
import { PasswordInput } from "@/components/password-input"
import { AuthCard } from "@/components/auth-card"
import { AuthWordmark } from "@/components/auth-wordmark"
import { AuthFooter } from "@/components/auth-footer"
import { SocialLogin } from "@/components/social-login"
import { AuthVerification, useAuthVerification } from "@/components/auth-verification"
import { externalLoginError } from "@/lib/external-login"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldGroup, FieldLabel, FieldSeparator } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { api, safeReturnTo } from "@/lib/api"
import { authErrorMessage } from "@/lib/auth-errors"

type LoginFormProps = { returnTo: string | null; oauthError?: string | null }
type UserResponse = { id: string; email: string; display_name?: string | null }
type PasskeyOptions = { ceremony_token: string; publicKey: Record<string, unknown> }
const rememberedEmailKey = "albatextus.remembered-email"

export function LoginForm({ returnTo, oauthError }: LoginFormProps) {
  const verification = useAuthVerification({ requireAgreement: true })
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [rememberEmail, setRememberEmail] = useState(false)
  const [busy, setBusy] = useState(false)
  const [passkeyBusy, setPasskeyBusy] = useState(false)
  const [error, setError] = useState(externalLoginError(oauthError || null))
  const [socialBusy, setSocialBusy] = useState(false)
  const destination = safeReturnTo(returnTo)
  const pending = busy || passkeyBusy || socialBusy

  useEffect(() => {
    try {
      const remembered = localStorage.getItem(rememberedEmailKey)
      if (remembered) {
        setEmail(remembered)
        setRememberEmail(true)
      }
    } catch {
      // Storage can be unavailable in private or restricted browsers.
    }
  }, [])

  function changeRememberEmail(checked: boolean) {
    setRememberEmail(checked)
    if (!checked) {
      try { localStorage.removeItem(rememberedEmailKey) } catch { /* Login does not require storage. */ }
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || !verification.ready) return
    setBusy(true)
    setError("")
    try {
      const normalizedEmail = email.trim()
      await api<UserResponse>("/api/v1/sessions", {
        method: "POST",
        body: JSON.stringify({ email: normalizedEmail, password }),
        headers: verification.headers,
      })
      try {
        if (rememberEmail) localStorage.setItem(rememberedEmailKey, normalizedEmail)
        else localStorage.removeItem(rememberedEmailKey)
      } catch { /* A storage preference must not prevent a successful login. */ }
      window.location.href = destination
    } catch (reason) {
      setError(authErrorMessage(reason, "暂时无法登录，请稍后重试。"))
      verification.reset()
      setBusy(false)
    }
  }

  async function passkeyLogin() {
    if (pending || !verification.ready) return
    setError("")
    if (!browserSupportsWebAuthn()) {
      setError("当前浏览器不支持通行密钥，请使用邮箱和密码登录。")
      return
    }
    setPasskeyBusy(true)
    try {
      const ceremony = await api<PasskeyOptions>("/api/v1/passkeys/login/options", { method: "POST", headers: verification.headers })
      const assertion = await startAuthentication({ optionsJSON: ceremony.publicKey as never })
      await api<UserResponse>("/api/v1/passkeys/login/finish", {
        method: "POST",
        body: JSON.stringify(assertion),
        headers: { "X-WebAuthn-Ceremony": ceremony.ceremony_token },
      })
      window.location.href = destination
    } catch (reason) {
      setError(authErrorMessage(reason, "通行密钥登录失败，请重试或使用邮箱登录。"))
      verification.reset()
      setPasskeyBusy(false)
    }
  }

  return (
    <AuthCard
      title="欢迎回来" titleId="login-title"
      brand={<AuthWordmark verified={Boolean(verification.config?.enabled && verification.ready)} />}
      footer={<AuthFooter returnTo={returnTo} />}
    >
      {error && <Alert id="login-error" variant="destructive" className="mb-5"><AlertCircle aria-hidden="true" /><AlertDescription>{error}</AlertDescription></Alert>}

      <form onSubmit={submit} aria-busy={pending} aria-describedby={error ? "login-error" : undefined}>
        <FieldGroup className="gap-5">
          <Field data-disabled={pending}>
            <FieldLabel htmlFor="email" className="sr-only">电子邮箱</FieldLabel>
            <Input
              id="email" name="email" type="email" autoComplete="username" autoCapitalize="none" spellCheck={false}
              required maxLength={320} value={email} disabled={pending} className="h-11 px-3" placeholder="邮箱或登录标识"
              onChange={(event) => { setEmail(event.target.value); setError("") }}
            />
          </Field>
          <Field data-disabled={pending}>
            <FieldLabel htmlFor="password" className="sr-only">密码</FieldLabel>
            <PasswordInput
              id="password" name="password" autoComplete="current-password" required value={password} disabled={pending}
              placeholder="密码" onChange={(event) => { setPassword(event.target.value); setError("") }}
            />
          </Field>
          <Field orientation="horizontal" data-disabled={pending} className="gap-2.5">
            <Checkbox id="remember-email" name="remember-email" checked={rememberEmail} disabled={pending} onCheckedChange={changeRememberEmail} />
            <FieldLabel htmlFor="remember-email">记住邮箱</FieldLabel>
          </Field>
          <AuthVerification verification={verification} disabled={pending} />
          <Button type="submit" size="lg" disabled={pending || !verification.ready} className="h-11 w-full">
            {busy && <Spinner data-icon="inline-start" aria-hidden="true" />}
            {busy ? "正在登录…" : "登录"}
          </Button>
        </FieldGroup>
      </form>

      <FieldSeparator className="my-6" />
      <SocialLogin returnTo={destination} disabled={pending || !verification.ready} onBusyChange={setSocialBusy} verificationHeaders={verification.headers} onAttemptFailed={verification.reset}>
        <Button type="button" variant="outline" size="sm" onClick={passkeyLogin} disabled={pending || !verification.ready} className="h-11 min-w-0 px-1.5 max-[400px]:flex-col max-[400px]:gap-0.5" aria-label={passkeyBusy ? "正在验证通行密钥" : "使用通行密钥登录"}>
          {passkeyBusy ? <Spinner data-icon="inline-start" aria-hidden="true" /> : <KeyRound data-icon="inline-start" aria-hidden="true" />}
          <span className="truncate">{passkeyBusy ? "验证中…" : "通行密钥"}</span>
        </Button>
      </SocialLogin>
    </AuthCard>
  )
}
