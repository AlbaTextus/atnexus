"use client"

import { useEffect, useState } from "react"
import { AlertCircle, Check, ChevronLeft, ChevronRight, KeyRound, LogOut, Plus, RefreshCw, Search, Settings2, ShieldCheck, Users } from "lucide-react"
import { api } from "@/lib/api"
import { roleNames } from "@/lib/applications"
import { canManageUser, type UserListResult, userManagementError } from "@/lib/users"
import { useAccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { UserManagementDialog, type UserAction } from "@/components/user-management-dialog"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

export function UsersList() {
  const actor = useAccountUser()
  const [result, setResult] = useState<UserListResult | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
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
    }).catch(reason => { if (!controller.signal.aborted) { setResult(null); setError(userManagementError(reason)) } }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [query, status, role, page, reload])

  const pages = Math.max(1, Math.ceil((result?.total || 0) / 20))
  function clearFilters() { setSearch(""); setQuery(""); setStatus("all"); setRole("all"); setPage(1) }
  function openAction(value: UserAction) { setNotice(""); setAction(value) }

  return <div className="flex flex-col gap-7">
    <div className="center-page-heading"><div><p className="center-eyebrow">USERS</p><h1>用户管理</h1><p>管理账户资料、访问权限与登录状态。</p></div><Button size="lg" onClick={() => openAction({ kind: "create" })}><Plus data-icon="inline-start" />新建用户</Button></div>
    {notice && <Alert role="status"><Check /><AlertDescription>{notice}</AlertDescription></Alert>}
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => setReload(value => value + 1)}>重新加载</Button></AlertDescription></Alert>}
    <div className="grid grid-cols-3 gap-3 sm:gap-4">
      {[{ label: "全部用户", count: result?.stats.total, icon: Users, note: "统一账户中心的所有用户" }, { label: "已启用", count: result?.stats.active, icon: Check, note: "可以登录并使用应用" }, { label: "管理人员", count: result?.stats.admins, icon: ShieldCheck, note: "管理员与超级管理员" }].map(item => <Card key={item.label}><CardHeader className="px-3 sm:px-4"><CardTitle className="flex items-center justify-between gap-2">{item.label}<item.icon className="hidden size-4 text-muted-foreground sm:block" /></CardTitle><CardDescription className="hidden sm:block">{item.note}</CardDescription></CardHeader><CardContent className="px-3 sm:px-4"><div className="center-stat">{item.count ?? "—"}</div></CardContent></Card>)}
    </div>
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3"><InputGroup className="h-10 w-full sm:w-80"><InputGroupAddon><Search /></InputGroupAddon><InputGroupInput aria-label="搜索用户" placeholder="搜索昵称、邮箱或用户 ID" maxLength={320} value={search} onChange={event => setSearch(event.target.value)} /></InputGroup><Button variant="outline" disabled={loading} onClick={() => setReload(value => value + 1)}><RefreshCw data-icon="inline-start" />刷新列表</Button></div>
      <div className="flex flex-wrap gap-3"><ToggleGroup value={[status]} onValueChange={values => { if (values[0]) { setStatus(values[0]); setPage(1) } }} variant="outline" spacing={0} aria-label="用户状态筛选"><ToggleGroupItem value="all">全部状态</ToggleGroupItem><ToggleGroupItem value="active">已启用</ToggleGroupItem><ToggleGroupItem value="inactive">已停用</ToggleGroupItem></ToggleGroup><ToggleGroup value={[role]} onValueChange={values => { if (values[0]) { setRole(values[0]); setPage(1) } }} variant="outline" spacing={0} aria-label="用户角色筛选"><ToggleGroupItem value="all">全部角色</ToggleGroupItem><ToggleGroupItem value="USER">普通用户</ToggleGroupItem><ToggleGroupItem value="ADMIN">管理员</ToggleGroupItem><ToggleGroupItem value="SUPER_ADMIN">超级管理员</ToggleGroupItem></ToggleGroup></div>
    </div>
    {loading ? <div className="flex flex-col gap-4" aria-label="正在加载用户"><Skeleton className="h-36 w-full" /><Skeleton className="h-36 w-full" /><Skeleton className="h-36 w-full" /></div> : result && <>
      {result.users.length ? <div className="flex flex-col gap-3">{result.users.map(user => {
        const manageable = canManageUser(actor, user)
        const self = actor.id === user.id
        return <Card key={user.id}><CardHeader className="gap-3"><div className="flex items-start gap-3"><UserAvatar src={user.avatar_url} name={user.display_name || (user.email_is_alias ? "用户" : user.email)} className="size-10" decorative /><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><CardTitle><h2 className="break-all">{user.display_name || (user.email_is_alias ? "第三方登录用户" : user.email.split("@")[0])}</h2></CardTitle><Badge variant={user.is_active ? "success" : "secondary"}>{user.is_active ? "已启用" : "已停用"}</Badge><Badge variant="outline">{roleNames[user.role] || user.role}</Badge>{self && <Badge variant="secondary">当前账户</Badge>}</div><CardDescription className="mt-1 break-all">{user.email_is_alias ? "登录标识：" : ""}{user.email}</CardDescription></div></div></CardHeader><CardContent className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"><div className="flex min-w-0 flex-col gap-1 text-xs text-muted-foreground"><p className="break-all font-mono">{user.id}</p><p>注册于 {new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", timeZone: "Asia/Shanghai" }).format(new Date(user.created_at))} · {user.has_password ? "已设置密码" : "第三方登录 / 未设置密码"}</p></div>{manageable ? <div className="flex shrink-0 flex-wrap gap-2"><Button variant="outline" size="sm" aria-label={`编辑 ${user.email}`} onClick={() => openAction({ kind: "edit", user })}><Settings2 data-icon="inline-start" />编辑</Button>{!self && <><Button variant="outline" size="sm" aria-label={`重置密码 ${user.email}`} onClick={() => openAction({ kind: "password", user })}><KeyRound data-icon="inline-start" />重置密码</Button><Button variant="outline" size="sm" aria-label={`撤销登录 ${user.email}`} onClick={() => openAction({ kind: "revoke", user })}><LogOut data-icon="inline-start" />撤销登录</Button></>}</div> : <p className="text-xs text-muted-foreground">{self ? "个人资料与密码请在用户中心修改" : "由超级管理员管理"}</p>}</CardContent></Card>
      })}</div> : <Empty className="min-h-64"><EmptyHeader><EmptyMedia variant="icon"><Users /></EmptyMedia><EmptyTitle>没有找到匹配的用户</EmptyTitle><EmptyDescription>试试其他搜索词，或调整角色与状态筛选。</EmptyDescription></EmptyHeader><EmptyContent><Button variant="outline" onClick={clearFilters}>清除筛选</Button></EmptyContent></Empty>}
      <nav aria-label="用户列表分页" className="flex flex-wrap items-center justify-between gap-3"><p className="text-sm text-muted-foreground" aria-live="polite">共 {result.total} 位用户 · 第 {page} / {pages} 页</p><div className="flex gap-2"><Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(value => value - 1)}><ChevronLeft data-icon="inline-start" />上一页</Button><Button variant="outline" size="sm" disabled={page >= pages} onClick={() => setPage(value => value + 1)}>下一页<ChevronRight data-icon="inline-end" /></Button></div></nav>
    </>}
    {action && <UserManagementDialog action={action} onClose={() => setAction(null)} onSaved={message => { setAction(null); setNotice(message); setReload(value => value + 1) }} />}
  </div>
}
