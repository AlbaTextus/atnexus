"use client"

import { useFeedback, feedback } from "@/lib/feedback"

import { useEffect, useRef, useState, type FormEvent, type ChangeEvent } from "react"
import { AlertCircle, Check, Crop, Pencil, RefreshCw, Save, Upload, UserRound } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { type ExternalIdentity, type LoginProvider } from "@/lib/external-login"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { AvatarCropDialog } from "@/components/avatar-crop-dialog"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ProfileImageUpload } from "@/components/profile-image-upload"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

export function AvatarSettings({ onPreviewChange }: { onPreviewChange?: (url: string | undefined) => void }) {
  const user = useAccountUser()
  const updateUser = useUpdateAccountUser()
  const [identities, setIdentities] = useState<ExternalIdentity[]>([])
  const [providers, setProviders] = useState<LoginProvider[]>([])
  const [source, setSource] = useState(user.avatar_source || "auto")
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useFeedback("error")
  const [loadError, setLoadError] = useState("")
  const [saved, setSaved] = useState(false)
  const [retry, setRetry] = useState(0)
  const [file, setFile] = useState<File | null>(null)
  const [filePreview, setFilePreview] = useState("")
  const [fileInputKey, setFileInputKey] = useState(0)
  const [cropFile, setCropFile] = useState<File | null>(null)
  const [originalFile, setOriginalFile] = useState<File | null>(null)
  const [originalPreview, setOriginalPreview] = useState("")
  const uploadInput = useRef<HTMLInputElement>(null)
  const [editorOpen, setEditorOpen] = useState(false)
  function changeEditorOpen(open: boolean) {
    if (busy || cropFile) return
    setSource(user.avatar_source || "auto"); setFile(null); setOriginalFile(null)
    setFileInputKey(key => key + 1); setError(""); setSaved(false); setEditorOpen(open)
  }
  useEffect(() => {
    if (!file) { setFilePreview(""); return }
    const url = URL.createObjectURL(file)
    setFilePreview(url)
    return () => URL.revokeObjectURL(url)
  }, [file])
  useEffect(() => {
    if (!originalFile) { setOriginalPreview(""); return }
    const url = URL.createObjectURL(originalFile)
    setOriginalPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [originalFile])

  function selectFile(event: ChangeEvent<HTMLInputElement>) {
    const selected = event.target.files?.[0]
    setSaved(false); setError("")
    if (!selected) return
    if (!['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(selected.type) || selected.size === 0 || selected.size > 5 * 1024 * 1024) {
      event.target.value = ""; setError("请选择不超过 5 MB 的 PNG、JPEG、WebP 或 GIF 图片。"); return
    }
    setCropFile(selected); event.target.value = ""
  }
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
  const preview = source === "custom" ? filePreview || user.custom_avatar_url : source === "auto" ? available[0]?.avatar_url : source === "default" ? "" : identities.find(identity => identity.provider === source)?.avatar_url
  const sourceName = source === "custom" ? "自定义上传" : source === "auto" ? "自动同步" : source === "default" ? "默认头像" : platformName(source)
  const displayName = user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])
  const dirty = source !== (user.avatar_source || "auto") || (source === "custom" && Boolean(file))
  const missingCustom = source === "custom" && !file && !user.custom_avatar_url
  useEffect(() => {
    onPreviewChange?.(dirty ? preview || "" : undefined)
  }, [dirty, preview, onPreviewChange])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy || loading || loadError || !dirty || missingCustom) return
    setBusy(true); setError(""); setSaved(false)
    try {
      const form = new FormData()
      if (file) form.append("file", file)
      if (originalFile) form.append("original", originalFile)
      const updated = await api<AccountUser>("/api/v1/account/avatar", source === "custom" && file ? { method: "POST", body: form } : { method: "PATCH", body: JSON.stringify({ source }) })
      updateUser(updated); setSource(updated.avatar_source || "auto"); setFile(null); setOriginalFile(null); setFileInputKey(key => key + 1); setSaved(true); setEditorOpen(false); feedback.success("头像设置已保存")
    } catch (reason) { setError(reason instanceof ApiError && reason.code.startsWith("avatar_") ? reason.message : managementError(reason)) } finally { setBusy(false) }
  }

  return <Dialog open={editorOpen} onOpenChange={changeEditorOpen}>
    <section className="profile-image-section" aria-labelledby="profile-avatar-heading">
      <h2 id="profile-avatar-heading">头像</h2>
      <input ref={uploadInput} type="file" accept="image/png,image/jpeg,image/webp,image/gif" className="sr-only" aria-label="选择头像原图" disabled={busy} onChange={selectFile} />
      <ProfileImageUpload title="头像" src={originalPreview || user.avatar_original_url || user.avatar_url} disabled={busy} onClick={() => uploadInput.current?.click()} />
      <div className="profile-image-controls">
        {originalFile && <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => setCropFile(originalFile)}><Crop />裁剪</Button>}
        <DialogTrigger render={<Button type="button" variant="ghost" size="sm" disabled={busy} />}><Pencil />头像来源</DialogTrigger>
      </div>
      {!editorOpen && error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!editorOpen && dirty && <form onSubmit={submit} className="profile-image-controls"><Button type="submit" size="sm" disabled={busy || loading || Boolean(loadError) || missingCustom}>{busy ? <Spinner /> : <Save />}保存头像</Button><Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => changeEditorOpen(false)}>取消</Button></form>}
    </section>
    <DialogContent className="center-dialog profile-editor-page avatar-editor-dialog max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-lg" showCloseButton={!busy}>
      <DialogHeader><DialogTitle>编辑头像</DialogTitle><DialogDescription>选择头像来源，或上传并裁剪自己的图片。</DialogDescription></DialogHeader>
      <form onSubmit={submit} aria-busy={busy} className="flex flex-col gap-5">
      <div className="flex flex-col gap-5">
        <div className="profile-editor-avatar flex items-center gap-4">
          <UserAvatar src={loading || loadError ? user.avatar_url : preview} name={displayName} className="size-16" />
          <div className="flex min-w-0 flex-col gap-2"><p className="truncate font-medium">{displayName}</p><div><Badge variant="secondary">{sourceName}</Badge></div><p className="text-xs text-muted-foreground">{dirty ? "头像预览，保存后生效" : "在用户中心和已接入应用中使用"}</p></div>
        </div>
        {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
        {saved && <Alert><Check /><AlertDescription role="status">头像已更新。</AlertDescription></Alert>}
        {loadError ? <Alert variant="destructive"><AlertCircle /><AlertDescription>{loadError}<Button type="button" variant="link" onClick={() => setRetry(value => value + 1)}>重新加载头像</Button></AlertDescription></Alert> : loading ? <Skeleton className="h-24 w-full" /> : <FieldGroup>
          <Field data-disabled={busy}>
            <FieldLabel id="avatar-source-label">头像来源</FieldLabel>
            <ToggleGroup aria-labelledby="avatar-source-label" aria-describedby="avatar-source-help" variant="outline" value={[source]} onValueChange={values => { if (values.length) { setSource(values[0]); setError(""); setSaved(false) } }} disabled={busy} className="flex w-full flex-wrap" spacing={2}>
              <ToggleGroupItem value="auto" aria-label="自动同步" className="h-16 min-w-20 flex-col gap-1 px-3"><RefreshCw /><span>自动同步</span></ToggleGroupItem>
              <ToggleGroupItem value="default" aria-label="默认头像" className="h-16 min-w-20 flex-col gap-1 px-3"><UserRound /><span>默认头像</span></ToggleGroupItem>
              <ToggleGroupItem value="custom" aria-label="自定义上传" className="h-16 min-w-20 flex-col gap-1 px-3"><Upload /><span>自定义上传</span></ToggleGroupItem>
              {identities.map(identity => <ToggleGroupItem key={identity.provider} value={identity.provider} aria-label={`${platformName(identity.provider)} 头像`} disabled={!identity.avatar_url || busy} title={!identity.avatar_url ? "该平台尚未提供头像，可重新登录该平台后查看。" : platformName(identity.provider)} className="h-16 min-w-20 max-w-40 flex-col gap-1 px-3"><UserAvatar src={identity.avatar_url} name={identity.display_name} decorative /><span className="max-w-full truncate">{platformName(identity.provider)}{!identity.avatar_url && " · 暂无头像"}</span></ToggleGroupItem>)}
            </ToggleGroup>
            <FieldDescription id="avatar-source-help">{source === "custom" ? "上传你自己的头像，第三方登录不会覆盖它。" : source === "default" ? "使用昵称生成的默认头像，第三方登录不会覆盖你的选择。" : source === "auto" ? "自动使用最先绑定且提供了头像的平台。平台没有头像时显示默认头像。" : `使用 ${sourceName} 的头像，下次通过该平台登录时同步更新。`}</FieldDescription>
          </Field>
          {source === "custom" && <Field data-disabled={busy}><FieldLabel htmlFor="avatar-file">选择头像图片</FieldLabel><Input key={fileInputKey} id="avatar-file" type="file" accept="image/png,image/jpeg,image/webp,image/gif" disabled={busy} onChange={selectFile} aria-describedby="avatar-file-help" /><FieldDescription id="avatar-file-help">支持 PNG、JPEG、WebP、GIF，最大 5 MB。选图后可拖动、缩放裁剪，GIF 保留动画（最多 200 帧）。</FieldDescription>{originalFile && <div><Button type="button" variant="outline" disabled={busy} onClick={() => setCropFile(originalFile)}><Crop data-icon="inline-start" />重新裁剪</Button></div>}</Field>}
          {available.length === 0 && source !== "custom" && <FieldDescription>绑定第三方账号，或重新通过已绑定平台登录后，即可使用平台提供的头像。</FieldDescription>}
        </FieldGroup>}
      </div>
      <DialogFooter className="flex-wrap gap-3">
        <Button type="submit" disabled={busy || loading || Boolean(loadError) || !dirty || missingCustom}>{busy ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}{busy && source === "custom" && file ? "正在上传…" : "保存头像"}</Button>
        <Button type="button" variant="outline" disabled={busy} onClick={() => changeEditorOpen(false)}>取消</Button>
      </DialogFooter>
      </form>
    </DialogContent>
    {cropFile && <AvatarCropDialog file={cropFile} onCancel={() => setCropFile(null)} onConfirm={cropped => { setOriginalFile(cropFile); setFile(cropped); setSource("custom"); setSaved(false); setError(""); setCropFile(null) }} />}
  </Dialog>
}
