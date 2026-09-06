"use client"

import { useEffect, useRef, useState } from "react"
import { AlertCircle, Check, Link2, Unlink } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { externalLoginError, type ExternalIdentity, type LoginProvider } from "@/lib/external-login"
import { ProviderIcon } from "@/components/provider-icon"
import { UserAvatar } from "@/components/user-avatar"
import { useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"

export function LinkedAccounts() {
  const updateUser = useUpdateAccountUser()
  const [providers, setProviders] = useState<LoginProvider[]>([])
  const [identities, setIdentities] = useState<ExternalIdentity[]>([])
  const [hasPassword, setHasPassword] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const [busy, setBusy] = useState("")
  const [unlinking, setUnlinking] = useState<LoginProvider | null>(null)
  const callbackHandled = useRef(false)

  async function load() {
    setLoading(true)
    try {
      const [p, i] = await Promise.all([api<{ providers: LoginProvider[] }>("/api/v1/auth/providers"), api<{ identities: ExternalIdentity[]; has_password: boolean }>("/api/v1/account/identities")])
      setProviders([...p.providers, ...i.identities.filter(identity => !p.providers.some(provider => provider.id === identity.provider)).map(identity => ({ id: identity.provider, name: identity.provider, enabled: false }))])
      setIdentities(i.identities); setHasPassword(i.has_password)
    } catch (reason) { setError(managementError(reason)) } finally { setLoading(false) }
  }
  useEffect(() => {
    if (!callbackHandled.current) {
    callbackHandled.current = true
    const query = new URLSearchParams(window.location.search)
    setError(externalLoginError(query.get("oauth_error")))
    if (query.get("oauth_linked") === "1") setNotice("第三方账号已绑定，下次可以直接使用该账号登录。")
    if (query.has("oauth_error") || query.has("oauth_linked")) {
      query.delete("oauth_error"); query.delete("oauth_linked")
      window.history.replaceState(null, "", window.location.pathname + (query.size ? `?${query}` : ""))
    }
    }
    void load()
  }, [])

  async function link(provider: LoginProvider) {
    if (busy) return
    setBusy(provider.id); setError(""); setNotice("")
    try {
      const response = await api<{ authorization_url: string }>(`/api/v1/auth/providers/${provider.id}/start`, { method: "POST", body: JSON.stringify({ link: true }) })
      window.location.assign(response.authorization_url)
    } catch (reason) { setError(managementError(reason)); setBusy("") }
  }
  async function unlink() {
    if (!unlinking || busy) return
    setBusy(unlinking.id); setError(""); setNotice("")
    try { await api(`/api/v1/account/identities/${unlinking.id}`, { method: "DELETE" }); setUnlinking(null); setNotice("第三方账号已解绑。"); await load(); updateUser(await api<AccountUser>("/api/v1/me")) }
    catch (reason) { setError(managementError(reason)); setUnlinking(null) } finally { setBusy("") }
  }

  return <Card>
    <CardHeader><CardTitle className="flex items-center gap-2"><Link2 size={18} />第三方账号</CardTitle><CardDescription>绑定后，可使用第三方账号登录同一个 AlbaTextus 账户。</CardDescription></CardHeader>
    <CardContent className="flex flex-col gap-4">
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button variant="link" disabled={Boolean(busy)} onClick={() => { setError(""); void load() }}>重新加载</Button></AlertDescription></Alert>}
      {notice && <Alert><Check /><AlertDescription role="status">{notice}</AlertDescription></Alert>}
      {loading ? <Skeleton className="h-36 w-full" /> : <div className="flex flex-col divide-y rounded-xl border">{providers.map(provider => {
        const identity = identities.find(item => item.provider === provider.id)
        return <div key={provider.id} className="flex flex-wrap items-center gap-3 p-4">
          {identity?.avatar_url ? <UserAvatar src={identity.avatar_url} name={identity.display_name || provider.name} /> : <ProviderIcon provider={provider.id} className="size-5" />}
          <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><p className="text-sm font-medium">{provider.name}</p><Badge variant={identity ? "success" : "secondary"}>{identity ? "已绑定" : provider.enabled ? "未绑定" : "暂未开放"}</Badge></div><p className="mt-1 break-all text-xs text-muted-foreground">{identity ? identity.display_name || identity.email || "已关联第三方账号" : provider.enabled ? "连接你的账号，下次登录更方便。" : "此登录方式尚未开放，开放后即可绑定。"}</p></div>
          <Button type="button" variant="outline" disabled={Boolean(busy) || loading || (!identity && !provider.enabled) || Boolean(identity && !hasPassword)} title={identity && !hasPassword ? "请先设置登录密码" : undefined} onClick={() => identity ? setUnlinking(provider) : void link(provider)}>{busy === provider.id ? <Spinner data-icon="inline-start" aria-hidden="true" /> : identity ? <Unlink data-icon="inline-start" /> : <Link2 data-icon="inline-start" />}{identity ? "解绑" : "绑定"}</Button>
        </div>
      })}</div>}
      {!hasPassword && identities.length > 0 && <p className="text-xs text-muted-foreground">请先设置登录密码，再解绑第三方账号，以便继续登录。</p>}
    </CardContent>
    <Dialog open={Boolean(unlinking)} onOpenChange={open => { if (!open && !busy) setUnlinking(null) }}><DialogContent className="center-dialog" showCloseButton={!busy}><DialogHeader><DialogTitle>解绑 {unlinking?.name}？</DialogTitle><DialogDescription>解绑后，该第三方账号无法再登录此账户。你仍可使用邮箱密码登录。</DialogDescription></DialogHeader><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setUnlinking(null)}>取消</Button><Button variant="destructive" disabled={Boolean(busy)} onClick={() => void unlink()}>{busy && <Spinner data-icon="inline-start" aria-hidden="true" />}确认解绑</Button></DialogFooter></DialogContent></Dialog>
  </Card>
}
