"use client"

import { useEffect, useState } from "react"
import { NovelDetail as DemoNovelDetail } from "@/components/novel-demo/novel-detail"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel-access"
import { mapDemoNovel, readAllContent, type DemoNovel } from "@/lib/novel-demo-data"
import type { Novel, NovelContributors, ContentAuthor, ContentTag, ContentPlatform, TableOfContents } from "@/lib/athub-client"

export function NovelDetail({ id }: { id: number }) {
  return <div className="novel-demo"><NovelAccessGate><DetailData key={id} id={id} /></NovelAccessGate></div>
}

function DetailData({ id }: { id: number }) {
  const { request } = useNovelAccess()
  const [novel, setNovel] = useState<DemoNovel | null>(null)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setNovel(null); setError("")
    async function load() {
      const [book, tags, toc, contributions] = await Promise.all([
        request<Novel>(`/api/v2/novelinfo?id=${id}`, controller.signal),
        readAllContent<ContentTag>(request, "/api/v2/tags", controller.signal),
        request<TableOfContents>(`/api/v2/toc?novelId=${id}`, controller.signal),
        request<NovelContributors>(`/api/v3/contributors?novelId=${id}`, controller.signal),
      ])
      const [platform, author] = await Promise.all([
        request<ContentPlatform>(`/api/v2/platforms?id=${book.platformId}`, controller.signal),
        book.authorId === null ? Promise.resolve(null) : request<ContentAuthor>(`/api/v2/authors?id=${book.authorId}`, controller.signal),
      ])
      if (!controller.signal.aborted) setNovel({ ...mapDemoNovel(book, tags, toc, platform, author), contributions })
    }
    void load().catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    })
    return () => controller.abort()
  }, [request, id, revision])
  if (error) return <div className="page-shell"><ContentFailure message={error} retry={() => setRevision(value => value + 1)} /></div>
  if (!novel) return <div className="page-shell novel-page-loading" role="status">正在加载作品…</div>
  return <DemoNovelDetail novel={novel} />
}
