"use client"

import { useState } from "react"
import { Link } from "next-view-transitions"
import { Tags } from "lucide-react"
import { Button } from "@/components/novel-demo/ui/button"
import { Empty, EmptyHeader, EmptyTitle } from "@/components/novel-demo/ui/empty"
import { NovelAccessGate, ContentFailure, useContentResource } from "@/components/novel-access"
import { safeContentURL, type ContentTag, type ContentPlatform } from "@/lib/athub-client"

export function NovelDirectory({ kind }: { kind: "tags" | "platforms" }) {
  return <div className="novel-demo"><div className="page-shell"><h1>{kind === "tags" ? "分类" : "数据来源"}</h1><NovelAccessGate><Directory key={kind} kind={kind} /></NovelAccessGate></div></div>
}

function Directory({ kind }: { kind: "tags" | "platforms" }) {
  const [page, setPage] = useState(0)
  const result = useContentResource<(ContentTag | ContentPlatform)[]>(`/api/v2/${kind}?limit=25&offset=${page * 24}`)
  if (result.error) return <ContentFailure message={result.error} retry={result.retry} />
  if (!result.data) return <p role="status">正在加载…</p>
  return <>
    <div className="novel-directory">{result.data.slice(0, 24).map(item => <article className="panel" key={item.id}>
      <h2>{kind === "tags" ? <Link href={`/library?tagId=${item.id}`}><Tags aria-hidden="true" />{item.name}</Link> : item.name}</h2>
      {"url" in item && safeContentURL(item.url) && <a href={safeContentURL(item.url)!} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">访问来源</a>}
    </article>)}</div>
    {!result.data.length && <Empty><EmptyHeader><EmptyTitle>{kind === "tags" ? "暂无标签" : "暂无数据来源"}</EmptyTitle></EmptyHeader></Empty>}
    <nav className="pagination" aria-label="目录分页"><Button variant="outline" disabled={page === 0} onClick={() => setPage(value => value - 1)}>上一页</Button><span>第 {page + 1} 页</span><Button variant="outline" disabled={result.data.length <= 24} onClick={() => setPage(value => value + 1)}>下一页</Button></nav>
  </>
}
