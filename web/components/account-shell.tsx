"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { createContext, useContext, useEffect, useId, useState, type ReactNode } from "react"
import { LayoutGroup, motion, useAnimationControls, useReducedMotion } from "framer-motion"
import { AlertCircle, BookOpen, FileText, House, LayoutGrid, LogOut, Settings2, ShieldCheck, ShieldX, UserRound, Users, type LucideIcon } from "lucide-react"
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

type NavigationItem = { href: string; title: string; icon: LucideIcon }
const MotionLink = motion.create(Link)
const navigationTransition = { duration: 0.32, ease: [0.22, 1, 0.36, 1] } as const
const userNavigation: NavigationItem[] = [
  { href: "/account", title: "用户中心", icon: House },
  { href: "/account/profile", title: "个人资料", icon: UserRound },
  { href: "/account/security", title: "账户安全", icon: ShieldCheck },
]
const adminNavigation: NavigationItem[] = [
  { href: "/admin/users", title: "用户管理", icon: Users },
  { href: "/admin/applications", title: "应用接入", icon: LayoutGrid },
  { href: "/admin/integration", title: "接入指南", icon: BookOpen },
  { href: "/admin/legal", title: "协议管理", icon: FileText },
]

function CapsuleNavItem({ item, active = false, reducedMotion }: { item: NavigationItem; active?: boolean; reducedMotion: boolean }) {
  const transition = reducedMotion ? { duration: 0 } : navigationTransition
  return (
    <MotionLink
      href={item.href}
      prefetch
      className={cn("center-nav-link", active && "is-active")}
      aria-label={item.title}
      aria-current={active ? "page" : undefined}
      title={active ? undefined : item.title}
      transition={transition}
      whileHover={reducedMotion ? undefined : { y: -1 }}
      whileTap={reducedMotion ? undefined : { scale: 0.97 }}
    >
      {active && <motion.span className="center-nav-highlight" layoutId={reducedMotion ? undefined : "active-navigation"} transition={transition} aria-hidden="true" />}
      <item.icon className="center-nav-icon" strokeWidth={1.8} aria-hidden="true" />
      <motion.span
        className="center-nav-label"
        aria-hidden="true"
        initial={false}
        animate={{ width: active ? "auto" : 0, opacity: active ? 1 : 0 }}
        transition={transition}
      ><span>{item.title}</span></motion.span>
    </MotionLink>
  )
}

export function AccountShell({ children, area = "account" }: { children: ReactNode; area?: "account" | "admin" }) {
  const pathname = usePathname()
  const navigationId = useId()
  const reducedMotion = Boolean(useReducedMotion())
  const pageAnimation = useAnimationControls()
  const [user, setUser] = useState<AccountUser | null>(null)
  const [error, setError] = useState("")
  const [leaving, setLeaving] = useState(false)
  const admin = Boolean(user && isAdminRole(user.role))
  const denied = area === "admin" && Boolean(user) && !admin
  const navigation = area === "admin" && !denied ? adminNavigation : userNavigation
  const active = navigation.find(item => item.href === "/account" ? pathname === item.href : pathname.startsWith(item.href))
  const title = area === "admin" && !denied ? "管理后台" : "用户中心"
  const areaLink: NavigationItem = area === "admin"
    ? { href: "/account", title: "用户中心", icon: House }
    : { href: "/admin/applications", title: "管理后台", icon: Settings2 }

  useEffect(() => {
    if (!user || denied) return
    pageAnimation.stop()
    // Keep the route wrapper mounted: changing its key also remounts page state.
    // A shallow fade keeps the new content readable throughout the transition.
    if (reducedMotion) {
      pageAnimation.set({ opacity: 1, y: 0 })
      return
    }
    pageAnimation.set({ opacity: 0.65, y: 5 })
    void pageAnimation.start({ opacity: 1, y: 0, transition: navigationTransition })
    return () => pageAnimation.stop()
  }, [pathname, Boolean(user), denied, reducedMotion, pageAnimation])

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
      <header className="center-topbar">
        <Link href="/" className="center-brand" aria-label="AlbaTextus 首页">AlbaTextus</Link>
        <LayoutGroup id={navigationId}>
          <nav aria-label={title + "导航"} className="center-nav">
            {navigation.map(item => <CapsuleNavItem key={item.href} item={item} active={active?.href === item.href} reducedMotion={reducedMotion} />)}
            {admin && <CapsuleNavItem key={areaLink.href} item={areaLink} reducedMotion={reducedMotion} />}
          </nav>
        </LayoutGroup>
        <div className="center-userbar">{user && <Link href="/account/profile" aria-label="编辑个人头像"><UserAvatar src={user.avatar_url} name={user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])} decorative /></Link>}<span className="center-user-email">{user?.email_is_alias ? user.display_name || "LINUX DO 账户" : user?.email}</span>{user && <Badge variant="outline">{roleNames[user.role] || "用户"}</Badge>}<Button variant="ghost" size="icon" aria-label="退出登录" disabled={!user || leaving} onClick={logout}><LogOut /></Button></div>
      </header>
      <div className="center-workspace">
        <main id="center-content" tabIndex={-1} className="center-content">
          {error && <Alert variant="destructive" className="mb-6"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => window.location.reload()}>重新加载</Button></AlertDescription></Alert>}
          {user ? denied ? <Empty className="min-h-80"><EmptyHeader><EmptyMedia variant="icon"><ShieldX /></EmptyMedia><EmptyTitle><h1>此页面仅限管理员访问</h1></EmptyTitle><EmptyDescription>你可以在用户中心使用已开放的应用，并管理自己的账户。</EmptyDescription></EmptyHeader><EmptyContent><Button nativeButton={false} role="link" render={<Link href="/account" />}>返回用户中心</Button></EmptyContent></Empty> : <AccountContext.Provider value={{ user, updateUser: setUser }}><motion.div className="center-page-enter" initial={false} animate={pageAnimation}>{children}</motion.div></AccountContext.Provider> : !error && <div aria-label="正在加载账户" className="flex flex-col gap-5"><Skeleton className="h-10 w-48" /><Skeleton className="h-36 w-full" /><Skeleton className="h-64 w-full" /></div>}
        </main>
        <footer className="center-footer">AlbaTextus · 让每个应用从同一个账户开始</footer>
      </div>
    </div>
  )
}
