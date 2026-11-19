"use client"

import { useEffect, useState } from "react"
import { NovelDetailView } from "@/components/novel/novel-detail-view"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel/novel-access"
import { mapNovelDetailModel, readAllContent, type NovelDetailModel } from "@/lib/novel/novel-view-models"
import type { Novel, NovelContributors, ContentAuthor, ContentTag, ContentPlatform, TableOfContents } from "@/lib/athub/athub-client"
import { NovelEdit, NovelEditorProvider } from "@/components/novel/novel-inline-editor"

export function NovelDetail({ id }: { id: number }) {
  return <div className="novel-demo"><NovelAccessGate><DetailData key={id} id={id} /></NovelAccessGate></div>
}

function DetailData({ id }: { id: number }) {
  const { request } = useNovelAccess()
  const [novel, setNovel] = useState<NovelDetailModel | null>(null)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const [catalogLoading, setCatalogLoading] = useState(true)
  const [catalogError, setCatalogError] = useState("")
  const [partialError, setPartialError] = useState("")
  useEffect(() => {
    const controller = new AbortController()
    setError(""); setPartialError(""); setCatalogError(""); setCatalogLoading(true)
    async function load() {
      const book = await request<Novel>(`/api/v5/novelinfo?id=${id}`, controller.signal)
      let tags: ContentTag[] = [], toc: TableOfContents | null = null
      let platform: ContentPlatform | null = null, author: ContentAuthor | null = null
      let contributions: NovelContributors | undefined
      const publish = () => {
        if (!controller.signal.aborted) setNovel({ ...mapNovelDetailModel(book, tags, toc, platform, author), contributions })
      }
      publish()
      const sections = [
        readAllContent<ContentTag>(request, "/api/v5/tags", controller.signal).then(value => { tags = value; publish() }),
        request<ContentPlatform>(`/api/v5/platforms?id=${book.platformId}`, controller.signal).then(value => { platform = value; publish() }),
        book.authorId === null ? Promise.resolve() : request<ContentAuthor>(`/api/v5/authors?id=${book.authorId}`, controller.signal).then(value => { author = value; publish() }),
        request<NovelContributors>(`/api/v4/contributors?novelId=${id}`, controller.signal).then(value => { contributions = value; publish() }),
        request<TableOfContents>(`/api/v5/toc?novelId=${id}`, controller.signal)
          .then(value => { toc = value; publish() })
          .catch(reason => {
            if (!controller.signal.aborted) setCatalogError(reason instanceof Error ? reason.message : "目录加载失败。")
            throw reason
          }).finally(() => { if (!controller.signal.aborted) setCatalogLoading(false) }),
      ]
      const results = await Promise.allSettled(sections)
      if (!controller.signal.aborted && results.some(result => result.status === "rejected")) setPartialError("部分作品资料加载失败，请重试。")
    }
    void load().catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    })
    return () => controller.abort()
  }, [request, id, revision])
  if (error) return <div className="page-shell"><ContentFailure message={error} retry={() => setRevision(value => value + 1)} /></div>
  if (!novel) return <div className="page-shell novel-page-loading" role="status">正在加载作品…</div>
  return <>
    {partialError && <div className="page-shell"><ContentFailure message={partialError} retry={() => setRevision(value => value + 1)} /></div>}
    <NovelEditorProvider id={id}>
      <NovelDetailView novel={novel} catalogLoading={catalogLoading} catalogError={catalogError} actions={<NovelEdit />} />
    </NovelEditorProvider>
  </>
}
