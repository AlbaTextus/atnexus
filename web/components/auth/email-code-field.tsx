"use client"

import { useEffect, useRef, useState } from "react"
import { api, ApiError } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

type Challenge = { challenge_id: string; expires_in: number; retry_after: number }

export function useEmailCode(email: string) {
  const [code, setCode] = useState("")
  const [challenge, setChallenge] = useState<{ id: string; email: string; expires: number } | null>(null)
  const [sending, setSending] = useState(false)
  const [retryAt, setRetryAt] = useState(0)
  const [now, setNow] = useState(0)
  const inFlight = useRef(false)
  useEffect(() => {
    const tick = () => setNow(Date.now())
    tick()
    const timer = window.setInterval(tick, 1000)
    return () => window.clearInterval(timer)
  }, [])
  const normalizedEmail = email.trim().toLowerCase()
  const challengeID = challenge?.email === normalizedEmail && challenge.expires > now ? challenge.id : ""
  const cooldown = Math.max(0, Math.ceil((retryAt - now) / 1000))
  function reset() { setCode(""); setChallenge(null) }
  async function send(path: string, headers?: Record<string, string>) {
    if (inFlight.current || Date.now() < retryAt) return false
    inFlight.current = true
    setSending(true)
    reset()
    try {
      const result = await api<Challenge>(path, { method: "POST", body: JSON.stringify({ email: normalizedEmail }), headers })
      const current = Date.now()
      setNow(current)
      setChallenge({ id: result.challenge_id, email: normalizedEmail, expires: current + result.expires_in * 1000 })
      setRetryAt(current + result.retry_after * 1000)
      return true
    } catch (error) {
      if (error instanceof ApiError && error.status === 429) { setNow(Date.now()); setRetryAt(Date.now() + 60000) }
      throw error
    } finally { inFlight.current = false; setSending(false) }
  }
  return { code, setCode, challengeID, cooldown, sending, send, reset }
}

export function EmailCodeField({ id, verification, disabled, sendDisabled, onSend }: {
  id: string
  verification: ReturnType<typeof useEmailCode>
  disabled: boolean
  sendDisabled?: boolean
  onSend: () => void
}) {
  return <Field data-disabled={disabled}>
    <FieldLabel htmlFor={id}>邮箱验证码</FieldLabel>
    <div className="flex flex-wrap items-center gap-2">
      <Input id={id} name="email_code" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} required value={verification.code} onChange={event => verification.setCode(event.target.value.replace(/\D/g, "").slice(0, 6))} disabled={disabled || !verification.challengeID} className="h-11 min-w-28 flex-1" placeholder="6 位验证码" aria-describedby={`${id}-help`} />
      <Button type="button" variant="outline" disabled={disabled || sendDisabled || verification.cooldown > 0} onClick={onSend} className="h-11">
        {verification.sending && <Spinner data-icon="inline-start" />}
        {verification.sending ? "正在发送…" : verification.cooldown > 0 ? `${verification.cooldown} 秒后重发` : "发送验证码"}
      </Button>
    </div>
    <FieldDescription id={`${id}-help`} role="status">{verification.challengeID ? "验证码已发送，5 分钟内有效，请检查收件箱或垃圾邮件。" : "请先向此邮箱发送验证码。"}</FieldDescription>
  </Field>
}
