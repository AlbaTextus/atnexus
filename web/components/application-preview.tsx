"use client"

import { Circle, CircleCheck, Eye, ShieldCheck } from "lucide-react"
import { ApplicationIcon } from "@/components/application-icon"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"

type PreviewDraft = { name: string; description: string; homepage_url: string; icon_url: string; callbacks: string; is_active: boolean }

function webAddress(value: string): URL | null {
  try {
    const url = new URL(value.trim())
    const local = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname)
    if (url.username || url.password || url.hash || /[\\\s*]/.test(value.trim())) return null
    return url.protocol === "https:" || (url.protocol === "http:" && local) ? url : null
  } catch { return null }
}

export function ApplicationPreview({ draft }: { draft: PreviewDraft }) {
  const homepage = webAddress(draft.homepage_url)
  const icon = webAddress(draft.icon_url)
  const callbacks = draft.callbacks.split(/\r?\n/).map(value => value.trim()).filter(Boolean)
  const checks = [
    { label: "应用名称", complete: Boolean(draft.name.trim()) },
    { label: "应用主页", complete: Boolean(homepage) },
    { label: "回调地址", complete: callbacks.length > 0 && callbacks.length <= 10 && callbacks.every(value => Boolean(webAddress(value))) },
  ]
  return <aside className="application-editor-preview" aria-label="应用预览">
    <Card>
      <CardHeader><CardTitle className="flex items-center gap-2"><Eye size={18} />实时预览</CardTitle><CardDescription>看看它在用户中心里的样子。</CardDescription></CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="application-preview-visual"><ApplicationIcon url={icon?.href} name={draft.name || "应用"} /></div>
        <div><p className="application-preview-title">{draft.name.trim() || "你的应用名称"}</p><p className="application-preview-description">{draft.description.trim() || "写一段简短介绍，让用户更了解你的应用。"}</p></div>
        <div className="flex flex-wrap gap-2"><Badge variant={draft.is_active ? "success" : "secondary"}>{draft.is_active ? "已启用" : "已停用"}</Badge><Badge variant="outline"><ShieldCheck data-icon="inline-start" />统一账户登录</Badge></div>
      </CardContent>
      <CardFooter><p className="min-w-0 break-all text-xs text-muted-foreground">{homepage?.host || "应用主页待填写"}</p></CardFooter>
    </Card>
    <Card>
      <CardHeader><CardTitle className="flex items-center justify-between gap-2">必填信息<Badge variant="secondary">{checks.filter(check => check.complete).length} / 3</Badge></CardTitle><CardDescription>填写完成后，保存应用即可接入。</CardDescription></CardHeader>
      <CardContent><ul className="application-preview-checks">{checks.map(check => <li key={check.label} data-complete={check.complete}><span>{check.label}</span>{check.complete ? <CircleCheck size={16} aria-label="已填写" /> : <Circle size={16} aria-label="待填写" />}</li>)}</ul></CardContent>
    </Card>
  </aside>
}
