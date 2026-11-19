"use client"

import { useFeedback, redirectWithFeedback } from "@/lib/feedback"

import { Link } from "next-view-transitions"
import { usePathname } from "next/navigation"
import { createContext, useContext, useEffect, useLayoutEffect, useState, type ReactNode } from "react"
import { motion, useAnimationControls, useReducedMotion } from "framer-motion"
import { Activity, AlertCircle, House, KeyRound, LayoutGrid, LibraryBig, LogOut, Settings2, ShieldCheck, ShieldX, UserRound, Users } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { isAdminRole, managementError } from "@/lib/admin/applications"
import { profileSetupPath } from "@/lib/auth/profile-setup"
import { cn } from "@/lib/utils"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { SiteHeader, navigationTransition, type NavigationItem } from "@/components/layout/site-header"
import { ThemeToggle } from "@/components/layout/theme-toggle"

export type AccountUser = { id: string; numeric_id: number; bio?: string; background_url?: string; background_original_url?: string; avatar_original_url?: string; email: string; email_verified?: boolean; profile_setup_required?: boolean; display_name?: string | null; role: string; level: 1 | 2 | 3 | 4; created_at?: string; has_password?: boolean; email_is_alias?: boolean; avatar_source?: string; avatar_url?: string; custom_avatar_url?: string }
const AccountContext = createContext<{ user: AccountUser; updateUser: (user: AccountUser) => void } | null>(null)
function useAccountContext() {
  const context = useContext(AccountContext)
  if (!context) throw new Error("Account provider is required")
  return context
}
export function useAccountUser() { return useAccountContext().user }
export function useUpdateAccountUser() { return useAccountContext().updateUser }

const userNavigation: NavigationItem[] = [
  { href: "/library", title: "返回书库", icon: LibraryBig, tone: "blue", leadingGap: 90 },
  { href: "/account", title: "用户中心", icon: House, tone: "orange" },
  { href: "/account/profile", title: "个人资料", icon: UserRound, tone: "blue" },
  { href: "/account/security", title: "账户安全", icon: ShieldCheck, tone: "teal" },
  { href: "/account/key", title: "API 密钥", icon: KeyRound, tone: "violet" },
]
const adminNavigation: NavigationItem[] = [
  { href: "/library", title: "返回书库", icon: LibraryBig, tone: "blue", leadingGap: 90 },
  { href: "/admin/users", title: "用户管理", icon: Users, tone: "blue" },
  { href: "/admin/usage", title: "调用统计", icon: Activity, tone: "teal" },
  { href: "/admin/applications", title: "应用接入", icon: LayoutGrid, tone: "violet" },
  { href: "/admin/settings", title: "配置", icon: Settings2, tone: "pink" },
]

export function AccountShell({ children, area = "account", initialUser }: { children: ReactNode; area?: "account" | "admin"; initialUser?: AccountUser }) {
  const pathname = usePathname()
  const reducedMotion = Boolean(useReducedMotion())
  const pageAnimation = useAnimationControls()
  const [user, setUser] = useState<AccountUser | null>(initialUser || null)
  const [error, setError] = useFeedback("error")
  const [leaving, setLeaving] = useState(false)
  const admin = Boolean(user && isAdminRole(user.role))
  const denied = area === "admin" && Boolean(user) && !admin
  const accountPath = user ? `/account/${user.numeric_id}` : "/account"
  const navigation = area === "admin" && !denied ? adminNavigation : userNavigation.map(item => ({ ...item, href: item.href.startsWith("/") ? item.href.replace("/account", accountPath) : item.href }))
  const active = navigation.find(item => item.href === accountPath ? pathname === item.href : pathname.startsWith(item.href))
  const title = area === "admin" && !denied ? "管理后台" : "用户中心"
  const areaLink: NavigationItem = area === "admin"
    ? { href: accountPath, title: "用户中心", icon: House, tone: "orange" }
    : { href: "/admin/users", title: "管理后台", icon: Settings2, tone: "pink" }

  useLayoutEffect(() => {
    if (!user || denied) return
    pageAnimation.stop()
    // Native transitions own the old/new page snapshots. Only animate the live
    // wrapper as a fallback, before paint, without remounting the page state.
    if (reducedMotion || "startViewTransition" in document) {
      pageAnimation.set({ opacity: 1, y: 0 })
      return
    }
    pageAnimation.set({ opacity: 0, y: 8 })
    void pageAnimation.start({ opacity: 1, y: 0, transition: navigationTransition })
    return () => pageAnimation.stop()
  }, [pathname, Boolean(user), denied, reducedMotion, pageAnimation])

  useEffect(() => {
    // The private account layout has already validated this session on the server.
    if (initialUser) return
    const controller = new AbortController()
    api<AccountUser>("/api/v1/user", { signal: controller.signal, cache: "no-store" }).then(value => {
      if (controller.signal.aborted) return
      if (value.profile_setup_required) { window.location.replace(profileSetupPath(window.location.pathname + window.location.search)); return }
      setUser(value)
      setError("")
    }).catch(reason => {
      if (controller.signal.aborted) return
      if (reason instanceof ApiError && reason.status === 401) {
        window.location.href = `/login?return_to=${encodeURIComponent(window.location.pathname)}`
      } else setError(managementError(reason))
    })
    return () => controller.abort()
  }, [initialUser, setError])

  async function logout() {
    setLeaving(true)
    try { await api("/api/v1/sessions/current", { method: "DELETE" }); redirectWithFeedback("/login", "已退出登录") }
    catch (reason) { setError(managementError(reason)); setLeaving(false) }
  }

  return (
    <div className={cn("center-shell", pathname === accountPath && "center-profile-home")}>
      <a className="center-skip" href="#center-content">跳转到主要内容</a>
      <SiteHeader items={admin ? [...navigation, areaLink] : navigation} activeHref={active?.href}
        navigationLabel={title + "导航"} routeKey={pathname} LinkComponent={Link}
        actions={<><ThemeToggle /><Button variant="ghost" size="icon" aria-label="退出登录" disabled={!user || leaving} onClick={logout}><LogOut /></Button></>} />
      <div className="center-workspace">
        <main id="center-content" tabIndex={-1} className="center-content">
          {error && <Alert variant="destructive" className="mb-6"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => window.location.reload()}>重新加载</Button></AlertDescription></Alert>}
          {user ? denied ? <Empty className="min-h-80"><EmptyHeader><EmptyMedia variant="icon"><ShieldX /></EmptyMedia><EmptyTitle><h1>此页面仅限管理员访问</h1></EmptyTitle><EmptyDescription>你可以返回用户中心管理自己的账户。</EmptyDescription></EmptyHeader><EmptyContent><Button nativeButton={false} role="link" render={<Link href="/account" />}>返回用户中心</Button></EmptyContent></Empty> : <AccountContext.Provider value={{ user, updateUser: setUser }}><motion.div className="center-page-enter" initial={false} animate={pageAnimation}>{children}</motion.div></AccountContext.Provider> : !error && <div aria-label="正在加载账户" className="flex flex-col gap-5"><Skeleton className="h-10 w-48" /><Skeleton className="h-36 w-full" /><Skeleton className="h-64 w-full" /></div>}
        </main>
      </div>
    </div>
  )
}
