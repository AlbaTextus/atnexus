"use client";

import { useEffect, useState } from "react";
import { DemoCover } from "./novel-cover";
import { Link } from "next-view-transitions";
import { AnimatePresence, MotionConfig, motion, useReducedMotion } from "framer-motion";
import { ArrowDownWideNarrow, Library, Search } from "lucide-react";

import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "./ui/empty";
import type { DemoLibraryBook } from "@/lib/novel-demo-data";

export function LibraryPage({ books, authorName, total, page, query, sort, pending, failed, onQuery, onSort, onPage }: {
  books: DemoLibraryBook[]; authorName?: string; total: number; page: number; query: string;
  sort: "updated" | "title"; pending: boolean; failed?: boolean; onQuery: (value: string) => void;
  onSort: (value: "updated" | "title") => void; onPage: (value: number) => void;
}) {
  const reducedMotion = useReducedMotion();
  const [draft, setDraft] = useState(query);
  useEffect(() => { setDraft(query); }, [query]);
  useEffect(() => {
    if (draft.trim() === query) return;
    const timer = setTimeout(() => onQuery(draft), 300);
    return () => clearTimeout(timer);
  }, [draft, query, onQuery]);
  const search = query.trim();
  const pageCount = Math.max(1, Math.ceil(total / 24));
  const currentPage = page;

  return <MotionConfig reducedMotion="user">

    <main className="page-shell library-page novel-content-ready">
      <header className="library-heading">
        <h1 id="library-title">{authorName || "书库"}</h1>
        {authorName && <Link href="/library">返回全部作品</Link>}
      </header>

      <form className="library-search" role="search" onSubmit={event => { event.preventDefault(); onQuery(draft); }}>
        <Search aria-hidden="true" />
        <Input type="search" value={draft} maxLength={200} onChange={event => setDraft(event.target.value)} placeholder="搜索书名或作者" aria-label="搜索书名或作者" />
      </form>

      <div className="library-layout">
        <section className="library-results" aria-label="书籍列表" aria-busy={pending}>
          <div className="library-toolbar">
            <p aria-live="polite">{pending ? "正在加载书籍…" : failed ? "书籍加载失败" : <>{search ? "搜索结果" : "全部作品"} <strong>{total}</strong> 部</>}</p>
            <div className="library-tools">
              <Button variant="ghost" size="sm" onClick={() => onSort(sort === "updated" ? "title" : "updated")} aria-label={`排序：${sort === "updated" ? "最近更新，点击按书名排序" : "书名顺序，点击按最近更新排序"}`}>
                <ArrowDownWideNarrow aria-hidden="true" />{sort === "updated" ? "最近更新" : "书名顺序"}
              </Button>
            </div>
          </div>
          {books.length ? <AnimatePresence mode="wait" initial={false}><motion.div className="library-books" key={`${sort}:${currentPage}`}
            initial={{ opacity: reducedMotion ? 1 : 0, y: reducedMotion ? 0 : 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: reducedMotion ? 1 : 0, y: reducedMotion ? 0 : -3 }}
            transition={{ duration: reducedMotion ? 0 : 0.18, ease: [0.22, 1, 0.36, 1] }}>
            {books.map(book => <motion.article layout={reducedMotion ? false : "position"} transition={{ layout: { duration: 0.28, ease: [0.22, 1, 0.36, 1] } }} className="library-book" key={book.id}>
              <Link href={`/library/books/${encodeURIComponent(book.id)}`} className="library-cover-link" aria-label={`查看 ${book.title}`}>
                <DemoCover src={book.cover} title={book.title}/>
              </Link>
              <div className="library-book-body">
                <h2><Link href={`/library/books/${encodeURIComponent(book.id)}`} title={book.title}>{book.title}</Link></h2>
                <p className="library-book-author" title={book.author}>{book.authorId === null ? book.author : <Link className="novel-author-link" href={`/library?authorId=${book.authorId}`}>{book.author}</Link>}</p>
              </div>
            </motion.article>)}
          </motion.div></AnimatePresence> : pending ? <div className="novel-page-loading" role="status">正在加载书籍…</div> : failed ? null : <div className="panel"><Empty><EmptyHeader><EmptyMedia variant="icon">{search ? <Search /> : <Library />}</EmptyMedia><EmptyTitle>{search ? "没有找到匹配的作品" : authorName ? "该作者暂无作品" : "书库还没有作品"}</EmptyTitle>{search ? <EmptyDescription>试试其他书名或作者。</EmptyDescription> : !authorName && <EmptyDescription>收录的作品将在这里展示。</EmptyDescription>}</EmptyHeader></Empty></div>}
          {(pageCount > 1 || currentPage > 1) && <div className="pagination"><Button variant="outline" disabled={pending || currentPage === 1} onClick={() => onPage(currentPage - 1)}>上一页</Button><span>{currentPage} / {pageCount}</span><Button variant="outline" disabled={pending || currentPage >= pageCount} onClick={() => onPage(currentPage + 1)}>下一页</Button></div>}
        </section>
      </div>
    </main>
  </MotionConfig>;
}
