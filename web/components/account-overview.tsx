"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { AlertCircle, ArrowUpRight, LayoutGrid, Search, ShieldCheck, UserRound } from "lucide-react"
import { api } from "@/lib/api"
import { managementError, roleNames } from "@/lib/applications"
import { useAccountUser } from "@/components/account-shell"
import { ApplicationIcon } from "@/components/application-icon"
import { UserAvatar } from "@/components/user-avatar"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"

type AvailableApplication = { id: string; name: string; description: string; homepage_url: string; icon_url: string }

export function AccountOverview() {
  const user = useAccountUser()
  const [apps, setApps] = useState<AvailableApplication[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [search, setSearch] = useState("")
  async function load() {
    setLoading(true); setError("")
    try { const result = await api<{ applications: AvailableApplication[] }>("/api/v1/account/applications"); setApps(result.applications) }
    catch (reason) { setError(managementError(reason)) } finally { setLoading(false) }
  }
  useEffect(() => { void load() }, [])
  const visible = apps.filter(app => `${app.name} ${app.description}`.toLowerCase().includes(search.trim().toLowerCase()))

  return <div className="flex flex-col gap-7">
    <div className="center-page-heading"><div><p className="center-eyebrow">YOUR SPACE</p><h1>你好，{user.display_name || user.email.split("@")[0]}</h1><p>欢迎来到你的用户中心。从这里开始，连接每一个应用。</p></div><Badge variant="success"><ShieldCheck data-icon="inline-start" />账户正常</Badge></div>
    <Card>
      <CardHeader><CardTitle>我的账户</CardTitle><CardDescription>你的个人资料与登录方式，在这里统一管理。</CardDescription></CardHeader>
      <CardContent className="flex flex-wrap items-center justify-between gap-5">
        <div className="flex min-w-0 items-center gap-4"><UserAvatar src={user.avatar_url} name={user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])} className="size-12" /><div className="min-w-0"><p className="break-all font-medium">{user.email_is_alias ? user.display_name || "LINUX DO 账户" : user.email}</p><div className="mt-2 flex flex-wrap gap-2"><Badge variant="secondary">{roleNames[user.role] || "用户"}</Badge>{user.created_at && <span className="text-xs text-muted-foreground">加入于 {new Date(user.created_at).toLocaleDateString("zh-CN")}</span>}</div></div></div>
        <div className="flex flex-wrap gap-2"><Button variant="outline" nativeButton={false} role="link" render={<Link href="/account/profile" />}><UserRound data-icon="inline-start" />编辑资料</Button><Button variant="outline" nativeButton={false} role="link" render={<Link href="/account/security" />}><ShieldCheck data-icon="inline-start" />安全设置</Button></div>
      </CardContent>
    </Card>
    <section aria-labelledby="my-apps-title" className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-4"><div><h2 id="my-apps-title" className="text-lg font-semibold">我的应用</h2><p className="mt-1 text-sm text-muted-foreground">使用同一个账户，访问已为你开放的应用。</p></div><InputGroup className="h-10 w-full sm:w-60"><InputGroupAddon><Search /></InputGroupAddon><InputGroupInput aria-label="搜索我的应用" placeholder="搜索应用" value={search} onChange={event => setSearch(event.target.value)} /></InputGroup></div>
      {error ? <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}<Button variant="link" onClick={load}>重新加载</Button></AlertDescription></Alert> : loading ? <div className="grid gap-4 sm:grid-cols-2"><Skeleton className="h-44 w-full" /><Skeleton className="h-44 w-full" /></div> : visible.length ? <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{visible.map(app => <Card key={app.id} interactive>
        <CardHeader className="flex flex-row items-center gap-3"><ApplicationIcon url={app.icon_url} name={app.name} /><CardTitle className="min-w-0 break-words">{app.name}</CardTitle></CardHeader>
        <CardContent className="flex-1"><p className="break-words text-sm leading-relaxed text-muted-foreground">{app.description || "使用 AlbaTextus 账户登录此应用。"}</p></CardContent>
        <CardFooter>{app.homepage_url ? <Button variant="outline" nativeButton={false} role="link" render={<a href={app.homepage_url} target="_blank" rel="noopener noreferrer" />} aria-label={`打开 ${app.name}（新窗口）`}>打开应用<ArrowUpRight data-icon="inline-end" /></Button> : <p className="text-xs text-muted-foreground">访问入口尚未配置，请联系管理员。</p>}</CardFooter>
      </Card>)}</div> : <Card><CardContent><Empty className="py-12"><EmptyHeader><EmptyMedia variant="icon"><LayoutGrid /></EmptyMedia><EmptyTitle>{apps.length ? "没有找到匹配的应用" : "暂无可用应用"}</EmptyTitle><EmptyDescription>{apps.length ? "试试其他名称或清除搜索条件。" : "管理员为你开放应用后，会显示在这里。"}</EmptyDescription></EmptyHeader>{apps.length > 0 && <EmptyContent><Button variant="outline" onClick={() => setSearch("")}>清除搜索</Button></EmptyContent>}</Empty></CardContent></Card>}
    </section>
  </div>
}
