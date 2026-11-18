"use client"

import { useEffect, useRef, useState, type ReactNode } from "react"
import { usePathname } from "next/navigation"
import { Link } from "next-view-transitions"
import { ChevronDown, ClipboardCheck, FileClock, FilePlus2, LibraryBig, PanelLeftClose, PanelLeftOpen, Search, SquarePen } from "lucide-react"
import type { AccountUser } from "@/components/account/account-shell"
import { NovelUserMenu } from "@/components/novel/novel-user-menu"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useNovelScrollReveal } from "@/components/novel/novel-motion"
import { api } from "@/lib/api"
import { hasOperation, type RolePolicy } from "@/lib/athub/athub-permissions"

const sections = [
  { href: "/creatplat/upload", title: "提交小说", icon: FilePlus2 },
  { href: "/creatplat/submissions", title: "我的提交", icon: FileClock },
  { href: "/creatplat/reviews", title: "审核中心", icon: ClipboardCheck },
] as const

export function CreatplatShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const root = useRef<HTMLDivElement>(null)
  useNovelScrollReveal(root, pathname)
  const [collapsed, setCollapsed] = useState(false)
  const [groupOpen, setGroupOpen] = useState(true)
  const [query, setQuery] = useState("")
  const [user, setUser] = useState<AccountUser | null>(null)
  const [canReview, setCanReview] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    api<AccountUser>("/api/v1/user", { signal: controller.signal, cache: "no-store" })
      .then(async value => {
        if (controller.signal.aborted) return
        setUser(value)
        if (value.level >= 3) {
          const policy = await api<RolePolicy>("/api/v1/account/athub/permissions", { signal: controller.signal, cache: "no-store" })
          if (!controller.signal.aborted) setCanReview(hasOperation(policy, "review"))
        }
      })
      .catch(() => { /* Each page handles its own authentication and permission errors. */ })
    return () => controller.abort()
  }, [])

  const visibleSections = sections.filter(section =>
    (section.href !== "/creatplat/reviews" || canReview || pathname === section.href) &&
    section.title.includes(query.trim()))

  return <div ref={root} className="creatplat-workspace" data-collapsed={collapsed}>
    <a className="novel-skip" href="#creatplat-content">跳转到主要内容</a>
    <aside className="creatplat-sidebar" aria-label="创作者平台侧栏">
      <Link href="/creatplat" className="creatplat-brand" aria-label="创作者平台" title={collapsed ? "创作者平台" : undefined}>
        <SquarePen aria-hidden="true" /><span className="creatplat-label">创作者平台</span>
      </Link>
      <div className="creatplat-sidebar-body">
        {!collapsed && <div className="creatplat-search">
          <Search aria-hidden="true" />
          <Input type="search" aria-label="搜索导航" placeholder="搜索导航…" value={query} onChange={event => setQuery(event.target.value)} />
        </div>}
        <nav aria-label="创作者平台导航">
          {!collapsed && <button type="button" className="creatplat-group-toggle" aria-expanded={groupOpen || Boolean(query.trim())} aria-controls="creatplat-navigation" onClick={() => setGroupOpen(value => !value)}>
            内容管理<ChevronDown aria-hidden="true" data-open={groupOpen || Boolean(query.trim())} />
          </button>}
          <div id="creatplat-navigation" hidden={!collapsed && !groupOpen && !query.trim()} className="creatplat-navigation">
            {visibleSections.map(({ href, title, icon: Icon }) => <Link key={href} href={href} className="creatplat-nav-item"
              aria-current={pathname === href ? "page" : undefined} aria-label={title} title={collapsed ? title : undefined}>
              <Icon aria-hidden="true" /><span className="creatplat-label">{title}</span>
            </Link>)}
            {!visibleSections.length && <p className="creatplat-search-empty" role="status">没有匹配的导航</p>}
          </div>
        </nav>
      </div>
      <div className="creatplat-sidebar-footer">
        <Link href="/library" className="creatplat-nav-item" aria-label="返回书库" title={collapsed ? "返回书库" : undefined}>
          <LibraryBig aria-hidden="true" /><span className="creatplat-label">返回书库</span>
        </Link>
        <div className="creatplat-sidebar-controls">
          <Button variant="ghost" size="icon" aria-label={collapsed ? "展开侧栏" : "收起侧栏"} aria-expanded={!collapsed} aria-controls="creatplat-navigation"
            onClick={() => { setCollapsed(value => !value); setQuery("") }}>
            {collapsed ? <PanelLeftOpen aria-hidden="true" /> : <PanelLeftClose aria-hidden="true" />}
          </Button>
          <NovelUserMenu user={user} />
        </div>
      </div>
    </aside>
    <main id="creatplat-content" tabIndex={-1} className="novel-surface creatplat-content">{children}</main>
  </div>
}
