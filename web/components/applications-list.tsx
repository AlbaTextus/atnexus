"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { AlertCircle, ArrowUpRight, Check, LayoutGrid, Plus, Search, Settings2 } from "lucide-react"
import { api } from "@/lib/api"
import { type Application, managementError, roleNames } from "@/lib/applications"
import { ApplicationIcon } from "@/components/application-icon"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

export function ApplicationsList() {
  const [apps, setApps] = useState<Application[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [admin, setAdmin] = useState(false)
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState("all")
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
      {[{ label: admin ? "全部应用" : "我的应用", count: apps.length, icon: LayoutGrid, tone: "blue", note: admin ? "管理员可管理全部应用" : "你创建并管理的应用" }, { label: "已启用", count: activeCount, icon: Check, tone: "green", note: "可以进行登录授权" }, { label: "已停用", count: apps.length - activeCount, icon: Settings2, tone: "orange", note: "已暂停新的登录授权" }].map(item => <Card key={item.label} className="gap-2 sm:gap-4"><CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 px-3 sm:px-4"><CardDescription>{item.label}</CardDescription><span className="application-stat-icon" data-tone={item.tone} aria-hidden="true"><item.icon size={18} /></span></CardHeader><CardContent className="px-3 sm:px-4"><div className="center-stat">{loading ? "—" : item.count}</div><p className="mt-1 hidden text-xs text-muted-foreground sm:block">{item.note}</p></CardContent></Card>)}
    </div>
    <div className="flex flex-wrap items-center justify-between gap-4">
      <div className="flex flex-wrap items-center gap-4"><h1 className="text-xl font-semibold">应用接入</h1><ToggleGroup value={[filter]} onValueChange={values => { if (values[0]) setFilter(values[0]) }} variant="outline" spacing={0} aria-label="应用状态筛选"><ToggleGroupItem value="all">全部应用</ToggleGroupItem><ToggleGroupItem value="active">已启用</ToggleGroupItem><ToggleGroupItem value="inactive">已停用</ToggleGroupItem></ToggleGroup></div>
      <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto"><InputGroup className="h-10 min-w-0 flex-1 sm:w-56"><InputGroupAddon><Search /></InputGroupAddon><InputGroupInput aria-label="搜索应用" placeholder="搜索名称或 Client ID" value={search} onChange={event => setSearch(event.target.value)} /></InputGroup><Button nativeButton={false} role="link" render={<Link href="/admin/applications/new" />}><Plus data-icon="inline-start" />创建应用</Button></div>
    </div>
    {loading ? <div className="flex flex-col gap-4" aria-label="正在加载应用"><Skeleton className="h-32 w-full" /><Skeleton className="h-32 w-full" /></div> : visible.length > 0 ? <div className="flex flex-col gap-4">{visible.map(app => <Card key={app.id} interactive><CardContent className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-4 py-2 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center"><ApplicationIcon url={app.icon_url} name={app.name} /><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h2 className="break-words text-base font-semibold">{app.name}</h2><Badge variant={app.is_active ? "success" : "secondary"}>{app.is_active ? "已启用" : "已停用"}</Badge></div><p className="mt-1 break-all font-mono text-xs text-muted-foreground">{app.id}</p><div className="mt-3 flex flex-wrap gap-2"><Badge variant="outline">{app.is_public ? "公开客户端 · PKCE" : "服务端应用"}</Badge><Badge variant="secondary">{roleNames[app.minimum_role] || "普通用户"}可访问</Badge></div></div><Button variant="outline" size="lg" nativeButton={false} role="link" className="col-start-2 justify-self-start sm:col-start-3 sm:row-start-1 sm:justify-self-end" render={<Link href={`/admin/applications/${encodeURIComponent(app.id)}`} />}><Settings2 data-icon="inline-start" />管理</Button></CardContent></Card>)}</div> : <Card><CardContent><Empty className="py-12"><EmptyHeader><EmptyMedia variant="icon"><LayoutGrid /></EmptyMedia><EmptyTitle>{apps.length ? "没有找到匹配的应用" : "接入你的第一个应用"}</EmptyTitle><EmptyDescription>{apps.length ? "试试其他搜索词或状态筛选。" : "创建应用，配置回调地址，即可使用统一账户登录你的子系统。"}</EmptyDescription></EmptyHeader><EmptyContent>{apps.length ? <Button variant="outline" onClick={() => { setSearch(""); setFilter("all") }}>清除筛选</Button> : <Button nativeButton={false} role="link" render={<Link href="/admin/applications/new" />}><Plus data-icon="inline-start" />创建应用</Button>}</EmptyContent></Empty></CardContent></Card>}
    <Card><CardHeader><CardTitle>第一次接入？</CardTitle><CardDescription>查看授权流程、接口地址和回调处理说明。</CardDescription></CardHeader><CardContent><Button variant="link" nativeButton={false} role="link" render={<Link href="/wiki/integration" />} className="px-0">阅读接入指南<ArrowUpRight data-icon="inline-end" /></Button></CardContent></Card>
  </div>
}
