"use client"

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"

export function UserAvatar({ src, name = "用户", className, decorative = false }: {
  src?: string | null
  name?: string | null
  className?: string
  decorative?: boolean
}) {
  const label = name?.trim() || "用户"
  const initials = Array.from(label).slice(0, 2).join("").toUpperCase()
  return <Avatar className={className} role={decorative ? undefined : "img"} aria-label={decorative ? undefined : `${label}的头像`} aria-hidden={decorative || undefined}>
    {src && <AvatarImage src={src} alt="" referrerPolicy="no-referrer" />}
    <AvatarFallback>{initials}</AvatarFallback>
  </Avatar>
}
