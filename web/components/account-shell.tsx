"use client"

import { useFeedback, redirectWithFeedback } from "@/lib/feedback"

import { Link } from "next-view-transitions"
import { usePathname } from "next/navigation"
import { createContext, useContext, useEffect, useLayoutEffect, useState, type ReactNode } from "react"
import { motion, useAnimationControls, useReducedMotion } from "framer-motion"
import { AlertCircle, House, LayoutGrid, LogOut, Settings2, ShieldCheck, ShieldX, UserRound, Users, WalletCards } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { isAdminRole, managementError } from "@/lib/applications"
import { profileSetupPath } from "@/lib/profile-setup"
import { cn } from "@/lib/utils"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { SiteHeader, navigationTransition, type NavigationItem } from "@/components/site-header"
import { MaintenanceControl } from "@/components/maintenance-control"
import { ThemeToggle } from "@/components/theme-toggle"

export type AccountUser = { id: string; numeric_id: number; bio?: string; email: string; email_verified?: boolean; profile_setup_required?: boolean; display_name?: string | null; role: string; level: 1 | 3 | 4; created_at?: string; has_password?: boolean; email_is_alias?: boolean; avatar_source?: string; avatar_url?: string; custom_avatar_url?: string }
const AccountContext = createContext<{ user: AccountUser; updateUser: (user: AccountUser) => void } | null>(null)
function useAccountContext() {
  const context = useContext(AccountContext)
  if (!context) throw new Error("Account provider is required")
  return context
}
export function useAccountUser() { return useAccountContext().user }
export function useUpdateAccountUser() { return useAccountContext().updateUser }

const userNavigation: NavigationItem[] = [
  { href: "/account", title: "用户中心", icon: House },
  { href: "/account/profile", title: "个人资料", icon: UserRound },
  { href: "/account/security", title: "账户安全", icon: ShieldCheck },
  { href: "/account/wallet", title: "我的钱包", icon: WalletCards },
]
const adminNavigation: NavigationItem[] = [
  { href: "/admin/users", title: "用户管理", icon: Users },
  { href: "/admin/applications", title: "应用接入", icon: LayoutGrid },
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
  const navigation = area === "admin" && !denied ? adminNavigation : userNavigation.map(item => ({ ...item, href: item.href.replace("/account", accountPath) }))
  const active = navigation.find(item => item.href === accountPath ? pathname === item.href : pathname.startsWith(item.href))
  const title = area === "admin" && !denied ? "管理后台" : "用户中心"
  const areaLink: NavigationItem = area === "admin"
    ? { href: accountPath, title: "用户中心", icon: House }
    : { href: "/admin/users", title: "管理后台", icon: Settings2 }

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
    let cancelled = false
    api<AccountUser>("/api/v1/user").then(value => {
      if (cancelled) return
      if (value.profile_setup_required) { window.location.replace(profileSetupPath(window.location.pathname + window.location.search)); return }
      setUser(value)
    }).catch(reason => {
      if (cancelled) return
      if (reason instanceof ApiError && reason.status === 401) {
        window.location.href = `/login?return_to=${encodeURIComponent(window.location.pathname)}`
      } else setError(managementError(reason))
    })
    return () => { cancelled = true }
  }, [])

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
        actions={<><ThemeToggle />{user?.role === "SUPER_ADMIN" && <MaintenanceControl />}<Button variant="ghost" size="icon" aria-label="退出登录" disabled={!user || leaving} onClick={logout}><LogOut /></Button></>} />
      <div className="center-workspace">
        <main id="center-content" tabIndex={-1} className="center-content">
          {error && <Alert variant="destructive" className="mb-6"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => window.location.reload()}>重新加载</Button></AlertDescription></Alert>}
          {user ? denied ? <Empty className="min-h-80"><EmptyHeader><EmptyMedia variant="icon"><ShieldX /></EmptyMedia><EmptyTitle><h1>此页面仅限管理员访问</h1></EmptyTitle><EmptyDescription>你可以在用户中心使用已开放的应用，并管理自己的账户。</EmptyDescription></EmptyHeader><EmptyContent><Button nativeButton={false} role="link" render={<Link href="/account" />}>返回用户中心</Button></EmptyContent></Empty> : <AccountContext.Provider value={{ user, updateUser: setUser }}><motion.div className="center-page-enter" initial={false} animate={pageAnimation}>{children}</motion.div></AccountContext.Provider> : !error && <div aria-label="正在加载账户" className="flex flex-col gap-5"><Skeleton className="h-10 w-48" /><Skeleton className="h-36 w-full" /><Skeleton className="h-64 w-full" /></div>}
        </main>
      </div>
    </div>
  )
}
