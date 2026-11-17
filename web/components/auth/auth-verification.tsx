"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { useTheme } from "next-themes"
import { AlertCircle, RotateCw, Shield, ShieldCheck } from "lucide-react"
import { api } from "@/lib/api"
import { loadTurnstile } from "@/lib/auth/turnstile"
import { wikiUrl } from "@/lib/wiki"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

type VerificationConfig = { enabled: boolean; site_key: string; test_mode: boolean; action: string; email_enabled: boolean; registration_enabled: boolean; external_login_enabled: boolean }

export function useAuthVerification({ requireAgreement = false }: { requireAgreement?: boolean } = {}) {
  const [config, setConfig] = useState<VerificationConfig | null>(null)
  const [token, setToken] = useState("")
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const [retry, setRetry] = useState(0)
  const [agreementAccepted, setAgreementAccepted] = useState(false)
  useEffect(() => {
    const controller = new AbortController()
    setError("")
    api<VerificationConfig>("/api/v1/auth/verification", { signal: controller.signal })
      .then(value => {
        if (controller.signal.aborted) return
        if (typeof value.enabled !== "boolean" || (value.enabled && (!value.site_key || value.action !== "authenticate"))) throw new Error("Invalid verification configuration")
        setConfig(value)
      })
      .catch(() => { if (!controller.signal.aborted) setError("无法加载安全验证，请重试。") })
    return () => controller.abort()
  }, [retry])
  const reset = useCallback(() => { setToken(""); setError(""); setAgreementAccepted(false); setRevision(value => value + 1) }, [])
  const onToken = useCallback((value: string) => { setToken(value); setError(""); setAgreementAccepted(false) }, [])
  const onError = useCallback((message: string) => { setToken(""); setError(message); setAgreementAccepted(false) }, [])
  const reload = useCallback(() => { reset(); setRetry(value => value + 1) }, [reset])
  const humanReady = Boolean(config && !error && (!config.enabled || token))
  const ready = humanReady && (!requireAgreement || agreementAccepted)
  const acceptAgreement = () => { if (humanReady) setAgreementAccepted(true) }
  const headers: Record<string, string> = config?.enabled ? { "X-Turnstile-Token": token } : {}
  return { config, error, revision, reset, onToken, onError, reload, ready, headers, humanReady, requireAgreement, agreementAccepted, acceptAgreement }
}

function TurnstileWidget({ config, onToken, onError }: {
  config: VerificationConfig
  onToken: (token: string) => void
  onError: (message: string) => void
}) {
  const { resolvedTheme } = useTheme()
  const theme = resolvedTheme === "dark" ? "dark" : "light"
  const wrapper = useRef<HTMLDivElement>(null)
  const container = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState<"flexible" | "compact" | null>(null)
  useEffect(() => {
    const element = wrapper.current
    if (!element) return
    const measure = () => setSize(element.clientWidth < 300 ? "compact" : "flexible")
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  useEffect(() => {
    if (!size || !container.current) return
    const element = container.current
    let disposed = false
    let widgetID: string | undefined
    onToken("")
    loadTurnstile().then(client => {
      if (disposed) return
      widgetID = client.render(element, {
        sitekey: config.site_key, action: config.action, theme, language: "zh-cn", size,
        "response-field": false, "refresh-expired": "auto",
        callback: value => { if (!disposed) onToken(value) },
        "expired-callback": () => { if (!disposed) onError("安全验证已过期，正在重新验证。") },
        "timeout-callback": () => { if (!disposed) onError("安全验证超时，请重试。") },
        "error-callback": () => { if (!disposed) onError("安全验证暂时无法完成，请重试。"); return true },
      })
    }).catch(() => { if (!disposed) onError("安全验证加载失败，请检查网络后重试。") })
    return () => {
      disposed = true
      if (widgetID !== undefined) window.turnstile?.remove(widgetID)
    }
  }, [config.site_key, config.action, size, theme, onToken, onError])

  return <div ref={wrapper} className="flex w-full justify-center" data-widget-size={size} role="group" aria-label="Cloudflare Turnstile 安全验证"><div ref={container} className={size === "compact" ? "min-h-36" : "min-h-16 w-full"} /></div>
}

export function AuthVerification({ verification, disabled = false }: { verification: ReturnType<typeof useAuthVerification>; disabled?: boolean }) {
  const { config, error } = verification
  const verified = Boolean(config?.enabled && verification.humanReady)
  const [settled, setSettled] = useState(false)
  const [closing, setClosing] = useState(false)
  const panel = useRef<HTMLDivElement>(null)
  const summary = useRef<HTMLDivElement>(null)
  const widget = useRef<HTMLDivElement>(null)
  const agreementButton = useRef<HTMLButtonElement>(null)
  const showAgreement = verification.requireAgreement && verification.humanReady && (!config?.enabled || settled)
  const closed = verification.requireAgreement ? verification.ready && closing : verified && settled

  useEffect(() => {
    setSettled(false)
    if (!verified) return
    // Keep the widget mounted: collapsing it must not discard the token or
    // its expiry callback. Leave a moment for the success feedback to register.
    const delay = window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 1200
    const timer = window.setTimeout(() => {
      if (!verification.requireAgreement && panel.current?.contains(document.activeElement)) summary.current?.focus({ preventScroll: true })
      setSettled(true)
    }, delay)
    return () => window.clearTimeout(timer)
  }, [verified, verification.requireAgreement])

  useEffect(() => {
    if (showAgreement && widget.current?.contains(document.activeElement)) agreementButton.current?.focus({ preventScroll: true })
  }, [showAgreement])

  useEffect(() => {
    setClosing(false)
    if (!verification.requireAgreement || !verification.ready) return
    const timer = window.setTimeout(() => {
      if (panel.current?.contains(document.activeElement)) summary.current?.focus({ preventScroll: true })
      setClosing(true)
    }, window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 280)
    return () => window.clearTimeout(timer)
  }, [verification.requireAgreement, verification.ready])

  if (config && !config.enabled && !verification.requireAgreement) return null
  return <div className="verification-gate" data-state={closed ? "closed" : "open"} data-verified={verified} data-agreement={verification.requireAgreement || undefined}>
    <div ref={summary} className="verification-gate-summary" tabIndex={-1} role="status" aria-live="polite" aria-atomic="true">
      <span className="flex min-w-0 items-center gap-2">{verified ? <ShieldCheck className="size-4 shrink-0" aria-hidden="true" /> : <Shield className="size-4 shrink-0" aria-hidden="true" />}<span>{verification.requireAgreement && verification.ready ? "已同意相关条款" : showAgreement ? "请阅读并同意以下条款" : verified ? "已通过安全验证" : error ? "需要重新验证" : config && !config.enabled ? "隐私政策与用户协议" : "安全验证"}</span></span>
      {config?.test_mode && <Badge variant="secondary">测试模式</Badge>}
    </div>
    <div ref={panel} className="verification-gate-panel" aria-hidden={closed || undefined} inert={closed}>
      <div className="verification-gate-clip">
        <div className="flex flex-col gap-3 pt-3">
          <div className="verification-stage" data-agreement-visible={showAgreement}>
            <div ref={widget} className="verification-widget" aria-hidden={showAgreement || undefined} inert={showAgreement}>
              {config?.enabled ? <TurnstileWidget key={verification.revision} config={config} onToken={verification.onToken} onError={verification.onError} /> : !config && !error ? <Skeleton className="h-16 w-full" role="status" aria-label="正在加载安全验证" /> : <div className="h-16" />}
            </div>
            {verification.requireAgreement && <div className="verification-agreement" aria-hidden={!showAgreement || undefined} inert={!showAgreement}>
              <Button ref={agreementButton} type="button" size="sm" className="min-w-28" disabled={disabled || !showAgreement || verification.agreementAccepted} onClick={verification.acceptAgreement}>{verification.agreementAccepted ? "已同意" : "同意并继续"}</Button>
              <p><a href={wikiUrl("/privacy")} target="_blank" rel="noopener noreferrer" aria-label="隐私政策（新窗口）">隐私政策</a>与<a href={wikiUrl("/terms")} target="_blank" rel="noopener noreferrer" aria-label="用户协议（新窗口）">用户协议</a></p>
            </div>}
          </div>
          {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button type="button" variant="link" disabled={disabled} onClick={verification.reload}><RotateCw data-icon="inline-start" />重新验证</Button></AlertDescription></Alert>}
        </div>
      </div>
    </div>
  </div>
}
