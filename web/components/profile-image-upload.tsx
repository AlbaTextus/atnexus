"use client"

import { ImagePlus } from "lucide-react"

export function ProfileImageUpload({ title, src, disabled, onClick }: {
  title: string; src?: string; disabled?: boolean; onClick: () => void
}) {
  return <button type="button" className="profile-image-upload" aria-label={`上传${title}`} disabled={disabled} onClick={onClick}>
    {src ? <img src={src} alt={`${title}原图`} referrerPolicy="no-referrer" /> : <ImagePlus className="size-7 text-muted-foreground" aria-hidden="true" />}
    <span className="profile-image-upload-action"><ImagePlus className="size-4" aria-hidden="true" />{src ? `更换${title}` : `上传${title}`}</span>
  </button>
}
