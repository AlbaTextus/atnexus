"use client";

import { useState } from "react";
import { DemoCover } from "./novel-cover";
import { Link } from "next-view-transitions";
import { AnimatePresence, MotionConfig, motion, useReducedMotion } from "framer-motion";
import { ArrowDownWideNarrow, ArrowRight, BookOpen, Feather, LayoutGrid, Library, List, Tags } from "lucide-react";

import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { ToggleGroup, ToggleGroupItem } from "./ui/toggle-group";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "./ui/empty";
import type { DemoLibraryBook } from "@/lib/novel-demo-data";

const navigation = [{ href: "/library", title: "书库", icon: Library }, { href: "/", title: "作品详情", icon: BookOpen }];

export function LibraryPage({ books, authorName }: { books: DemoLibraryBook[]; authorName?: string }) {
  const reducedMotion = useReducedMotion();
  const [sort, setSort] = useState<"updated" | "title">("updated");
  const [view, setView] = useState<"list" | "grid">("list");
  const [page, setPage] = useState(1);
  const tags = [...new Set(books.flatMap(book => book.tags))];
  const ordered = [...books].sort((a, b) => sort === "title" ? a.title.localeCompare(b.title, "zh-CN") : b.updatedAt.localeCompare(a.updatedAt));
  const pageCount = Math.max(1, Math.ceil(ordered.length / 12));
  const currentPage = Math.min(page, pageCount);

  return <MotionConfig reducedMotion="user">

    <main className="page-shell library-page novel-content-ready">
      <section className="panel library-intro" aria-labelledby="library-title">
        <div className="library-intro-copy">
          <span className="library-eyebrow"><Library aria-hidden="true" /> ALBATEXTUS LIBRARY</span>
          <h1 id="library-title">{authorName || "书库"}</h1>
          <p>{authorName ? <Link href="/library">返回全部作品</Link> : "在文字里，遇见另一个世界。"}</p>
        </div>
        <div className="library-totals" aria-label="书库概况">
          <div><BookOpen aria-hidden="true" /><strong>{books.length}<small>部</small></strong><span>收录作品</span></div>
          <div><Feather aria-hidden="true" /><strong>{new Set(books.flatMap(book => book.authorId === null ? [] : [book.authorId])).size}<small>位</small></strong><span>创作作者</span></div>
          <div><Tags aria-hidden="true" /><strong>{tags.length}<small>个</small></strong><span>故事标签</span></div>
        </div>
      </section>

      <div className="library-layout">
        <section className="library-results" aria-label="书籍列表">
          <div className="library-toolbar">
            <p aria-live="polite">全部作品 <strong>{ordered.length}</strong> 部</p>
            <div className="library-tools">
              <Button variant="ghost" size="sm" onClick={() => setSort(sort === "updated" ? "title" : "updated")} aria-label={`排序：${sort === "updated" ? "最近更新，点击按书名排序" : "书名顺序，点击按最近更新排序"}`}>
                <ArrowDownWideNarrow aria-hidden="true" />{sort === "updated" ? "最近更新" : "书名顺序"}
              </Button>
              <ToggleGroup type="single" spacing={1} size="sm" value={view} onValueChange={value => { if (value === "list" || value === "grid") setView(value); }} className="library-view-switch" aria-label="展示方式">
                <ToggleGroupItem value="list" aria-label="列表视图"><List /></ToggleGroupItem>
                <ToggleGroupItem value="grid" aria-label="网格视图"><LayoutGrid /></ToggleGroupItem>
              </ToggleGroup>
            </div>
          </div>
          {ordered.length ? <AnimatePresence mode="wait" initial={false}><motion.div className="library-books" data-view={view} key={`${view}:${currentPage}`}
            initial={{ opacity: reducedMotion ? 1 : 0, y: reducedMotion ? 0 : 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: reducedMotion ? 1 : 0, y: reducedMotion ? 0 : -3 }}
            transition={{ duration: reducedMotion ? 0 : 0.18, ease: [0.22, 1, 0.36, 1] }}>
            {ordered.slice((currentPage - 1) * 12, currentPage * 12).map(book => <motion.article layout={reducedMotion ? false : "position"} transition={{ layout: { duration: 0.28, ease: [0.22, 1, 0.36, 1] } }} className="panel library-book" key={book.id}>
              <Link href={`/library/books/${encodeURIComponent(book.id)}`} className="library-cover-link" aria-label={`查看 ${book.title}`}>
                <DemoCover src={book.cover} title={book.title}/>
              </Link>
              <div className="library-book-body">
                <div className="library-book-top"><Badge variant="secondary">{book.status}</Badge><time dateTime={book.updatedAt}>{book.updatedAt.slice(0, 10)} 更新</time></div>
                <h2><Link href={`/library/books/${encodeURIComponent(book.id)}`}>{book.title}</Link></h2>
                <p className="library-book-author"><Feather aria-hidden="true" />{book.authorId === null ? book.author : <Link className="novel-author-link" href={`/library?authorId=${book.authorId}`}>{book.author}</Link>}</p>
                <p className="library-book-synopsis">{book.synopsis}</p>
                <div className="book-tags"><div className="tag-cloud">{book.tags.map(tag => <Badge key={tag} variant="secondary">{tag}</Badge>)}</div></div>
                <div className="library-book-footer"><span>{book.words} <span aria-hidden="true">·</span> {book.chapterCount === null ? "—" : book.chapterCount} 章</span><Link href={`/library/books/${encodeURIComponent(book.id)}`}>查看详情<ArrowRight aria-hidden="true" /></Link></div>
              </div>
            </motion.article>)}
          </motion.div></AnimatePresence> : <div className="panel"><Empty><EmptyHeader><EmptyMedia variant="icon"><Library /></EmptyMedia><EmptyTitle>{authorName ? "该作者暂无作品" : "书库还没有作品"}</EmptyTitle>{!authorName && <EmptyDescription>收录的作品将在这里展示。</EmptyDescription>}</EmptyHeader></Empty></div>}
          {pageCount > 1 && <div className="pagination"><Button variant="outline" disabled={currentPage === 1} onClick={() => setPage(currentPage - 1)}>上一页</Button><span>{currentPage} / {pageCount}</span><Button variant="outline" disabled={currentPage === pageCount} onClick={() => setPage(currentPage + 1)}>下一页</Button></div>}
        </section>
      </div>
    </main>
  </MotionConfig>;
}
