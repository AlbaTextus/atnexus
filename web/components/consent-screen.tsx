"use client"

import { useFeedback } from "@/lib/feedback"

import Link from "next/link"
import { useEffect, useState } from "react"
import { AlertCircle, ArrowRight, ChevronDown, Fingerprint, Globe, KeyRound, Mail, RefreshCw, ShieldCheck, UserRound } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { roleNames } from "@/lib/applications"
import { AuthLayout } from "@/components/auth-layout"
import { ApplicationIcon } from "@/components/application-icon"
import { UserAvatar } from "@/components/user-avatar"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"

type Consent = {
  application: { id: string; name: string; description: string; homepage_url: string; icon_url: string; minimum_role: string }
  user: { id: string; display_name?: string | null; email: string; avatar_url: string; email_is_alias: boolean }
  scopes: string[]
  can_refresh: boolean
  redirect_origin: string
  expires_at: string
}

function consentError(reason: unknown) {
  if (reason instanceof ApiError) {
    if (reason.code === "login_required") return "登录状态已过期，请返回应用重新发起登录。"
    if (reason.code === "invalid_consent") return "这次授权请求已过期、已处理或不属于当前账户，请返回应用重新发起登录。"
    if (reason.code === "consent_changed") return "应用信息或账户权限已变化，请返回应用重新发起登录。"
    if (reason.code === "access_denied") return "当前账户不满足这个应用的访问要求。"
    if (["unauthorized_client", "invalid_request", "invalid_scope"].includes(reason.code)) return "应用的授权请求无效或配置已变化，请返回应用重新发起登录。"
  }
  return "暂时无法完成授权，请稍后重试。"
}

export function ConsentScreen({ requestToken }: { requestToken: string }) {
  const [consent, setConsent] = useState<Consent | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<"allow" | "deny" | "">("")
  const [error, setError] = useFeedback("error")
  const [unusable, setUnusable] = useState(false)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setConsent(null); setLoading(true); setError(""); setUnusable(false)
    if (!/^[A-Za-z0-9_-]{43}$/.test(requestToken)) {
      setLoading(false); setUnusable(true); setError("请从需要登录的应用发起授权请求。")
      return () => controller.abort()
    }
    api<Consent>(`/api/v1/oauth/consent?request=${encodeURIComponent(requestToken)}`, { signal: controller.signal, cache: "no-store" })
      .then(value => { if (!controller.signal.aborted) setConsent(value) })
      .catch(reason => { if (!controller.signal.aborted) { setError(consentError(reason)); setUnusable(reason instanceof ApiError && reason.status < 500) } })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [requestToken, retry])

  useEffect(() => {
    if (!consent) return
    const expire = () => { setUnusable(true); setError("这次授权请求已过期，请返回应用重新发起登录。") }
    const remaining = Date.parse(consent.expires_at) - Date.now()
    if (remaining <= 0) { expire(); return }
    const timer = window.setTimeout(expire, remaining)
    return () => window.clearTimeout(timer)
  }, [consent])

  async function decide(decision: "allow" | "deny") {
    if (busy || loading || !consent || unusable) return
    setBusy(decision); setError("")
    try {
      const result = await api<{ redirect_url: string }>("/api/v1/oauth/consent", { method: "POST", body: JSON.stringify({ request_token: requestToken, decision }) })
      window.location.assign(result.redirect_url)
    } catch (reason) {
      setError(consentError(reason)); setBusy("")
      // A response may have been lost after the single-use decision committed.
      // Only a fresh preview can establish whether this request is still pending.
      setUnusable(true)
    }
  }

  const userName = consent?.user.display_name || (consent?.user.email_is_alias ? "当前账户" : consent?.user.email.split("@")[0]) || "当前账户"
  const permissions = consent ? [
    { id: "identity", icon: Fingerprint, title: "识别你的账户", description: "获取唯一账户标识，用于建立或关联应用内的账户。" },
    ...(consent.scopes.includes("profile") ? [{ id: "profile", icon: UserRound, title: "读取基本资料", description: "包括昵称、头像、账户身份和注册时间。" }] : []),
    ...(consent.scopes.includes("email") ? [{ id: "email", icon: Mail, title: consent.user.email_is_alias ? "读取登录标识" : "读取电子邮箱", description: consent.user.email_is_alias ? "此账户使用第三方登录标识，该标识不是收件邮箱。" : "获取你的邮箱地址，用于在应用中识别账户。" }] : []),
    ...(consent.can_refresh ? [{ id: "refresh", icon: RefreshCw, title: "保持登录状态", description: "允许应用在授权有效期内刷新登录凭据，减少重复登录。" }] : []),
    ...consent.scopes.filter(scope => !["openid", "profile", "email", "offline_access"].includes(scope)).map(scope => ({ id: scope, icon: KeyRound, title: `应用权限：${scope}`, description: "此权限由应用申请，用于提供相应服务。" })),
  ] : []

  return <AuthLayout appearance="white">
    <div className="consent-screen flex w-full max-w-xl flex-col gap-5">
      <Link href="/account" className="consent-brand">ALBATEXTUS</Link>
      {loading ? <div className="flex flex-col gap-5" role="status" aria-label="正在加载应用授权"><Skeleton className="mx-auto size-20" /><Skeleton className="mx-auto h-8 w-48" /><Skeleton className="h-24 w-full" /><Skeleton className="h-72 w-full" /></div> : <>
        <header className="consent-heading">
          {consent ? <ApplicationIcon url={consent.application.icon_url} name={consent.application.name} /> : <ShieldCheck className="mx-auto size-12 text-muted-foreground" aria-hidden="true" />}
          <h1>{consent?.application.name || "应用授权"}</h1>
        </header>
        {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
        {consent && <>
          <Card><CardHeader className="flex flex-row items-center gap-3"><UserAvatar src={consent.user.avatar_url} name={userName} className="size-11" /><div className="min-w-0"><CardTitle>{userName}</CardTitle><CardDescription className="break-all">{consent.user.email_is_alias ? "以当前第三方账户的身份授权" : consent.user.email}</CardDescription></div></CardHeader></Card>
          <Card><CardHeader className="justify-items-center text-center"><CardTitle>应用信息</CardTitle><CardDescription>{consent.application.description || "此应用已接入 ALBATEXTUS 统一登录。"}</CardDescription></CardHeader><CardContent className="flex flex-col gap-5">
            <div className="flex flex-col gap-3 text-center text-sm">
              {consent.application.homepage_url && <div className="flex min-w-0 items-start justify-center gap-2"><Globe className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><a href={consent.application.homepage_url} target="_blank" rel="noopener noreferrer" className="break-all text-link underline-offset-4 hover:underline">{consent.application.homepage_url}</a></div>}
              <div className="flex flex-wrap items-center justify-center gap-2"><ShieldCheck className="size-4 text-muted-foreground" aria-hidden="true" /><span className="text-muted-foreground">最低访问身份</span><Badge variant="secondary">{roleNames[consent.application.minimum_role] || consent.application.minimum_role}</Badge></div>
            </div>
            <Separator />
            <Collapsible>
              <h2><CollapsibleTrigger render={<Button variant="ghost" className="group w-full justify-between" />}><span>基本信息</span><ChevronDown data-icon="inline-end" className="transition-transform duration-300 ease-out group-aria-expanded:rotate-180" /></CollapsibleTrigger></h2>
              <CollapsibleContent className="consent-permissions-panel">
                <div className="consent-permissions-body flex flex-col gap-4 pt-4">
                <p className="text-sm text-muted-foreground">获取下列信息</p>
                <ul className="flex flex-col gap-4">{permissions.map(permission => <li key={permission.id} className="flex items-start gap-3"><permission.icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden="true" /><div className="min-w-0"><p className="break-words text-sm font-medium">{permission.title}</p><p className="mt-1 text-xs leading-6 text-muted-foreground">{permission.description}</p></div></li>)}</ul>
                </div>
              </CollapsibleContent>
            </Collapsible>
          </CardContent></Card>
          {!unusable && <div className="flex flex-col gap-3"><Button size="lg" className="h-12 w-full" disabled={Boolean(busy)} onClick={() => void decide("allow")}>{busy === "allow" ? <Spinner data-icon="inline-start" /> : <ShieldCheck data-icon="inline-start" />}允许并继续<ArrowRight data-icon="inline-end" /></Button><Button variant="outline" size="lg" className="h-12 w-full" disabled={Boolean(busy)} onClick={() => void decide("deny")}>{busy === "deny" && <Spinner data-icon="inline-start" />}拒绝</Button></div>}
        </>}
        {(!consent || unusable) && !loading && <div className="flex flex-wrap justify-center gap-3">{requestToken && <Button variant="outline" onClick={() => setRetry(value => value + 1)}><RefreshCw data-icon="inline-start" />重新检查</Button>}<Button nativeButton={false} role="link" render={<Link href="/account" />}>返回用户中心</Button></div>}
      </>}
      <nav className="flex items-center justify-center gap-4 text-xs text-muted-foreground" aria-label="授权说明"><Link href="/wiki/privacy" target="_blank" rel="noopener noreferrer" className="hover:text-foreground">隐私政策</Link><Link href="/wiki/terms" target="_blank" rel="noopener noreferrer" className="hover:text-foreground">用户协议</Link></nav>
    </div>
  </AuthLayout>
}
