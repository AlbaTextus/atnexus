"use client"

import { useEffect, useRef, useState, type ReactNode } from "react"
import { Link } from "next-view-transitions"
import { usePathname } from "next/navigation"
import type { AccountUser } from "@/components/account/account-shell"
import { NovelUserMenu } from "@/components/novel/novel-user-menu"
import { SiteHeader } from "@/components/layout/site-header"
import { api } from "@/lib/api"
import { novelSections } from "@/lib/novel/novel-sections"
import { useNovelScrollReveal } from "@/components/novel/novel-motion"

export function NovelShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const root = useRef<HTMLDivElement>(null)
  useNovelScrollReveal(root, pathname)
  const [user, setUser] = useState<AccountUser | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    api<AccountUser>("/api/v1/user", { signal: controller.signal, cache: "no-store" })
      .then(value => {
        if (controller.signal.aborted) return
        setUser(value)
      })
      .catch(() => { /* The public library also works without an account session. */ })
    return () => controller.abort()
  }, [])

  const activeSection = novelSections.find(section => section.href === pathname ||
    (section.id === "library" && pathname.startsWith("/library/books/")))

  return <div ref={root} className="novel-workspace">
      <a className="novel-skip" href="#novel-content">跳转到主要内容</a>
      <SiteHeader items={[...novelSections]} activeHref={activeSection?.href} navigationLabel="小说库导航"
        routeKey={pathname} LinkComponent={Link}
        actions={<NovelUserMenu user={user} />} />
      <main id="novel-content" tabIndex={-1} className="novel-surface">{children}</main>
    </div>
}
