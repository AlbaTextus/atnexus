"use client"

import { useEffect, useState, type ReactNode } from "react"
import { HoverCard } from "radix-ui"
import { CalendarDays, ShieldCheck } from "lucide-react"
import { api } from "@/lib/api"
import { safeContentURL } from "@/lib/athub-client"
import { UserAvatar } from "@/components/user-avatar"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "@/components/ui/button"

type PublicProfile = {
  id: string; numeric_id: number; display_name?: string | null; avatar_url: string;
  background_url: string; bio: string; level: number; created_at: string
}

export function UserProfilePreview({ profile, previewURL, nameAction }: { profile: PublicProfile; previewURL?: string; nameAction?: ReactNode }) {
  const name = profile.display_name || "用户"
  const background = previewURL || safeContentURL(profile.background_url)
  const joinedAt = new Date(profile.created_at)
  return <article className="w-full overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground">
    <div className="aspect-[3/1] bg-muted">{background && <img src={background} alt="" className="size-full object-cover" />}</div>
    <div className="px-5 pb-5">
      <UserAvatar src={profile.avatar_url} name={name} className="relative -mt-9 size-18 ring-4 ring-popover" />
      <div className="mt-4 flex min-w-0 items-center gap-2"><h3 className="min-w-0 break-words text-xl font-semibold">{name}</h3>{nameAction}</div>
      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5"><ShieldCheck className="size-3.5" aria-hidden="true" />LT{profile.level}</span>
        {!Number.isNaN(joinedAt.getTime()) && <span className="inline-flex items-center gap-1.5"><CalendarDays className="size-3.5" aria-hidden="true" /><time dateTime={profile.created_at}>{joinedAt.toLocaleDateString("zh-CN")} 加入</time></span>}
      </div>
    </div>
  </article>
}

export function UserProfileCard({ userId, name, children }: { userId: string; name: string; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const [profile, setProfile] = useState<PublicProfile | null>(null)
  const [error, setError] = useState("")
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    setProfile(null); setError("")
    api<PublicProfile>(`/api/v1/profiles/${encodeURIComponent(userId)}`, { signal: controller.signal, cache: "no-store" })
      .then(value => { if (!controller.signal.aborted) setProfile(value) })
      .catch(() => { if (!controller.signal.aborted) setError("暂时无法加载用户资料。") })
    return () => controller.abort()
  }, [open, userId, attempt])
  return <HoverCard.Root open={open} onOpenChange={setOpen} openDelay={250} closeDelay={180}>
    <HoverCard.Trigger asChild><button type="button" aria-label={`查看${name}的资料`} className="inline-flex shrink-0 cursor-pointer rounded-full border-0 bg-transparent p-0 outline-offset-4 focus-visible:outline-2 focus-visible:outline-ring" onFocus={() => setOpen(true)} onBlur={event => { if (!event.relatedTarget || !event.relatedTarget.closest?.('[data-user-profile-card]')) setOpen(false) }} onClick={() => setOpen(true)}>{children}</button></HoverCard.Trigger>
    <HoverCard.Portal>
      <div className="pointer-events-none fixed inset-0 z-50" style={{ zoom: "calc(1 / var(--site-root-scale, var(--site-scale, 1)))" }}>
      <HoverCard.Content data-user-profile-card side="top" align="start" sideOffset={12} collisionPadding={16} className="pointer-events-auto z-50 w-84 rounded-xl font-sans shadow-xl outline-none" style={{ zoom: "var(--site-scale, 1)" }}>
        {profile ? <UserProfilePreview profile={profile} /> : <div className="overflow-hidden rounded-xl border bg-popover p-5 text-sm text-popover-foreground" aria-live="polite">{error ? <><p>{error}</p><Button variant="link" size="sm" onClick={() => setAttempt(value => value + 1)}>重试</Button></> : <div role="status" aria-label="正在加载用户资料" className="flex flex-col gap-3"><Skeleton className="h-24 w-full" /><Skeleton className="size-14 rounded-full" /><Skeleton className="h-5 w-32" /></div>}</div>}
      </HoverCard.Content>
      </div>
    </HoverCard.Portal>
  </HoverCard.Root>
}
