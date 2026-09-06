"use client"

import Link from "next/link"
import { useRouter } from "next/navigation"
import { useEffect, useState, type FormEvent } from "react"
import { AlertCircle, ArrowLeft, Check, Code2, LockKeyhole, Save, Settings2, Trash2 } from "lucide-react"
import { api } from "@/lib/api"
import { type Application, type OAuthMetadata, managementError } from "@/lib/applications"
import { CopyField } from "@/components/copy-field"
import { ClientSecretField } from "@/components/client-secret-field"
import { ApplicationPreview } from "@/components/application-preview"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

type Draft = { name: string; homepage_url: string; description: string; icon_url: string; minimum_role: string; callbacks: string; is_active: boolean; is_public: boolean }
const initial: Draft = { name: "", homepage_url: "", description: "", icon_url: "", minimum_role: "USER", callbacks: "", is_active: true, is_public: false }
const toDraft = (app: Application): Draft => ({ name: app.name, homepage_url: app.homepage_url, description: app.description, icon_url: app.icon_url, minimum_role: app.minimum_role, callbacks: app.redirect_uris.join("\n"), is_active: app.is_active, is_public: app.is_public })

export function ApplicationEditor({ id }: { id?: string }) {
  const router = useRouter()
  const [app, setApp] = useState<Application | null>(null)
  const [draft, setDraft] = useState<Draft>(initial)
  const [saved, setSaved] = useState<Draft>(initial)
  const [metadata, setMetadata] = useState<OAuthMetadata | null>(null)
  const [loading, setLoading] = useState(Boolean(id))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const [secret, setSecret] = useState("")
  const [secretOpen, setSecretOpen] = useState(false)
  const [secretRevision, setSecretRevision] = useState(0)
  const [confirm, setConfirm] = useState<"delete" | "rotate" | null>(null)
  const [confirmError, setConfirmError] = useState("")
  const [deleteName, setDeleteName] = useState("")
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)

  useEffect(() => {
    let active = true
    api<OAuthMetadata>("/.well-known/oauth-authorization-server").then(value => { if (active) setMetadata(value) }).catch(() => {})
    if (id) api<Application>(`/api/v1/applications/${encodeURIComponent(id)}`).then(value => { if (active) { setApp(value); setDraft(toDraft(value)); setSaved(toDraft(value)) } }).catch(reason => { if (active) setError(managementError(reason)) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [id])

  useEffect(() => {
    if (!dirty) return
    const handler = (event: BeforeUnloadEvent) => { event.preventDefault() }
    window.addEventListener("beforeunload", handler)
    return () => window.removeEventListener("beforeunload", handler)
  }, [dirty])

  function change<K extends keyof Draft>(key: K, value: Draft[K]) { setDraft(current => ({ ...current, [key]: value })); setNotice("") }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy) return
    setError(""); setNotice(""); setBusy(true)
    const { callbacks, ...fields } = draft
    const payload = { ...fields, redirect_uris: callbacks.split(/\r?\n/).map(value => value.trim()).filter(Boolean) }
    try {
      if (app) {
        const value = await api<Application>(`/api/v1/applications/${app.id}`, { method: "PUT", body: JSON.stringify(payload) })
        setApp(value); setDraft(toDraft(value)); setSaved(toDraft(value)); setNotice("应用配置已保存。")
      } else {
        const value = await api<{ application: Application; client_secret: string }>("/api/v1/applications", { method: "POST", body: JSON.stringify(payload) })
        setApp(value.application); setDraft(toDraft(value.application)); setSaved(toDraft(value.application))
        if (value.client_secret) { setSecret(value.client_secret); setSecretOpen(true) }
        else router.replace(`/admin/applications/${value.application.id}`)
      }
    } catch (reason) { setError(managementError(reason)) } finally { setBusy(false) }
  }

  async function confirmAction() {
    if (!app || busy) return
    setBusy(true); setConfirmError("")
    try {
      if (confirm === "delete") {
        await api(`/api/v1/applications/${app.id}`, { method: "DELETE" }); router.replace("/admin/applications")
      } else {
        const result = await api<{ client_secret: string }>(`/api/v1/applications/${app.id}/secret`, { method: "POST" })
        setSecret(result.client_secret); setSecretRevision(value => value + 1); setApp({ ...app, secret_available: true }); setSecretOpen(true)
      }
      setConfirm(null)
    } catch (reason) { setConfirmError(managementError(reason)) } finally { setBusy(false) }
  }

  function closeSecret() {
    setSecretOpen(false); setSecret("")
    if (!id && app) router.replace(`/admin/applications/${app.id}`)
  }

  if (loading) return <div className="flex flex-col gap-6"><Skeleton className="h-12 w-64" /><Skeleton className="h-72 w-full" /><Skeleton className="h-96 w-full" /></div>
  return <div className="flex flex-col gap-6">
    <div className="center-page-heading"><div><p className="center-eyebrow">APPLICATION SETTINGS</p><h1>{app ? app.name : "创建应用"}</h1><p>{app ? "管理接入凭据、应用信息和登录权限。" : "连接你的子系统，共用一个 AlbaTextus 账户。"}</p></div><Button variant="outline" nativeButton={false} role="link" render={<Link href="/admin/applications" />}><ArrowLeft data-icon="inline-start" />返回应用</Button></div>
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
    {notice && <Alert><Check /><AlertDescription role="status">{notice}</AlertDescription></Alert>}
    {id && !app ? <Button variant="outline" onClick={() => window.location.reload()}>重新加载</Button> : <>
      {app && <Card><CardHeader><CardTitle className="flex items-center gap-2"><LockKeyhole size={18} />接入信息</CardTitle><CardDescription>将这些信息配置到子系统的 OAuth 客户端中。</CardDescription></CardHeader><CardContent><FieldGroup className="gap-5">
        <CopyField label="Client ID" value={app.id} />
        {app.is_public ? <Field><FieldLabel>Client Secret</FieldLabel><FieldDescription>公开客户端使用 S256 PKCE，无需保存客户端密钥。</FieldDescription></Field> : <ClientSecretField key={`${app.id}:${secretRevision}`} applicationId={app.id} available={app.secret_available} busy={busy} onRotate={() => { setConfirm("rotate"); setConfirmError("") }} />}
        {metadata ? <><CopyField label="Authorization Endpoint" value={metadata.authorization_endpoint} /><CopyField label="Token Endpoint" value={metadata.token_endpoint} /><CopyField label="User Endpoint" value={metadata.userinfo_endpoint} /></> : <Alert><AlertCircle /><AlertDescription>接口地址暂时加载失败。<Button variant="link" onClick={() => { void api<OAuthMetadata>("/.well-known/oauth-authorization-server").then(setMetadata).catch(reason => setError(managementError(reason))) }}>重新加载</Button></AlertDescription></Alert>}<div className="flex flex-wrap gap-2">{app.allowed_scopes.map(scope => <Badge key={scope} variant="secondary">{scope}</Badge>)}<Badge variant="outline">Authorization Code + PKCE</Badge></div></FieldGroup></CardContent></Card>}
      <div className="application-editor-layout">
      <form onSubmit={submit} aria-busy={busy} className="min-w-0">
        <Card><CardHeader><CardTitle className="flex items-center gap-2"><Settings2 size={18} />应用信息</CardTitle><CardDescription>填写对用户展示的信息与授权回调地址。</CardDescription></CardHeader><CardContent><FieldGroup>
          <Field data-disabled={busy}><FieldLabel htmlFor="app-name">应用名称</FieldLabel><Input id="app-name" name="name" value={draft.name} onChange={e => change("name",e.target.value)} required maxLength={100} disabled={busy} placeholder="例如：AlbaTextus 文档" className="h-11" /></Field>
          <Field data-disabled={busy}><FieldLabel htmlFor="app-homepage">应用主页</FieldLabel><Input id="app-homepage" name="homepage_url" type="url" value={draft.homepage_url} onChange={e => change("homepage_url",e.target.value)} required maxLength={2048} disabled={busy} placeholder="https://your-app.example.com" className="h-11" /></Field>
          <Field data-disabled={busy}><FieldLabel htmlFor="app-description">应用描述 <span className="text-muted-foreground">（选填）</span></FieldLabel><Textarea id="app-description" name="description" value={draft.description} onChange={e => change("description",e.target.value)} maxLength={1000} disabled={busy} placeholder="简单介绍这个应用的用途" className="min-h-24" /></Field>
          <Field data-disabled={busy}><FieldLabel htmlFor="app-callbacks">回调地址</FieldLabel><Textarea id="app-callbacks" name="redirect_uris" value={draft.callbacks} onChange={e => change("callbacks",e.target.value)} required disabled={busy} placeholder="https://your-app.example.com/auth/callback" className="min-h-24" aria-describedby="callback-help" /><FieldDescription id="callback-help">每行一个地址，最多 10 个。需精确匹配，不支持通配符；本地开发可使用 localhost 的 HTTP 地址。</FieldDescription></Field>
          <Field data-disabled={busy}><FieldLabel htmlFor="app-icon">应用图标 <span className="text-muted-foreground">（选填）</span></FieldLabel><Input id="app-icon" name="icon_url" type="url" value={draft.icon_url} onChange={e => change("icon_url",e.target.value)} maxLength={2048} disabled={busy} placeholder="https://your-app.example.com/logo.png" className="h-11" /></Field>
          <Field data-disabled={busy}><FieldLabel id="app-type-label">应用类型</FieldLabel>{app ? <div><Badge variant="secondary">{app.is_public ? <Code2 data-icon="inline-start" /> : <LockKeyhole data-icon="inline-start" />}{app.is_public ? "公开客户端 · PKCE" : "服务端应用"}</Badge></div> : <ToggleGroup aria-labelledby="app-type-label" value={[draft.is_public ? "public" : "confidential"]} onValueChange={values => { if (values[0]) change("is_public",values[0] === "public") }} disabled={busy || Boolean(app)} variant="outline" spacing={0} className="max-w-full flex-wrap"><ToggleGroupItem value="confidential"><LockKeyhole />服务端应用</ToggleGroupItem><ToggleGroupItem value="public"><Code2 />公开客户端</ToggleGroupItem></ToggleGroup>}<FieldDescription>{draft.is_public ? "适用于纯前端或原生应用，必须使用 S256 PKCE。" : "适用于有后端的子系统，Client Secret 仅保存在服务端。"} 创建后不可更改。</FieldDescription></Field>
          <Field data-disabled={busy}><FieldLabel id="app-role-label">最低访问角色</FieldLabel><ToggleGroup aria-labelledby="app-role-label" value={[draft.minimum_role]} onValueChange={values => { if (values[0]) change("minimum_role",values[0]) }} disabled={busy} variant="outline" spacing={0} className="max-w-full flex-wrap"><ToggleGroupItem value="USER">普通用户</ToggleGroupItem><ToggleGroupItem value="ADMIN">管理员</ToggleGroupItem><ToggleGroupItem value="SUPER_ADMIN">超级管理员</ToggleGroupItem></ToggleGroup><FieldDescription>只有达到该角色的账户才能登录此应用。</FieldDescription></Field>
          <Field orientation="horizontal" data-disabled={busy}><FieldContent><FieldLabel htmlFor="app-active">启用应用</FieldLabel><FieldDescription>停用后将阻止登录，并撤销已有授权。</FieldDescription></FieldContent><Switch id="app-active" checked={draft.is_active} onCheckedChange={value => change("is_active",value)} disabled={busy} /></Field>
          {app && <p className="text-xs text-muted-foreground">修改回调地址、访问角色或启停状态会撤销已有授权，子系统需要重新登录。</p>}
        </FieldGroup></CardContent><CardFooter className="flex-wrap gap-3"><Button type="submit" size="lg" disabled={busy || Boolean(app && !dirty)}>{busy ? <Spinner data-icon="inline-start" aria-hidden="true" /> : <Save data-icon="inline-start" />}{app ? "保存修改" : "创建应用"}</Button><Button type="button" variant="outline" disabled={busy || !dirty} onClick={() => { setDraft(saved); setError(""); setNotice("") }}>取消修改</Button>{app && <Button type="button" variant="destructive" disabled={busy} className="ml-auto" onClick={() => { setConfirm("delete"); setConfirmError(""); setDeleteName("") }}><Trash2 data-icon="inline-start" />删除应用</Button>}</CardFooter></Card>
      </form>
      <ApplicationPreview draft={draft} />
      </div>
    </>}
    <Dialog open={secretOpen} onOpenChange={open => { if (!open) closeSecret() }}><DialogContent className="center-dialog max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg"><DialogHeader><DialogTitle>Client Secret 已生成</DialogTitle><DialogDescription>管理员可随时在接入信息中查看或复制密钥。请将它配置到子系统服务端。</DialogDescription></DialogHeader><FieldGroup>{app && <CopyField label="Client ID" value={app.id} />}<CopyField key={secretRevision} label="Client Secret" value={secret} secret /></FieldGroup><DialogFooter><Button onClick={closeSecret}>完成</Button></DialogFooter></DialogContent></Dialog>
    <Dialog open={confirm !== null} onOpenChange={open => { if (!open && !busy) setConfirm(null) }}><DialogContent showCloseButton={!busy} className="center-dialog max-h-[calc(100dvh-2rem)] overflow-y-auto"><DialogHeader><DialogTitle>{confirm === "delete" ? "删除这个应用？" : "重置客户端密钥？"}</DialogTitle><DialogDescription>{confirm === "delete" ? "应用配置及其授权将被永久删除。请输入应用名称确认。" : "旧密钥和已有授权会立即失效，请在重置后更新子系统配置。"}</DialogDescription></DialogHeader>{confirmError && <Alert variant="destructive"><AlertDescription>{confirmError}</AlertDescription></Alert>}{confirm === "delete" && <Field><FieldLabel htmlFor="delete-app-name">应用名称：{app?.name}</FieldLabel><Input id="delete-app-name" value={deleteName} onChange={event => setDeleteName(event.target.value)} disabled={busy} /></Field>}<DialogFooter><Button variant="outline" disabled={busy} onClick={() => setConfirm(null)}>取消</Button><Button variant="destructive" disabled={busy || (confirm === "delete" && deleteName !== app?.name)} onClick={confirmAction}>{busy && <Spinner data-icon="inline-start" aria-hidden="true" />}{confirm === "delete" ? "确认删除" : "确认重置"}</Button></DialogFooter></DialogContent></Dialog>
  </div>
}
