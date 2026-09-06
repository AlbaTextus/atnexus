"use client"

import { useEffect, useState } from "react"
import { startRegistration } from "@simplewebauthn/browser"
import { AlertCircle, Fingerprint, KeyRound, Plus, ShieldCheck, Trash2 } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { ChangePassword } from "@/components/change-password"
import { LinkedAccounts } from "@/components/linked-accounts"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"

type Passkey = { id: string; name: string; created_at: string; last_used_at?: string | null }
type PasskeyOptions = { ceremony_token: string; publicKey: Record<string, unknown> }

export function Account() {
  const [passkeys, setPasskeys] = useState<Passkey[]>([])
  const [name, setName] = useState("我的通行密钥")
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [deleting, setDeleting] = useState<Passkey | null>(null)
  const [deleteError, setDeleteError] = useState("")

  async function load() {
    setLoading(true)
    try { const response = await api<{ passkeys: Passkey[] }>("/api/v1/passkeys"); setPasskeys(response.passkeys) }
    catch (reason) { setError(managementError(reason)) } finally { setLoading(false) }
  }
  useEffect(() => { void load() }, [])

  async function addPasskey() {
    setBusy(true); setError("")
    try {
      const ceremony = await api<PasskeyOptions>("/api/v1/passkeys/registration/options", { method: "POST" })
      const credential = await startRegistration({ optionsJSON: ceremony.publicKey as never })
      await api<Passkey>("/api/v1/passkeys/registration/finish", { method: "POST", body: JSON.stringify(credential), headers: { "X-WebAuthn-Ceremony": ceremony.ceremony_token, "X-Passkey-Name": encodeURIComponent(name.trim() || "我的通行密钥") } })
      setName("我的通行密钥"); await load()
    } catch (reason) { setError(reason instanceof Error && reason.name === "NotAllowedError" ? "通行密钥设置已取消或超时，请重试。" : managementError(reason)) } finally { setBusy(false) }
  }

  async function removePasskey() {
    if (!deleting) return
    setBusy(true); setDeleteError("")
    try { await api(`/api/v1/passkeys/${deleting.id}`, { method: "DELETE" }); setDeleting(null); await load() }
    catch (reason) { setDeleteError(managementError(reason)) } finally { setBusy(false) }
  }

  return <div className="flex max-w-3xl flex-col gap-7">
    <div className="center-page-heading"><div><p className="center-eyebrow">YOUR ACCOUNT</p><h1>账户安全</h1><p>管理你的统一身份与登录方式，安全连接每个子系统。</p></div><Badge variant="success"><ShieldCheck data-icon="inline-start" />账户正常</Badge></div>
    <ChangePassword />
    <LinkedAccounts />
    <Card><CardHeader><CardTitle className="flex items-center gap-2"><Fingerprint size={18} />通行密钥</CardTitle><CardDescription>使用指纹、面容或设备 PIN 登录，无需输入密码。</CardDescription></CardHeader><CardContent className="flex flex-col gap-5">
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => { setError(""); void load() }}>重试</Button></AlertDescription></Alert>}
      <form onSubmit={event => { event.preventDefault(); void addPasskey() }} ><FieldGroup className="flex-row flex-wrap items-end gap-3"><Field className="min-w-0 flex-1" data-disabled={busy}><FieldLabel htmlFor="passkey-name">通行密钥名称</FieldLabel><Input id="passkey-name" value={name} onChange={event => setName(event.target.value)} maxLength={100} disabled={busy} className="h-10" /></Field><Button type="submit" disabled={busy} className="h-10">{busy ? <Spinner data-icon="inline-start" aria-hidden="true" /> : <Plus data-icon="inline-start" />}添加通行密钥</Button></FieldGroup></form>
      {loading ? <Skeleton className="h-32 w-full" /> : passkeys.length === 0 ? <Empty className="border"><EmptyHeader><EmptyMedia variant="icon"><KeyRound /></EmptyMedia><EmptyTitle>还没有通行密钥</EmptyTitle><EmptyDescription>添加后，可以在登录页面使用设备验证快速登录。</EmptyDescription></EmptyHeader></Empty> : <div className="flex flex-col divide-y rounded-xl border">{passkeys.map(passkey => <div key={passkey.id} className="flex items-center justify-between gap-4 p-4"><div className="min-w-0"><p className="break-words text-sm font-medium">{passkey.name}</p><p className="mt-1 text-xs text-muted-foreground">添加于 {new Date(passkey.created_at).toLocaleDateString("zh-CN")}{passkey.last_used_at ? ` · 最近使用 ${new Date(passkey.last_used_at).toLocaleDateString("zh-CN")}` : ""}</p></div><Button variant="ghost" size="icon" aria-label={`删除 ${passkey.name}`} disabled={busy} onClick={() => { setDeleting(passkey); setDeleteError("") }}><Trash2 /></Button></div>)}</div>}
    </CardContent></Card>
    <Dialog open={Boolean(deleting)} onOpenChange={open => { if (!open && !busy) setDeleting(null) }}><DialogContent showCloseButton={!busy} className="center-dialog"><DialogHeader><DialogTitle>删除通行密钥？</DialogTitle><DialogDescription>删除“{deleting?.name}”后，该密钥将无法登录。你仍可使用邮箱密码或其他通行密钥。</DialogDescription></DialogHeader>{deleteError && <Alert variant="destructive"><AlertDescription>{deleteError}</AlertDescription></Alert>}<DialogFooter><Button variant="outline" disabled={busy} onClick={() => setDeleting(null)}>取消</Button><Button variant="destructive" disabled={busy} onClick={removePasskey}>确认删除</Button></DialogFooter></DialogContent></Dialog>
  </div>
}
