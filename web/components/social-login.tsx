"use client"

import { useEffect, useState, type ReactNode } from "react"
import { AlertCircle } from "lucide-react"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"
import { managementError } from "@/lib/applications"
import { authErrorMessage } from "@/lib/auth-errors"
import { type LoginProvider } from "@/lib/external-login"
import { ProviderIcon } from "@/components/provider-icon"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"

export function SocialLogin({ returnTo, disabled = false, onBusyChange, verificationHeaders, onAttemptFailed, children }: { returnTo: string; disabled?: boolean; onBusyChange?: (busy: boolean) => void; verificationHeaders?: Record<string, string>; onAttemptFailed?: () => void; children?: ReactNode }) {
  const [providers, setProviders] = useState<LoginProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState("")
  const [error, setError] = useState("")
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError("")
    api<{ providers: LoginProvider[] }>("/api/v1/auth/providers", { signal: controller.signal }).then(data => setProviders(data.providers)).catch(reason => { if (!controller.signal.aborted) setError(managementError(reason)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [retry])

  async function login(provider: LoginProvider) {
    if (disabled || busy || !provider.enabled) return
    setBusy(provider.id); setError(""); onBusyChange?.(true)
    try {
      const result = await api<{ authorization_url: string }>(`/api/v1/auth/providers/${provider.id}/start`, { method: "POST", body: JSON.stringify({ return_to: returnTo }), headers: verificationHeaders })
      window.location.assign(result.authorization_url)
    } catch (reason) { setError(authErrorMessage(reason, managementError(reason))); setBusy(""); onBusyChange?.(false); onAttemptFailed?.() }
  }

  return <div className="flex flex-col gap-3">
    <div className={cn("grid", children ? "auth-login-methods grid-cols-3 gap-2" : "grid-cols-2 gap-3")}>
      {loading ? <Skeleton className="col-span-2 h-11 w-full" /> : providers.map(provider => {
        return <Button key={provider.id} type="button" variant="outline" size={children ? "sm" : "lg"} className={cn("h-11 min-w-0", children && "px-1.5 max-[400px]:flex-col max-[400px]:gap-0.5")} disabled={disabled || Boolean(busy) || !provider.enabled} title={!provider.enabled ? "暂未开放" : undefined} onClick={() => void login(provider)}>{busy === provider.id ? <Spinner data-icon="inline-start" aria-hidden="true" /> : <ProviderIcon provider={provider.id} data-icon="inline-start" />}<span className="truncate">{provider.name}</span></Button>
      })}
      {children}
    </div>
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button type="button" variant="link" disabled={Boolean(busy)} onClick={() => setRetry(value => value + 1)}>重试</Button></AlertDescription></Alert>}
  </div>
}
