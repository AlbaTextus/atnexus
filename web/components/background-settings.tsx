"use client"

import { useEffect, useRef, useState, type ChangeEvent } from "react"
import { Crop, Trash2, Save } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { AvatarCropDialog } from "@/components/avatar-crop-dialog"
import { ProfileImageUpload } from "@/components/profile-image-upload"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"

export function BackgroundSettings({ onPreviewChange }: { onPreviewChange?: (url: string | undefined) => void }) {
  const user = useAccountUser()
  const updateUser = useUpdateAccountUser()
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [original, setOriginal] = useState<File | null>(null)
  const [cropFile, setCropFile] = useState<File | null>(null)
  const [originalPreview, setOriginalPreview] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  useEffect(() => {
    if (!original) { setOriginalPreview(""); return }
    const url = URL.createObjectURL(original)
    setOriginalPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [original])
  useEffect(() => {
    if (!file) { onPreviewChange?.(undefined); return }
    const url = URL.createObjectURL(file)
    onPreviewChange?.(url)
    return () => URL.revokeObjectURL(url)
  }, [file, onPreviewChange])
  function selectFile(event: ChangeEvent<HTMLInputElement>) {
    const selected = event.target.files?.[0]
    event.target.value = ""
    setError("")
    if (!selected) return
    if (!["image/png", "image/jpeg", "image/webp"].includes(selected.type) || selected.size === 0 || selected.size > 5 * 1024 * 1024) {
      setError("请选择不超过 5 MB 的 PNG、JPEG 或 WebP 图片。")
      return
    }
    setCropFile(selected)
  }
  async function save(remove = false) {
    if (busy || !remove && !file) return
    setBusy(true); setError("")
    try {
      const body = new FormData()
      if (file) body.append("file", file)
      if (original) body.append("original", original)
      const updated = await api<AccountUser>("/api/v1/account/background", remove ? { method: "DELETE" } : { method: "POST", body })
      updateUser(updated); setFile(null); setOriginal(null)
      feedback.success(remove ? "背投已移除" : "背投已保存")
    } catch (reason) { setError(reason instanceof Error ? reason.message : "背投保存失败，请重试。") }
    finally { setBusy(false) }
  }
  return <section className="profile-image-section" data-profile-source="background" aria-labelledby="profile-background-heading">
    <h2 id="profile-background-heading">背投</h2>
    <input ref={input} type="file" accept="image/png,image/jpeg,image/webp" disabled={busy} onChange={selectFile} className="sr-only" aria-label="选择背投原图" />
    <ProfileImageUpload title="背投" src={originalPreview || user.background_original_url || user.background_url} disabled={busy} onClick={() => input.current?.click()} />
    <div className="profile-image-controls">
      {original && <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => setCropFile(original)}><Crop />裁剪</Button>}
      {user.background_url && <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => save(true)}>{busy ? <Spinner /> : <Trash2 />}移除背投</Button>}
    </div>
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {file && <div className="profile-image-controls"><Button type="button" size="sm" disabled={busy} onClick={() => save()}>{busy ? <Spinner /> : <Save />}保存背投</Button><Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => { setFile(null); setOriginal(null); setError("") }}>取消</Button></div>}
    {cropFile && <AvatarCropDialog target="background" file={cropFile} onCancel={() => setCropFile(null)} onConfirm={cropped => { setOriginal(cropFile); setFile(cropped); setCropFile(null) }} />}
  </section>
}
