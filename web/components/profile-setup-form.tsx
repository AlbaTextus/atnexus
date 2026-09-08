"use client"

import { useRef, useState, type FormEvent } from "react"
import { AlertCircle } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { afterProfileSetup, profileSetupPath } from "@/lib/profile-setup"
import { redirectWithFeedback, useFeedback } from "@/lib/feedback"
import { AuthCard } from "@/components/auth-card"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

export function ProfileSetupForm({ initialName = "", destination }: { initialName?: string; destination: string }) {
  const [name, setName] = useState(initialName)
  const [busy, setBusy] = useState(false)
  const [invalid, setInvalid] = useState(false)
  const [error, setError] = useFeedback("error")
  const inFlight = useRef(false)
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (inFlight.current) return
    const username = name.trim()
    if (!username || [...username].length > 100 || /[\u0000-\u001f\u007f-\u009f]/.test(username)) {
      setInvalid(true); setError("请填写 1–100 个字符的用户名，不要包含换行等控制字符。")
      return
    }
    inFlight.current = true; setBusy(true); setError(""); setInvalid(false)
    try {
      await api("/api/v1/account/setup", { method: "POST", body: JSON.stringify({ username }) })
      redirectWithFeedback(afterProfileSetup(destination), "用户名已设置，欢迎加入")
    } catch (reason) {
      if (reason instanceof ApiError && reason.code === "profile_setup_completed") { window.location.replace(afterProfileSetup(destination)); return }
      if (reason instanceof ApiError && reason.status === 401) { window.location.replace(`/login?return_to=${encodeURIComponent(profileSetupPath(destination))}`); return }
      setInvalid(reason instanceof ApiError && reason.code === "invalid_username")
      setError(reason instanceof ApiError && reason.code === "invalid_username" ? reason.message : "暂时无法保存用户名，请稍后重试。")
      inFlight.current = false; setBusy(false)
    }
  }
  return <AuthCard title="设置用户名" titleId="profile-setup-title" footer={null}>
    <form onSubmit={submit} aria-busy={busy}>
      <FieldGroup>
        {error && <Alert id="profile-setup-error" variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
        <Field data-disabled={busy} data-invalid={invalid}>
          <FieldLabel htmlFor="setup-username">用户名</FieldLabel>
          <Input id="setup-username" name="username" autoComplete="nickname" autoFocus required value={name} onChange={event => { setName(event.target.value); setInvalid(false); setError("") }} disabled={busy} aria-invalid={invalid} aria-describedby={invalid ? "profile-setup-error" : "setup-username-help"} className="h-11" placeholder="希望大家怎么称呼你" />
          <FieldDescription id="setup-username-help">用于展示你的社区身份。</FieldDescription>
        </Field>
        <Button type="submit" size="lg" disabled={busy || !name.trim()} className="h-11 w-full">{busy && <Spinner data-icon="inline-start" />}{busy ? "正在保存…" : "保存并继续"}</Button>
      </FieldGroup>
    </form>
  </AuthCard>
}
