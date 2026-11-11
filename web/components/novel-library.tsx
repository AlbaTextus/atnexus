"use client"

import { useEffect, useState } from "react"
import { LibraryPage } from "@/components/novel-demo/library-page"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel-access"
import { mapDemoBook, readAllContent, type DemoLibraryBook } from "@/lib/novel-demo-data"
import { ContentApiError, type Novel, type ContentAuthor, type ContentTag, type TableOfContents } from "@/lib/athub-client"

type LibraryFilters = { tagId?: number; authorId?: number }

export function NovelLibrary({ tagId, authorId }: LibraryFilters) {
  return <div className="novel-demo"><NovelAccessGate><LibraryData key={`${tagId || "all"}:${authorId || "all"}`} tagId={tagId} authorId={authorId} /></NovelAccessGate></div>
}

function LibraryData({ tagId, authorId }: LibraryFilters) {
  const { request } = useNovelAccess()
  const [books, setBooks] = useState<DemoLibraryBook[] | null>(null)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const [authorName, setAuthorName] = useState<string>()
  useEffect(() => {
    const controller = new AbortController()
    setBooks(null); setError("")
    const filters = new URLSearchParams()
    if (tagId) filters.set("tagId", String(tagId))
    if (authorId) filters.set("authorId", String(authorId))
    Promise.all([
      readAllContent<Novel>(request, `/api/v2/novelinfo${filters.size ? `?${filters}` : ""}`, controller.signal),
      readAllContent<ContentTag>(request, "/api/v2/tags", controller.signal),
      authorId
        ? request<ContentAuthor>(`/api/v2/authors?id=${authorId}`, controller.signal).then(author => [author])
        : readAllContent<ContentAuthor>(request, "/api/v2/authors", controller.signal),
    ]).then(async ([novels, tags, authors]) => {
      const mapped = novels.map(book => mapDemoBook(book, tags, authors))
      // Keep the existing chapter-count field populated without changing the UI.
      // Bound parallel reads rather than starting one request per book at once.
      let next = 0
      await Promise.all(Array.from({ length: Math.min(4, mapped.length) }, async () => {
        while (next < mapped.length) {
          const index = next++
          try {
            const toc = await request<TableOfContents>(`/api/v2/toc?novelId=${novels[index].id}`, controller.signal)
            mapped[index].chapterCount = toc.chapters.length
          } catch (reason) {
            if (!(reason instanceof ContentApiError) || reason.status !== 404) throw reason
          }
        }
      }))
      if (!controller.signal.aborted) {
        setAuthorName(authorId ? authors[0]?.name?.trim() || `作者 #${authorId}` : undefined)
        setBooks(mapped)
      }
    }).catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    })
    return () => controller.abort()
  }, [request, tagId, authorId, revision])
  if (error) return <div className="page-shell"><ContentFailure message={error} retry={() => setRevision(value => value + 1)} /></div>
  if (!books) return <div className="page-shell novel-page-loading" role="status">正在加载书籍…</div>
  return <LibraryPage books={books} authorName={authorName} />
}
