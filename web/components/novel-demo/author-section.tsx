"use client"

import { useEffect, useState } from "react"
import { Link } from "next-view-transitions"
import { ContentFailure, useNovelAccess } from "@/components/novel-access"
import { safeContentURL, type Novel } from "@/lib/athub-client"
import { readAllContent } from "@/lib/novel-demo-data"
import { DemoCover } from "./novel-cover"

export function AuthorSection({ novelId, authorId }: { novelId: string; authorId: number | null }) {
  const { request } = useNovelAccess()
  const [books, setBooks] = useState<Novel[]>([])
  const [loading, setLoading] = useState(authorId !== null)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    if (authorId === null) return
    const controller = new AbortController()
    setLoading(true)
    setError("")
    void readAllContent<Novel>(request, `/api/v2/novelinfo?authorId=${authorId}`, controller.signal)
      .then(items => {
        if (!controller.signal.aborted) {
          setBooks(items.filter(book => String(book.id) !== novelId))
        }
      })
      .catch(reason => {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "作者作品加载失败。")
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [request, authorId, novelId, revision])

  if (authorId === null || (!loading && !error && !books.length)) return null

  return <section className="panel novel-author-section" aria-label="同作者作品" aria-busy={loading}>
    <hr className="novel-author-divider" />
    {loading ? <p className="novel-author-status" role="status">正在加载作者作品…</p>
      : error ? <ContentFailure message={error} retry={() => setRevision(value => value + 1)} />
      : books.length ? <div className="library-books novel-author-books">
        {books.map(book => <article className="library-book" key={book.id}>
          <Link href={`/library/books/${book.id}`} className="library-cover-link" aria-label={`查看 ${book.title}`}>
            <DemoCover src={safeContentURL(book.coverUrl)} title={book.title} />
          </Link>
          <div className="library-book-body">
            <h3><Link href={`/library/books/${book.id}`} title={book.title}>{book.title}</Link></h3>
          </div>
        </article>)}
      </div> : null}
  </section>
}
