"use client"

import { useEffect, useState, type ReactNode } from "react"
import { Link } from "next-view-transitions"
import { usePathname } from "next/navigation"
import { UserRound } from "lucide-react"
import type { AccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { ThemeToggle } from "@/components/theme-toggle"
import { IconButton } from "@/components/icon-button"
import { MaintenanceControl } from "@/components/maintenance-control"
import { SiteHeader } from "@/components/site-header"
import { api } from "@/lib/api"
import { novelSections } from "@/lib/novel-sections"

export function NovelShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const [user, setUser] = useState<AccountUser | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    api<AccountUser>("/api/v1/user", { signal: controller.signal, cache: "no-store" })
      .then(value => { if (!controller.signal.aborted) setUser(value) })
      .catch(() => { /* The public library also works without an account session. */ })
    return () => controller.abort()
  }, [])

  const accountHref = user ? `/account/${user.numeric_id}` : "/account"
  const activeSection = novelSections.find(section => section.href === pathname ||
    (section.id === "library" && pathname.startsWith("/library/books/")))

  return <div className="novel-workspace">
      <a className="novel-skip" href="#novel-content">跳转到主要内容</a>
      <SiteHeader items={[...novelSections]} activeHref={activeSection?.href} navigationLabel="小说库导航"
        routeKey={pathname} LinkComponent={Link}
        actions={<>
          <ThemeToggle />
          {user?.role === "SUPER_ADMIN" && <MaintenanceControl />}
          <IconButton nativeButton={false} role="link" render={<Link href={accountHref} />} label="进入用户中心">
            {user ? <UserAvatar src={user.avatar_url} name={user.display_name || user.email} decorative />
              : <UserRound aria-hidden="true" />}
          </IconButton>
        </>} />
      <main id="novel-content" tabIndex={-1} className="novel-surface">{children}</main>
    </div>
}
