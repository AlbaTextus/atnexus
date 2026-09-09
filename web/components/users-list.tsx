"use client"

import { useFeedback } from "@/lib/feedback"

import { useEffect, useState } from "react"
import { AnimatePresence, LayoutGroup, motion, useIsPresent, useReducedMotion } from "framer-motion"
import { AlertCircle, Check, ChevronLeft, ChevronRight, Circle, Plus, RefreshCw, Search, ShieldCheck, Users } from "lucide-react"
import { api } from "@/lib/api"
import { roleNames } from "@/lib/applications"
import { type ManagedUser, type UserListResult, userManagementError } from "@/lib/users"
import { UserDetailsRow } from "@/components/user-details-row"
import { UserManagementDialog, type UserAction } from "@/components/user-management-dialog"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader } from "@/components/ui/card"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

export function UsersList() {
  const reducedMotion = Boolean(useReducedMotion())
  const transition = reducedMotion ? { duration: 0 } : { duration: 0.24, ease: [0.22, 1, 0.36, 1] as const }
  const [result, setResult] = useState<UserListResult | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useFeedback("error")
  const [notice, setNotice] = useFeedback("success")
  const [search, setSearch] = useState("")
  const [query, setQuery] = useState("")
  const [status, setStatus] = useState("all")
  const [role, setRole] = useState("all")
  const [page, setPage] = useState(1)
  const [reload, setReload] = useState(0)
  const [action, setAction] = useState<UserAction | null>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => { setQuery(search.trim()); setPage(1) }, 300)
    return () => window.clearTimeout(timer)
  }, [search])

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError("")
    const params = new URLSearchParams({ page: String(page), page_size: "20", q: query })
    if (status !== "all") params.set("status", status)
    if (role !== "all") params.set("role", role)
    api<UserListResult>(`/api/v1/admin/users?${params}`, { signal: controller.signal }).then(data => {
      if (controller.signal.aborted) return
      if (page > 1 && data.users.length === 0) { setPage(Math.max(1, Math.ceil(data.total / data.page_size))); return }
      setResult(data)
    }).catch(reason => { if (!controller.signal.aborted) setError(userManagementError(reason)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [query, status, role, page, reload])

  const pages = Math.max(1, Math.ceil((result?.total || 0) / 20))
  function clearFilters() { setSearch(""); setQuery(""); setStatus("all"); setRole("all"); setPage(1) }
  function openAction(value: UserAction) { setNotice(""); setAction(value) }

  return <div className="users-list-page flex flex-1 flex-col gap-7">
    {notice && <Alert role="status"><Check /><AlertDescription>{notice}</AlertDescription></Alert>}
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => setReload(value => value + 1)}>重新加载</Button></AlertDescription></Alert>}
    <div className="user-stats grid grid-cols-3 gap-3 sm:gap-4">
      {[{ label: "全部用户", count: result?.stats.total, icon: Users, tone: "blue", note: "统一账户中心的所有用户" }, { label: "已启用", count: result?.stats.active, icon: Check, tone: "green", note: "可以登录并使用应用" }, { label: "管理人员", count: result?.stats.admins, icon: ShieldCheck, tone: "orange", note: "管理员与超级管理员" }].map(item => <Card key={item.label} className="gap-2 sm:gap-4"><CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 px-3 sm:px-4"><CardDescription>{item.label}</CardDescription><span className="application-stat-icon" data-tone={item.tone} aria-hidden="true"><item.icon size={18} /></span></CardHeader><CardContent className="px-3 sm:px-4"><div className="center-stat">{item.count ?? "—"}</div><p className="mt-1 hidden text-xs text-muted-foreground sm:block">{item.note}</p></CardContent></Card>)}
    </div>
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h1 className="text-xl font-semibold">用户管理</h1>
        <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto"><InputGroup className="h-10 min-w-0 flex-1 sm:w-64"><InputGroupAddon><Search /></InputGroupAddon><InputGroupInput aria-label="搜索用户" placeholder="搜索昵称、邮箱或用户 ID" maxLength={320} value={search} onChange={event => setSearch(event.target.value)} /></InputGroup><Button variant="outline" disabled={loading} onClick={() => setReload(value => value + 1)}><RefreshCw data-icon="inline-start" className={loading ? "animate-spin motion-reduce:animate-none" : undefined} />刷新列表</Button><Button onClick={() => openAction({ kind: "create" })}><Plus data-icon="inline-start" />新建用户</Button></div>
      </div>
      <div className="flex flex-wrap gap-3">
        <ToggleGroup value={[status]} onValueChange={values => { if (values[0]) { setStatus(values[0]); setPage(1) } }} variant="pill" size="lg" spacing={1} aria-label="用户状态筛选">
          <ToggleGroupItem value="all" aria-label="全部状态" title="全部状态"><Circle aria-hidden="true" className="text-muted-foreground" /></ToggleGroupItem>
          <ToggleGroupItem value="active" aria-label="已启用" title="已启用"><Circle aria-hidden="true" className="text-(--feedback-success)" /></ToggleGroupItem>
          <ToggleGroupItem value="inactive" aria-label="已停用" title="已停用"><Circle aria-hidden="true" className="text-(--feedback-error)" /></ToggleGroupItem>
        </ToggleGroup>
        <ToggleGroup value={[role]} onValueChange={values => { if (values[0]) { setRole(values[0]); setPage(1) } }} variant="pill" size="lg" spacing={1} aria-label="用户角色筛选">
          <ToggleGroupItem value="all" aria-label="全部角色" title="全部角色"><Circle aria-hidden="true" className="text-muted-foreground" /></ToggleGroupItem>
          <ToggleGroupItem value="USER" aria-label={roleNames.USER} title={roleNames.USER}>LT1</ToggleGroupItem>
          <ToggleGroupItem value="LT2" disabled aria-label="LT2（暂未开放）" title="LT2（暂未开放）">LT2</ToggleGroupItem>
          <ToggleGroupItem value="ADMIN" aria-label={roleNames.ADMIN} title={roleNames.ADMIN}>LT3</ToggleGroupItem>
          <ToggleGroupItem value="SUPER_ADMIN" aria-label={roleNames.SUPER_ADMIN} title={roleNames.SUPER_ADMIN}>LT4</ToggleGroupItem>
        </ToggleGroup>
      </div>
    </div>
    <span role="status" className="sr-only">{loading ? result ? "正在更新用户列表" : "正在加载用户" : error ? "用户列表更新失败" : `已显示 ${result?.users.length || 0} 位用户`}</span>
    <LayoutGroup>
      <motion.div layout={reducedMotion ? false : "size"} transition={transition} className="relative flex flex-col gap-3" aria-label="用户列表" aria-busy={loading}>
        <AnimatePresence initial={false} mode="popLayout">
          {!result && loading ? <motion.div key="loading" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition} className="flex flex-col gap-4" aria-label="正在加载用户"><Skeleton className="h-36 w-full" /><Skeleton className="h-36 w-full" /><Skeleton className="h-36 w-full" /></motion.div>
            : result?.users.length ? result.users.map(user => <AnimatedUserRow key={user.id} user={user} reducedMotion={reducedMotion} onSaved={message => { setNotice(message); setReload(value => value + 1) }} />)
            : result && <motion.div key="empty" layout={!reducedMotion} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={transition}><Empty className="min-h-64"><EmptyHeader><EmptyMedia variant="icon"><Users /></EmptyMedia><EmptyTitle>没有找到匹配的用户</EmptyTitle><EmptyDescription>试试其他搜索词，或调整角色与状态筛选。</EmptyDescription></EmptyHeader><EmptyContent><Button variant="outline" onClick={clearFilters}>清除筛选</Button></EmptyContent></Empty></motion.div>}
        </AnimatePresence>
      </motion.div>
      {result && <motion.nav layout={reducedMotion ? false : "position"} transition={transition} aria-label="用户列表分页" className="mt-auto flex flex-wrap items-center justify-between gap-3"><p className="text-sm text-muted-foreground" aria-live="polite">共 {result.total} 位用户 · 第 {result.page} / {pages} 页</p><div className="flex gap-2"><Button variant="outline" size="sm" disabled={loading || result.page <= 1} onClick={() => setPage(result.page - 1)}><ChevronLeft data-icon="inline-start" />上一页</Button><Button variant="outline" size="sm" disabled={loading || result.page >= pages} onClick={() => setPage(result.page + 1)}>下一页<ChevronRight data-icon="inline-end" /></Button></div></motion.nav>}
    </LayoutGroup>
    {action && <UserManagementDialog action={action} onClose={() => setAction(null)} onSaved={message => { setAction(null); setNotice(message); setReload(value => value + 1) }} />}
  </div>
}

// Stable user keys preserve open details and focus during background refreshes.
// Retiring rows remain only for their exit animation, never as clickable results.
function AnimatedUserRow({ user, onSaved, reducedMotion, ref }: { user: ManagedUser; onSaved: (message: string) => void; reducedMotion: boolean; ref?: React.Ref<HTMLDivElement> }) {
  const present = useIsPresent()
  return <motion.div ref={ref} layout={reducedMotion ? false : "position"} inert={!present} aria-hidden={!present || undefined}
    initial={{ opacity: 0, y: reducedMotion ? 0 : 8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: reducedMotion ? 0 : -4 }}
    transition={reducedMotion ? { duration: 0 } : { duration: 0.24, ease: [0.22, 1, 0.36, 1] }}>
    <UserDetailsRow user={user} onSaved={onSaved} />
  </motion.div>
}
