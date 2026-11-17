"use client"

import { useFeedback } from "@/lib/feedback"

import Link from "next/link"
import { useEffect, useState } from "react"
import { AlertCircle, ArrowRight, ChevronDown, Fingerprint, Globe, KeyRound, Mail, RefreshCw, ShieldCheck, UserRound } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { roleNames } from "@/lib/admin/applications"
import { AuthLayout } from "@/components/auth/auth-layout"
import { UserAvatar } from "@/components/shared/user-avatar"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardFooter, CardHeader } from "@/components/ui/card"
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
    <div className="consent-screen">
      <Card className="consent-card account-member-card" aria-busy={loading || Boolean(busy)}>
        <CardHeader className="consent-card-heading">
          <div className="account-member-heading"><Link href="/account" className="account-member-brand">ALBATEXTUS</Link><Fingerprint className="size-7" aria-hidden="true" /></div>
        </CardHeader>
        {loading ? <CardContent className="consent-loading" role="status" aria-label="正在加载应用授权">
          <div className="consent-loading-heading">
            <span className="consent-loading-mark" aria-hidden="true"><Fingerprint /></span>
            <p>正在准备授权信息<span className="consent-loading-dots" aria-hidden="true"><i /><i /><i /></span></p>
          </div>
          <div className="consent-identity" aria-hidden="true">
            <Skeleton className="size-11 shrink-0 rounded-full" />
            <div className="flex flex-1 flex-col gap-3"><Skeleton className="h-3 w-20" /><Skeleton className="h-3 w-3/5" /></div>
            <ShieldCheck className="consent-identity-seal" />
          </div>
          <div className="flex flex-col gap-6" aria-hidden="true">
            <div className="flex items-center justify-between gap-4"><Skeleton className="h-4 w-2/5" /><Skeleton className="h-3 w-12" /></div>
            <Separator />
            <Skeleton className="h-4 w-1/4" />
          </div>
          <div className="consent-actions" aria-hidden="true"><Skeleton className="h-12 w-full" /><Skeleton className="h-12 w-full" /></div>
        </CardContent> : <>
          <CardContent className="consent-card-body">
            {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
            {consent && <>
              <div className="consent-application">
                <div className="min-w-0"><h2>{consent.application.name}</h2><p>希望使用你的 ALBATEXTUS 账户登录</p></div>
              </div>
              <div className="consent-identity">
                <UserAvatar src={consent.user.avatar_url} name={userName} className="size-11 shrink-0" />
                <div className="min-w-0"><p className="consent-identity-label">当前授权身份</p><p className="consent-identity-name">{userName}</p><p className="consent-identity-email">{consent.user.email_is_alias ? "第三方登录账户" : consent.user.email}</p></div>
                <ShieldCheck className="consent-identity-seal" aria-hidden="true" />
              </div>
              <section className="consent-access" aria-labelledby="consent-access-title">
                <Collapsible>
                  <h2 id="consent-access-title"><CollapsibleTrigger render={<Button variant="ghost" className="consent-access-trigger group w-full justify-between" />}><span>此应用将获得以下权限</span><span className="consent-access-count">{String(permissions.length).padStart(2, "0")} 项<ChevronDown data-icon="inline-end" className="transition-transform duration-300 ease-out group-aria-expanded:rotate-180" /></span></CollapsibleTrigger></h2>
                  <CollapsibleContent className="consent-permissions-panel">
                    <div className="consent-permissions-body pt-4"><ul className="consent-permission-list">{permissions.map(permission => <li key={permission.id}><span className="consent-permission-icon"><permission.icon aria-hidden="true" /></span><div className="min-w-0"><h3>{permission.title}</h3><p>{permission.description}</p></div></li>)}</ul></div>
                  </CollapsibleContent>
                </Collapsible>
              </section>
              <Separator />
              <Collapsible className="consent-details">
                <h2><CollapsibleTrigger render={<Button variant="ghost" className="group w-full justify-between" />}><span>关于此应用</span><ChevronDown data-icon="inline-end" className="transition-transform duration-300 ease-out group-aria-expanded:rotate-180" /></CollapsibleTrigger></h2>
                <CollapsibleContent className="consent-permissions-panel">
                  <div className="consent-permissions-body consent-details-body">
                    <p>{consent.application.description || "此应用已接入 ALBATEXTUS 统一登录。"}</p>
                    {consent.application.homepage_url && <a href={consent.application.homepage_url} target="_blank" rel="noopener noreferrer"><Globe aria-hidden="true" />{consent.application.homepage_url}</a>}
                    <div className="flex flex-wrap items-center gap-2"><span>最低访问身份</span><Badge variant="secondary">{roleNames[consent.application.minimum_role] || consent.application.minimum_role}</Badge></div>
                    <p>授权后返回 <span className="consent-return-origin">{consent.redirect_origin}</span></p>
                  </div>
                </CollapsibleContent>
              </Collapsible>
            </>}
            {(!consent || unusable) && <div className="flex flex-wrap gap-3">{requestToken && <Button variant="outline" onClick={() => setRetry(value => value + 1)}><RefreshCw data-icon="inline-start" />重新检查</Button>}<Button nativeButton={false} role="link" render={<Link href="/account" />}>返回用户中心</Button></div>}
          </CardContent>
          {consent && !unusable && <CardFooter className="consent-card-footer">
            <div className="consent-actions">
              <Button variant="outline" size="lg" disabled={Boolean(busy)} onClick={() => void decide("deny")}>{busy === "deny" && <Spinner data-icon="inline-start" />}拒绝</Button>
              <Button size="lg" disabled={Boolean(busy)} onClick={() => void decide("allow")}>{busy === "allow" && <Spinner data-icon="inline-start" />}允许并继续<ArrowRight data-icon="inline-end" /></Button>
            </div>
          </CardFooter>}
        </>}
      </Card>
    </div>
  </AuthLayout>
}
