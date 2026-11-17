"use client"

import { useCallback, useEffect, useState } from "react"
import { usePathname, useSearchParams } from "next/navigation"
import { LibraryPage } from "@/components/novel/library-page"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel/novel-access"
import { mapNovelListItem, type NovelListItem } from "@/lib/novel/novel-view-models"
import { type Novel, type ContentAuthor } from "@/lib/athub/athub-client"

type LibraryFilters = { tagId?: number; authorId?: number }
type LibraryResult = { books: NovelListItem[]; total: number; authorName?: string }
const pageSize = 24

export function NovelLibrary({ tagId, authorId }: LibraryFilters) {
  return <div className="novel-demo"><NovelAccessGate><LibraryData key={`${tagId || "all"}:${authorId || "all"}`} tagId={tagId} authorId={authorId} /></NovelAccessGate></div>
}

function LibraryData({ tagId, authorId }: LibraryFilters) {
  const { request } = useNovelAccess()
  const params = useSearchParams()
  const pathname = usePathname()
  const rawPage = Number(params.get("page") || 1)
  const page = Number.isSafeInteger(rawPage) && rawPage > 0 && rawPage <= 1_000_000 ? rawPage : 1
  const query = (params.get("q") || "").slice(0, 200)
  const sort = params.get("sort") === "title" ? "title" : "updated"
  const [result, setResult] = useState<LibraryResult | null>(null)
  const [pending, setPending] = useState(true)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const queryString = params.toString()
  const update = useCallback((values: Record<string, string>) => {
    const next = new URLSearchParams(queryString)
    for (const [name, value] of Object.entries(values)) {
      if (!value || (name === "page" && value === "1") || (name === "sort" && value === "updated")) next.delete(name)
      else next.set(name, value)
    }
    // Native history updates avoid another RSC/account lookup for every filter.
    window.history.replaceState(null, "", `${pathname}${next.size ? `?${next}` : ""}`)
  }, [pathname, queryString])
  const changeQuery = useCallback((value: string) => update({ q: value.trim(), page: "1" }), [update])
  useEffect(() => {
    const controller = new AbortController()
    setPending(true); setError("")
    const filters = new URLSearchParams({ view: "page", limit: String(pageSize), offset: String((page - 1) * pageSize), sort })
    if (query.trim()) filters.set("q", query.trim().normalize("NFKC"))
    if (tagId) filters.set("tagId", String(tagId))
    if (authorId) filters.set("authorId", String(authorId))
    async function load() {
      const data = await request<{ items: Novel[]; total: number }>(`/api/v5/novelinfo?${filters}`, controller.signal)
      controller.signal.throwIfAborted()
      const lastPage = Math.max(1, Math.ceil(data.total / pageSize))
      if (page > lastPage) { update({ page: String(lastPage) }); return }
      const ids = [...new Set([...data.items.flatMap(book => book.authorId === null ? [] : [book.authorId]), ...(authorId ? [authorId] : [])])].sort((a, b) => a - b)
      const authors = ids.length
        ? await request<ContentAuthor[]>(`/api/v5/authors?ids=${ids.join(",")}`, controller.signal)
        : []
      if (!controller.signal.aborted) {
        setResult({ books: data.items.map(book => mapNovelListItem(book, [], authors)), total: data.total,
          authorName: authorId ? authors.find(author => author.id === authorId)?.name?.trim() || `作者 #${authorId}` : undefined })
      }
    }
    void load().catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    }).finally(() => {
      if (!controller.signal.aborted) setPending(false)
    })
    return () => controller.abort()
  }, [request, tagId, authorId, page, query, sort, revision, update])
  return <>
    {error && <div className="page-shell"><ContentFailure message={error} retry={() => setRevision(value => value + 1)} /></div>}
    <LibraryPage books={result?.books || []} total={result?.total || 0} authorName={result?.authorName} page={page}
      query={query} sort={sort} pending={pending} failed={!!error} onQuery={changeQuery}
      onSort={value => update({ sort: value, page: "1" })} onPage={value => update({ page: String(value) })} />
  </>
}
