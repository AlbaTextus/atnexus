"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { usePathname, useSearchParams } from "next/navigation"
import { LibraryPage } from "@/components/novel/library-page"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel/novel-access"
import { mapNovelListItem, readAllContent } from "@/lib/novel/novel-view-models"
import { type ContentAuthor, type ContentPlatform, type ContentTag, type Novel } from "@/lib/athub/athub-client"

type LibraryFilters = { tagId?: number; authorId?: number }
type RawLibraryResult = { items: Novel[]; authors: ContentAuthor[]; total: number; authorName?: string }
type FeaturedRaw = { items: Novel[]; authors: ContentAuthor[] }
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
  const [result, setResult] = useState<RawLibraryResult | null>(null)
  const [featuredRaw, setFeaturedRaw] = useState<FeaturedRaw | null>(null)
  const [tags, setTags] = useState<ContentTag[]>([])
  const [platforms, setPlatforms] = useState<ContentPlatform[]>([])
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
    void Promise.allSettled([
      readAllContent<ContentTag>(request, "/api/v5/tags", controller.signal),
      readAllContent<ContentPlatform>(request, "/api/v5/platforms", controller.signal),
    ]).then(([tagResult, platformResult]) => {
      if (controller.signal.aborted) return
      if (tagResult.status === "fulfilled") setTags(tagResult.value)
      if (platformResult.status === "fulfilled") setPlatforms(platformResult.value)
    })
    return () => controller.abort()
  }, [request])

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
        const authorName = authorId ? authors.find(author => author.id === authorId)?.name?.trim() || `作者 #${authorId}` : undefined
        setResult({ items: data.items, authors, total: data.total, authorName })
        if (!query.trim() && !tagId && !authorId && page === 1 && sort === "updated") {
          setFeaturedRaw({ items: data.items.slice(0, 12), authors })
        }
      }
    }
    void load().catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    }).finally(() => {
      if (!controller.signal.aborted) setPending(false)
    })
    return () => controller.abort()
  }, [request, tagId, authorId, page, query, sort, revision, update])

  const books = useMemo(
    () => (result ? result.items.map(book => mapNovelListItem(book, tags, result.authors, platforms)) : []),
    [result, tags, platforms],
  )
  const featuredBooks = useMemo(() => {
    const source = featuredRaw ?? (result && !query.trim() && !tagId && !authorId && page === 1
      ? { items: result.items.slice(0, 12), authors: result.authors }
      : null)
    return source ? source.items.map(book => mapNovelListItem(book, tags, source.authors, platforms)) : []
  }, [featuredRaw, result, query, tagId, authorId, page, tags, platforms])

  const tagName = tagId ? tags.find(tag => tag.id === tagId)?.name || `标签 #${tagId}` : undefined

  return <>
    {error && <div className="page-shell"><ContentFailure message={error} retry={() => setRevision(value => value + 1)} /></div>}
    <LibraryPage
      books={books}
      featuredBooks={featuredBooks}
      tagId={tagId}
      tagName={tagName}
      authorId={authorId}
      authorName={result?.authorName}
      total={result?.total || 0}
      page={page}
      query={query}
      sort={sort}
      pending={pending}
      failed={!!error}
      onQuery={changeQuery}
      onSort={value => update({ sort: value, page: "1" })}
      onPage={value => update({ page: String(value) })}
    />
  </>
}
