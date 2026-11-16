"use client"

import { useState, type FormEvent } from "react"
import { Pencil, Save } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import { isAdminRole, managementError } from "@/lib/applications"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { AvatarSettings } from "@/components/avatar-settings"
import { BioEditor } from "@/components/bio-editor"
import { BackgroundSettings } from "@/components/background-settings"
import { UserProfilePreview } from "@/components/user-profile-card"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

export function AccountProfile() {
  const user = useAccountUser()
  const updateUser = useUpdateAccountUser()
  const [avatarPreview, setAvatarPreview] = useState<string>()
  const [backgroundPreview, setBackgroundPreview] = useState<string>()
  const [name, setName] = useState(user.display_name || "")
  const [nameOpen, setNameOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const dirty = name.trim() !== (user.display_name || "")
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy || !isAdminRole(user.role) || !dirty || !name.trim()) return
    setBusy(true); setError("")
    try {
      const value = await api<AccountUser>("/api/v1/account/profile", { method: "PATCH", body: JSON.stringify({ display_name: name.trim() }) })
      updateUser(value); setName(value.display_name || ""); setNameOpen(false); feedback.success("个人资料已保存")
    } catch (reason) { setError(managementError(reason)) } finally { setBusy(false) }
  }
  const nameAction = isAdminRole(user.role) && <Dialog open={nameOpen} onOpenChange={open => { if (!busy) { setNameOpen(open); setName(user.display_name || ""); setError("") } }}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon-sm" aria-label="编辑昵称" />}><Pencil className="size-3.5" /></DialogTrigger>
    <DialogContent className="center-dialog sm:max-w-sm" showCloseButton={!busy}>
      <DialogHeader><DialogTitle>编辑昵称</DialogTitle></DialogHeader>
      <form onSubmit={submit} className="flex flex-col gap-5" aria-busy={busy}>
        <Field><FieldLabel htmlFor="profile-name">昵称</FieldLabel><Input id="profile-name" value={name} onChange={event => setName(event.target.value)} required maxLength={100} disabled={busy} autoComplete="nickname" /></Field>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={() => setNameOpen(false)}>取消</Button><Button type="submit" disabled={busy || !dirty || !name.trim()}>{busy ? <Spinner /> : <Save />}保存昵称</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
  return <div className="profile-editor-page profile-editor-layout mx-auto flex w-full flex-col gap-10">
    <h1 className="sr-only">编辑个人资料</h1>
    <div className="profile-media-layout">
      <AvatarSettings onPreviewChange={setAvatarPreview} />
      <div className="profile-card-preview text-left"><UserProfilePreview profile={{ ...user, display_name: user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0]), avatar_url: avatarPreview ?? user.avatar_url ?? "", background_url: user.background_url || "", bio: user.bio || "", created_at: user.created_at || "" }} previewURL={backgroundPreview} nameAction={nameAction} /></div>
      <BackgroundSettings onPreviewChange={setBackgroundPreview} />
    </div>
    <div className="profile-bio-section"><BioEditor /></div>
  </div>
}
