"use client"

import Link from "next/link"
import { useEffect, useId, useState } from "react"
import { Search } from "lucide-react"
import { integrationContent, integrationDescription, integrationTitle } from "@/lib/wiki-integration"
import { communitySearchPages } from "@/lib/wiki-community"

const pages = [{ title: "欢迎来到 ALBATEXTUS", href: "/wiki", content: "账户 用户中心 个人资料 头像 简介 登录 安全 通行密钥" }, ...communitySearchPages, { title: integrationTitle, href: "/wiki/integration", content: `${integrationDescription} ${integrationContent}` }, { title: "隐私政策", href: "/wiki/privacy", content: "隐私 数据 个人信息" }, { title: "用户协议", href: "/wiki/terms", content: "条款 服务 协议" }]
export function WikiSearch() {
  const id = useId()
  const [query, setQuery] = useState("")
  const [index, setIndex] = useState(pages)
  useEffect(() => {
    const controller = new AbortController()
    Promise.all(["privacy", "terms"].map(async slug => {
      const response = await fetch(`/api/v1/legal/${slug}`, { signal: controller.signal, cache: "no-store" })
      if (!response.ok) throw new Error("search index unavailable")
      return response.json()
    })).then(docs => { if (!controller.signal.aborted) setIndex(pages.map(page => ({ ...page, content: docs.find(doc => `/wiki/${doc.slug}` === page.href)?.content || page.content }))) }).catch(() => {})
    return () => controller.abort()
  }, [])
  const results = index.filter(page => `${page.title} ${page.content}`.toLowerCase().includes(query.trim().toLowerCase()))
  return <div className="wiki-search"><Search size={16} aria-hidden="true" /><input aria-label="搜索文档" aria-controls={id} placeholder="搜索文档…" value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === "Escape") setQuery("") }} />
    {query.trim() && <div id={id} className="wiki-search-results"><p role="status">{results.length ? `${results.length} 篇文档` : "没有找到结果"}</p>{results.map(page => <Link key={page.href} href={page.href} onClick={() => setQuery("")}>{page.title}</Link>)}</div>}
  </div>
}
