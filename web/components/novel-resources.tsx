"use client"

import { useEffect, useState } from "react"
import { BookOpen, Building2, Tags, UsersRound } from "lucide-react"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel-access"

const resources = [
  { path: "/api/v5/novelinfo", title: "小说", icon: BookOpen, tone: "blue" },
  { path: "/api/v5/tags", title: "标签", icon: Tags, tone: "violet" },
  { path: "/api/v5/authors", title: "作者", icon: UsersRound, tone: "teal" },
  { path: "/api/v5/platforms", title: "发行方", icon: Building2, tone: "pink" },
] as const

export function NovelResources() {
  return <div className="novel-demo"><div className="page-shell novel-resources-page">
    <header className="library-heading"><h1>资源</h1></header>
    <NovelAccessGate><ResourceCounts /></NovelAccessGate>
  </div></div>
}

function ResourceCounts() {
  const { request } = useNovelAccess()
  const [counts, setCounts] = useState<number[] | null>(null)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setError("")
    Promise.all(resources.map(async resource =>
      (await request<{ total: number }>(`${resource.path}?view=count`, controller.signal)).total,
    )).then(values => {
      if (!controller.signal.aborted) setCounts(values)
    }).catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    })
    return () => controller.abort()
  }, [request, revision])

  if (error) return <ContentFailure message={error} retry={() => setRevision(value => value + 1)} />
  if (!counts) return <div className="novel-loading novel-page-loading" role="status">正在加载资源…</div>

  return <dl className="novel-resource-counts novel-content-ready" aria-label="书库资源统计">
    {resources.map((resource, index) => <div className="panel novel-resource-count" data-tone={resource.tone} key={resource.path}>
      <dt><resource.icon aria-hidden="true" />{resource.title}</dt>
      <dd>{counts[index].toLocaleString("zh-CN")}</dd>
    </div>)}
  </dl>
}
