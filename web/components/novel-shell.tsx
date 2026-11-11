"use client"

import { useEffect, useState, type ReactNode } from "react"
import { Link } from "next-view-transitions"
import { usePathname } from "next/navigation"
import type { AccountUser } from "@/components/account-shell"
import { NovelUserMenu } from "@/components/novel-user-menu"
import { SiteHeader } from "@/components/site-header"
import { api } from "@/lib/api"
import { novelSections } from "@/lib/novel-sections"
import { hasOperation, type RolePolicy } from "@/lib/athub-permissions"

export function NovelShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const [user, setUser] = useState<AccountUser | null>(null)
  const [canReview, setCanReview] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    api<AccountUser>("/api/v1/user", { signal: controller.signal, cache: "no-store" })
      .then(async value => {
        if (controller.signal.aborted) return
        setUser(value)
        if (value.level >= 3) {
          const permissions = await api<RolePolicy>("/api/v1/account/athub/permissions", { signal: controller.signal })
          if (!controller.signal.aborted) setCanReview(hasOperation(permissions, "review"))
        }
      })
      .catch(() => { /* The public library also works without an account session. */ })
    return () => controller.abort()
  }, [])

  const activeSection = novelSections.find(section => section.href === pathname ||
    (section.id === "library" && pathname.startsWith("/library/books/")))

  return <div className="novel-workspace">
      <a className="novel-skip" href="#novel-content">跳转到主要内容</a>
      <SiteHeader items={[...novelSections]} activeHref={activeSection?.href} navigationLabel="小说库导航"
        routeKey={pathname} LinkComponent={Link}
        actions={<NovelUserMenu user={user} canReview={canReview} />} />
      <main id="novel-content" tabIndex={-1} className="novel-surface">{children}</main>
    </div>
}
