"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { createContext, useContext, useEffect, useState, type ReactNode } from "react"
import { AlertCircle, ArrowLeft, BookOpen, ChevronRight, FileText, House, LayoutGrid, LogOut, Settings2, ShieldCheck, ShieldX, UserRound } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { isAdminRole, managementError, roleNames } from "@/lib/applications"
import { cn } from "@/lib/utils"
import { UserAvatar } from "@/components/user-avatar"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"

export type AccountUser = { id: string; email: string; display_name?: string | null; role: string; created_at?: string; has_password?: boolean; email_is_alias?: boolean; avatar_source?: string; avatar_url?: string }
const AccountContext = createContext<{ user: AccountUser; updateUser: (user: AccountUser) => void } | null>(null)
function useAccountContext() {
  const context = useContext(AccountContext)
  if (!context) throw new Error("Account provider is required")
  return context
}
export function useAccountUser() { return useAccountContext().user }
export function useUpdateAccountUser() { return useAccountContext().updateUser }

const userNavigation = [
  { href: "/account", title: "用户中心", icon: House },
  { href: "/account/profile", title: "个人资料", icon: UserRound },
  { href: "/account/security", title: "账户安全", icon: ShieldCheck },
]
const adminNavigation = [
  { href: "/admin/applications", title: "应用接入", icon: LayoutGrid },
  { href: "/admin/integration", title: "接入指南", icon: BookOpen },
  { href: "/admin/legal", title: "协议管理", icon: FileText },
]

export function AccountShell({ children, area = "account" }: { children: ReactNode; area?: "account" | "admin" }) {
  const pathname = usePathname()
  const [user, setUser] = useState<AccountUser | null>(null)
  const [error, setError] = useState("")
  const [leaving, setLeaving] = useState(false)
  const admin = Boolean(user && isAdminRole(user.role))
  const denied = area === "admin" && Boolean(user) && !admin
  const navigation = area === "admin" && admin ? adminNavigation : userNavigation
  const active = navigation.find(item => item.href === "/account" ? pathname === item.href : pathname.startsWith(item.href))
  const title = area === "admin" && admin ? "管理后台" : "用户中心"

  useEffect(() => {
    let cancelled = false
    api<AccountUser>("/api/v1/me").then(value => { if (!cancelled) setUser(value) }).catch(reason => {
      if (cancelled) return
      if (reason instanceof ApiError && reason.status === 401) {
        window.location.href = `/login?return_to=${encodeURIComponent(window.location.pathname)}`
      } else setError(managementError(reason))
    })
    return () => { cancelled = true }
  }, [])

  async function logout() {
    setLeaving(true)
    try { await api("/api/v1/sessions/current", { method: "DELETE" }); window.location.href = "/login" }
    catch (reason) { setError(managementError(reason)); setLeaving(false) }
  }

  return (
    <div className="center-shell">
      <a className="center-skip" href="#center-content">跳转到主要内容</a>
      <aside className="center-sidebar">
        <Link href={area === "admin" && admin ? "/admin/applications" : "/account"} className="center-brand"><span className="center-brand-mark">A</span><span>AlbaTextus<small>{title}</small></span></Link>
        <p className="center-nav-label">{area === "admin" && admin ? "系统管理" : "我的账户"}</p>
        <nav aria-label={title + "导航"} className="center-nav">
          {navigation.map(item => <Link key={item.href} href={item.href} className={cn("center-nav-link", active?.href === item.href && "is-active")} aria-current={active?.href === item.href ? "page" : undefined}><item.icon size={18} /><span>{item.title}</span>{active?.href === item.href && <ChevronRight size={15} />}</Link>)}
        </nav>
        {admin && <div className="center-area-switch"><Button variant="outline" nativeButton={false} role="link" render={<Link href={area === "admin" ? "/account" : "/admin/applications"} />}>{area === "admin" ? <ArrowLeft data-icon="inline-start" /> : <Settings2 data-icon="inline-start" />}{area === "admin" ? "返回用户中心" : "管理后台"}</Button></div>}
        <div className="center-sidebar-bottom"><ShieldCheck size={18} /><span>一个账户，连接每个应用<small>AlbaTextus Account</small></span></div>
      </aside>
      <div className="center-workspace">
        <header className="center-topbar"><div className="center-breadcrumb">{title}<ChevronRight size={14} /><span>{denied ? "访问受限" : (pathname === "/account" ? "概览" : active?.title) || "我的账户"}</span></div><div className="center-userbar">{user && <Link href="/account/profile" aria-label="编辑个人头像"><UserAvatar src={user.avatar_url} name={user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])} decorative /></Link>}<span className="center-user-email">{user?.email_is_alias ? user.display_name || "LINUX DO 账户" : user?.email}</span>{user && <Badge variant="outline">{roleNames[user.role] || "用户"}</Badge>}<Button variant="ghost" size="icon" aria-label="退出登录" disabled={!user || leaving} onClick={logout}><LogOut /></Button></div></header>
        <main id="center-content" tabIndex={-1} className="center-content">
          {error && <Alert variant="destructive" className="mb-6"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => window.location.reload()}>重新加载</Button></AlertDescription></Alert>}
          {user ? denied ? <Empty className="min-h-80"><EmptyHeader><EmptyMedia variant="icon"><ShieldX /></EmptyMedia><EmptyTitle><h1>此页面仅限管理员访问</h1></EmptyTitle><EmptyDescription>你可以在用户中心使用已开放的应用，并管理自己的账户。</EmptyDescription></EmptyHeader><EmptyContent><Button nativeButton={false} role="link" render={<Link href="/account" />}>返回用户中心</Button></EmptyContent></Empty> : <AccountContext.Provider value={{ user, updateUser: setUser }}><div key={pathname} className="center-page-enter">{children}</div></AccountContext.Provider> : !error && <div aria-label="正在加载账户" className="flex flex-col gap-5"><Skeleton className="h-10 w-48" /><Skeleton className="h-36 w-full" /><Skeleton className="h-64 w-full" /></div>}
        </main>
        <footer className="center-footer">AlbaTextus · 让每个应用从同一个账户开始</footer>
      </div>
    </div>
  )
}
