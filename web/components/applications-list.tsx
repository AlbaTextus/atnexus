"use client"

import { Link } from "next-view-transitions"
import { useEffect, useRef, useState } from "react"
import { AlertCircle, ArrowUpRight, Check, ChevronRight, Circle, LayoutGrid, Plus, Search, Settings2 } from "lucide-react"
import { api } from "@/lib/api"
import { type Application, managementError } from "@/lib/applications"
import { feedback } from "@/lib/feedback"
import { wikiUrl } from "@/lib/wiki"
import { ApplicationIcon } from "@/components/application-icon"
import { SiteFeatureControls } from "@/components/site-feature-controls"
import { Badge } from "@/components/ui/badge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle, StatCard } from "@/components/ui/card"
import { EmptyState } from "@/components/ui/empty"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

const minimumLevels: Record<string, string> = { USER: "LT1", LEVEL_2: "LT2", ADMIN: "LT3", SUPER_ADMIN: "LT4" }

export function ApplicationsList() {
  const [apps, setApps] = useState<Application[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [admin, setAdmin] = useState(false)
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState("all")
  const [pending, setPending] = useState<string[]>([])
  const inFlight = useRef(new Set<string>())
  async function toggleStatus(app: Application) {
    if (inFlight.current.has(app.id)) return
    inFlight.current.add(app.id)
    setPending(ids => [...ids, app.id])
    try {
      const updated = await api<Application>(`/api/v1/applications/${encodeURIComponent(app.id)}/status`, { method: "PATCH", body: JSON.stringify({ is_active: !app.is_active }) })
      setApps(items => items.map(item => item.id === app.id ? updated : item))
      feedback.success(`${updated.name}已${updated.is_active ? "启用" : "停用"}`)
    } catch (reason) { feedback.error(managementError(reason)) }
    finally { inFlight.current.delete(app.id); setPending(ids => ids.filter(id => id !== app.id)) }
  }
  async function load() {
    setLoading(true); setError("")
    try { const result = await api<{ applications: Application[]; can_manage_all: boolean }>("/api/v1/applications"); setApps(result.applications); setAdmin(result.can_manage_all) }
    catch (reason) { setError(managementError(reason)) } finally { setLoading(false) }
  }
  useEffect(() => { void load() }, [])
  const activeCount = apps.filter(app => app.is_active).length
  const visible = apps.filter(app => (filter === "all" || app.is_active === (filter === "active")) && `${app.name} ${app.id}`.toLowerCase().includes(search.trim().toLowerCase()))

  return <div className="flex flex-col gap-7">
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={load}>重试</Button></AlertDescription></Alert>}
    <div className="application-stats grid grid-cols-3 gap-3 sm:gap-4">
      {([
        { label: admin ? "全部应用" : "我的应用", count: apps.length, icon: LayoutGrid, tone: "blue", note: admin ? "管理员可管理全部应用" : "你创建并管理的应用" },
        { label: "已启用", count: activeCount, icon: Check, tone: "green", note: "可以进行登录授权" },
        { label: "已停用", count: apps.length - activeCount, icon: Settings2, tone: "orange", note: "已暂停新的登录授权" },
      ] as const).map(item => <StatCard key={item.label} label={item.label} value={loading ? null : item.count} icon={<item.icon size={18} />} tone={item.tone} note={item.note} />)}
    </div>
    <div className="flex flex-wrap items-center justify-between gap-4">
      <div className="flex flex-wrap items-center gap-4">
        <h1 className="text-xl font-semibold">应用接入</h1>
        <ToggleGroup value={[filter]} onValueChange={values => { if (values[0]) setFilter(values[0]) }} variant="pill" size="lg" spacing={1} aria-label="应用状态筛选">
          <ToggleGroupItem value="all" aria-label="全部应用" title="全部应用"><Circle aria-hidden="true" className="text-muted-foreground" /></ToggleGroupItem>
          <ToggleGroupItem value="active" aria-label="已启用" title="已启用"><Circle aria-hidden="true" className="text-(--feedback-success)" /></ToggleGroupItem>
          <ToggleGroupItem value="inactive" aria-label="已停用" title="已停用"><Circle aria-hidden="true" className="text-(--feedback-error)" /></ToggleGroupItem>
        </ToggleGroup>
      </div>
      <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto"><InputGroup className="h-10 min-w-0 flex-1 sm:w-56"><InputGroupAddon><Search /></InputGroupAddon><InputGroupInput aria-label="搜索应用" placeholder="搜索名称或 Client ID" value={search} onChange={event => setSearch(event.target.value)} /></InputGroup><Button nativeButton={false} role="link" render={<Link href="/admin/applications/new" />}><Plus data-icon="inline-start" />创建应用</Button></div>
    </div>
    {loading ? <div className="flex flex-col gap-3" aria-label="正在加载应用"><Skeleton className="h-20 w-full" /><Skeleton className="h-20 w-full" /></div> : visible.length > 0 ? <div role="list" aria-label="接入列表" className="flex flex-col gap-3">{visible.map(app =>
      <div key={app.id} role="listitem" className="managed-user-row managed-application-row">
        <Link href={`/admin/applications/${encodeURIComponent(app.id)}`} className="managed-application-main" aria-label={`管理 ${app.name}`}>
          <ApplicationIcon url={app.icon_url} name={app.name} />
          <h2 className="managed-user-name">{app.name}</h2>
        </Link>
        <Badge variant="outline" aria-label={`最低访问等级 ${minimumLevels[app.minimum_role] || "未设置"}`}>{minimumLevels[app.minimum_role] || "—"}</Badge>
        <Badge variant={app.is_active ? "success" : "secondary"} role="switch" aria-checked={app.is_active} aria-label={`启用 ${app.name}`} aria-busy={pending.includes(app.id)} onClick={() => toggleStatus(app)} title={app.is_active ? "点击停用，已有授权将失效" : "点击启用"} className="managed-application-status h-7 px-3" render={<button type="button" disabled={pending.includes(app.id)} />}>
          {pending.includes(app.id) && <Spinner data-icon="inline-start" />}
          {app.is_active ? "已启用" : "已停用"}
        </Badge>
        <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      </div>
    )}</div> : <Card><CardContent><EmptyState className="py-12" icon={<LayoutGrid />}
      title={apps.length ? "没有找到匹配的应用" : "接入你的第一个应用"}
      description={apps.length ? "试试其他搜索词或状态筛选。" : "创建应用，配置回调地址，即可使用统一账户登录你的子系统。"}>
      {apps.length ? <Button variant="outline" onClick={() => { setSearch(""); setFilter("all") }}>清除筛选</Button> : <Button nativeButton={false} role="link" render={<Link href="/admin/applications/new" />}><Plus data-icon="inline-start" />创建应用</Button>}
    </EmptyState></CardContent></Card>}
    <Card><CardHeader><CardTitle>第一次接入？</CardTitle><CardDescription>查看授权流程、接口地址和回调处理说明。</CardDescription></CardHeader><CardContent><Button variant="link" nativeButton={false} role="link" render={<a href={wikiUrl("/integration")} />} className="px-0">阅读接入指南<ArrowUpRight data-icon="inline-end" /></Button></CardContent></Card>
    <SiteFeatureControls />
  </div>
}
