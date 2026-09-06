"use client"

import Link from "next/link"
import { useEffect, useState, type FormEvent } from "react"
import { AlertCircle, Check, Link2, RefreshCw, Save, UserRound } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { type ExternalIdentity, type LoginProvider } from "@/lib/external-login"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

export function AvatarSettings() {
  const user = useAccountUser()
  const updateUser = useUpdateAccountUser()
  const [identities, setIdentities] = useState<ExternalIdentity[]>([])
  const [providers, setProviders] = useState<LoginProvider[]>([])
  const [source, setSource] = useState(user.avatar_source || "auto")
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [loadError, setLoadError] = useState("")
  const [saved, setSaved] = useState(false)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setLoadError("")
    Promise.all([
      api<{ identities: ExternalIdentity[] }>("/api/v1/account/identities", { signal: controller.signal }),
      api<{ providers: LoginProvider[] }>("/api/v1/auth/providers", { signal: controller.signal }),
    ]).then(([accounts, platforms]) => { setIdentities(accounts.identities); setProviders(platforms.providers) })
      .catch(reason => { if (!controller.signal.aborted) setLoadError(managementError(reason)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [retry])

  const available = identities.filter(identity => identity.avatar_url)
  const platformName = (id: string) => providers.find(provider => provider.id === id)?.name || id
  const preview = source === "auto" ? available[0]?.avatar_url : source === "default" ? "" : identities.find(identity => identity.provider === source)?.avatar_url
  const sourceName = source === "auto" ? "自动同步" : source === "default" ? "默认头像" : platformName(source)
  const displayName = user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])
  const dirty = source !== (user.avatar_source || "auto")

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy || loading || loadError || !dirty) return
    setBusy(true); setError(""); setSaved(false)
    try {
      const updated = await api<AccountUser>("/api/v1/account/avatar", { method: "PATCH", body: JSON.stringify({ source }) })
      updateUser(updated); setSource(updated.avatar_source || "auto"); setSaved(true)
    } catch (reason) { setError(managementError(reason)) } finally { setBusy(false) }
  }

  return <form onSubmit={submit} aria-busy={busy}>
    <Card>
      <CardHeader><CardTitle>我的头像</CardTitle><CardDescription>使用你熟悉的头像，让每个应用都能认出你。</CardDescription></CardHeader>
      <CardContent className="flex flex-col gap-5">
        <div className="flex items-center gap-4">
          <UserAvatar src={loading || loadError ? user.avatar_url : preview} name={displayName} className="size-16" />
          <div className="flex min-w-0 flex-col gap-2"><p className="truncate font-medium">{displayName}</p><div><Badge variant="secondary">{sourceName}</Badge></div><p className="text-xs text-muted-foreground">{dirty ? "头像预览，保存后生效" : "在用户中心和已接入应用中使用"}</p></div>
        </div>
        {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
        {saved && <Alert><Check /><AlertDescription role="status">头像已更新。</AlertDescription></Alert>}
        {loadError ? <Alert variant="destructive"><AlertCircle /><AlertDescription>{loadError}<Button type="button" variant="link" onClick={() => setRetry(value => value + 1)}>重新加载头像</Button></AlertDescription></Alert> : loading ? <Skeleton className="h-24 w-full" /> : <FieldGroup>
          <Field data-disabled={busy}>
            <FieldLabel id="avatar-source-label">头像来源</FieldLabel>
            <ToggleGroup aria-labelledby="avatar-source-label" aria-describedby="avatar-source-help" variant="outline" value={[source]} onValueChange={values => { if (values.length) { setSource(values[0]); setError(""); setSaved(false) } }} disabled={busy} className="grid w-full grid-cols-2 sm:flex sm:flex-wrap" spacing={3}>
              <ToggleGroupItem value="auto" aria-label="自动同步" className="h-24 min-w-24 flex-col gap-2 px-4"><RefreshCw /><span>自动同步</span></ToggleGroupItem>
              <ToggleGroupItem value="default" aria-label="默认头像" className="h-24 min-w-24 flex-col gap-2 px-4"><UserRound /><span>默认头像</span></ToggleGroupItem>
              {identities.map(identity => <ToggleGroupItem key={identity.provider} value={identity.provider} aria-label={`${platformName(identity.provider)} 头像`} disabled={!identity.avatar_url || busy} title={!identity.avatar_url ? "该平台尚未提供头像，可重新登录该平台后查看。" : platformName(identity.provider)} className="h-24 min-w-0 flex-col gap-2 px-4 sm:min-w-24 sm:max-w-48"><UserAvatar src={identity.avatar_url} name={identity.display_name} decorative /><span className="max-w-full truncate">{platformName(identity.provider)}{!identity.avatar_url && " · 暂无头像"}</span></ToggleGroupItem>)}
            </ToggleGroup>
            <FieldDescription id="avatar-source-help">{source === "default" ? "使用昵称生成的默认头像，第三方登录不会覆盖你的选择。" : source === "auto" ? "自动使用最先绑定且提供了头像的平台。平台没有头像时显示默认头像。" : `使用 ${sourceName} 的头像，下次通过该平台登录时同步更新。`}</FieldDescription>
          </Field>
          {available.length === 0 && <FieldDescription>绑定第三方账号，或重新通过已绑定平台登录后，即可使用平台提供的头像。</FieldDescription>}
        </FieldGroup>}
      </CardContent>
      <CardFooter className="flex-wrap gap-3">
        <Button type="submit" disabled={busy || loading || Boolean(loadError) || !dirty}>{busy ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}保存头像</Button>
        {dirty && <Button type="button" variant="outline" disabled={busy} onClick={() => { setSource(user.avatar_source || "auto"); setError(""); setSaved(false) }}>取消修改</Button>}
        <Button type="button" variant="ghost" nativeButton={false} role="link" render={<Link href="/account/security" />}><Link2 data-icon="inline-start" />管理第三方账号</Button>
      </CardFooter>
    </Card>
  </form>
}
