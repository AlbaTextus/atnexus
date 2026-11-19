"use client";

import { useEffect, useState } from "react";
import { NovelCover } from "./novel-cover";
import { Link } from "next-view-transitions";
import { AnimatePresence, MotionConfig, motion, useReducedMotion } from "framer-motion";
import {
  ArrowDownWideNarrow,
  ArrowRight,
  BookOpen,
  History,
  Library,
  Search,
  Sparkles,
  UserRound,
  X,
} from "lucide-react";

import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "./ui/empty";
import type { NovelListItem } from "@/lib/novel/novel-view-models";

type LibraryPageProps = {
  books: NovelListItem[];
  featuredBooks?: NovelListItem[];
  tagId?: number;
  tagName?: string;
  authorId?: number;
  authorName?: string;
  total: number;
  page: number;
  query: string;
  sort: "updated" | "title";
  pending: boolean;
  failed?: boolean;
  onQuery: (value: string) => void;
  onSort: (value: "updated" | "title") => void;
  onPage: (value: number) => void;
};

export function LibraryPage({
  books,
  featuredBooks = [],
  tagId,
  tagName,
  authorId,
  authorName,
  total,
  page,
  query,
  sort,
  pending,
  failed,
  onQuery,
  onSort,
  onPage,
}: LibraryPageProps) {
  const reducedMotion = useReducedMotion();
  const [draft, setDraft] = useState(query);
  const [failedBackdrop, setFailedBackdrop] = useState<string | null>(null);

  useEffect(() => { setDraft(query); }, [query]);
  useEffect(() => {
    if (draft.trim() === query) return;
    const timer = setTimeout(() => onQuery(draft), 300);
    return () => clearTimeout(timer);
  }, [draft, query, onQuery]);

  const search = query.trim();
  const pageCount = Math.max(1, Math.ceil(total / 24));
  const currentPage = page;
  const isFiltered = Boolean(search || authorId || tagId || currentPage > 1);

  const heroPool = featuredBooks.length ? featuredBooks : books.slice(0, 12);
  const showPortal = !isFiltered && heroPool.length > 0;
  const activeHero = heroPool[0] || null;
  const rankingBooks = heroPool.slice(0, 5);

  const headingTitle = authorName
    ? authorName
    : tagName
      ? `标签：${tagName}`
      : "全部作品";

  const statusTone = (status: string) => {
    if (status === "已完结") return "completed";
    if (status === "太监") return "abandoned";
    return "ongoing";
  };

  return (
    <MotionConfig reducedMotion="user">
      <main className={`page-shell library-page novel-content-ready${showPortal ? " is-portal" : ""}`}>
        {showPortal && activeHero && (
          <section className="library-hero" aria-label="精选作品推荐">
            <div className="novel-cover-ambience library-hero-ambience" aria-hidden="true">
              {activeHero.cover && failedBackdrop !== activeHero.cover ? (
                <img
                  key={activeHero.cover}
                  src={activeHero.cover}
                  alt=""
                  decoding="async"
                  referrerPolicy="no-referrer"
                  onError={() => setFailedBackdrop(activeHero.cover)}
                />
              ) : (
                <div className="library-hero-ambience-fallback" />
              )}
            </div>

            <div className="library-hero-stage">
              <Link
                href={`/library/books/${encodeURIComponent(activeHero.id)}`}
                className="library-hero-cover"
                aria-label={`查看 ${activeHero.title}`}
              >
                <NovelCover src={activeHero.cover} title={activeHero.title} fallback="暂无封面" />
              </Link>

              <div className="library-hero-info">
                <div className="library-hero-meta">
                  <span>更新于 {activeHero.updatedAt}</span>
                  {activeHero.publisher && <span>{activeHero.publisher}</span>}
                  <span className="library-hero-status" data-tone={statusTone(activeHero.status)}>
                    {activeHero.status}
                  </span>
                </div>

                <h1 className="library-hero-title">
                  <Link href={`/library/books/${encodeURIComponent(activeHero.id)}`}>
                    {activeHero.title}
                  </Link>
                </h1>

                <div className="library-hero-subline">
                  {activeHero.authorId === null ? (
                    <span className="library-hero-author">{activeHero.author}</span>
                  ) : (
                    <Link className="library-hero-author is-link" href={`/library?authorId=${activeHero.authorId}`}>
                      {activeHero.author}
                    </Link>
                  )}
                  {activeHero.tags.slice(0, 3).map(tag => (
                    <span className="library-hero-tag" key={tag}>{tag}</span>
                  ))}
                </div>

                <p className="library-hero-synopsis">{activeHero.synopsis}</p>

                <div className="library-hero-actions">
                  <Link
                    href={`/library/books/${encodeURIComponent(activeHero.id)}`}
                    className="library-hero-btn-primary"
                  >
                    <BookOpen aria-hidden="true" />
                    查看详情
                  </Link>
                  {activeHero.authorId !== null && (
                    <Link
                      href={`/library?authorId=${activeHero.authorId}`}
                      className="library-hero-btn-secondary"
                    >
                      <UserRound aria-hidden="true" />
                      同作者作品
                    </Link>
                  )}
                  <Link
                    href={`/library/books/${encodeURIComponent(activeHero.id)}/history`}
                    className="library-hero-btn-secondary"
                  >
                    <History aria-hidden="true" />
                    修订记录
                  </Link>
                </div>
              </div>
            </div>
          </section>
        )}

        {isFiltered && (
          <>
            <header className="library-heading">
              <h1 id="library-title">{authorName || (tagName ? `标签：${tagName}` : "书库")}</h1>
              {(authorId || tagId || search || currentPage > 1) && (
                <Link href="/library" onClick={() => { setDraft(""); }}>返回书库首页</Link>
              )}
            </header>

            <form
              className="library-search"
              role="search"
              onSubmit={event => {
                event.preventDefault();
                onQuery(draft);
              }}
            >
              <Search aria-hidden="true" />
              <Input
                type="search"
                value={draft}
                maxLength={200}
                onChange={event => setDraft(event.target.value)}
                placeholder="搜索书名或作者"
                aria-label="搜索书名或作者"
              />
              {draft && (
                <button
                  type="button"
                  className="library-search-clear"
                  aria-label="清除搜索"
                  onClick={() => {
                    setDraft("");
                    onQuery("");
                  }}
                >
                  <X aria-hidden="true" />
                </button>
              )}
            </form>
          </>
        )}

        {showPortal && rankingBooks.length > 0 && (
          <section className="library-ranking" aria-label="近期更新榜单">
            <div className="library-ranking-header">
              <div className="library-ranking-title-group">
                <div className="library-ranking-badges" aria-hidden="true">
                  <span className="library-ranking-brand">atnexus</span>
                  <span className="library-ranking-pill-badge">RECENT</span>
                  <span className="library-ranking-pill-badge is-dark">NOVEL</span>
                </div>
                <h2 className="library-ranking-heading">
                  <span>精选更新</span>
                  <span className="library-ranking-exclaim" aria-hidden="true">///</span>
                </h2>
              </div>
              <a href="#all-books" className="library-ranking-more">
                完整书库 <ArrowRight aria-hidden="true" />
              </a>
            </div>

            <ol className="library-ranking-list">
              {rankingBooks.map((book, index) => (
                <li className="library-rank-card" key={book.id}>
                  {book.cover ? (
                    <div className="library-rank-ambience" aria-hidden="true">
                      <img src={book.cover} alt="" loading="lazy" referrerPolicy="no-referrer" />
                    </div>
                  ) : (
                    <div className="library-rank-ambience is-fallback" aria-hidden="true" />
                  )}

                  <Link
                    href={`/library/books/${encodeURIComponent(book.id)}`}
                    className="library-rank-overlay-link"
                    aria-label={`查看 ${book.title}`}
                  />

                  <div className="library-rank-num" data-rank={index + 1}>
                    <span>{index + 1}</span>
                    <small aria-hidden="true">—</small>
                  </div>

                  <div className="library-rank-cover">
                    <NovelCover src={book.cover} title={book.title} fallback="暂无" />
                  </div>

                  <div className="library-rank-body">
                    <h3>
                      <Link href={`/library/books/${encodeURIComponent(book.id)}`} title={book.title}>
                        {book.title}
                      </Link>
                    </h3>
                    <p className="library-rank-meta">
                      <span>{book.author}</span>
                      {book.publisher && <span> · {book.publisher}</span>}
                      <span> · {book.updatedAt}</span>
                    </p>
                    <div className="library-rank-tags">
                      <span className="library-rank-status" data-tone={statusTone(book.status)}>
                        <Sparkles aria-hidden="true" />
                        {book.status}
                      </span>
                      {book.tags.slice(0, 4).map(tag => (
                        <span className="library-rank-tag" key={tag}>{tag}</span>
                      ))}
                    </div>
                  </div>

                  <div className="library-rank-end">
                    <span className="library-rank-end-pill">
                      {book.tags.length > 0 ? `${book.tags.length} 标签 · ` : ""}
                      {book.status}
                    </span>
                  </div>
                </li>
              ))}
            </ol>
          </section>
        )}

        <div className="library-layout" id="all-books">
          <section className="library-results" aria-label="书籍列表" aria-busy={pending}>
            <div className="library-toolbar">
              <div className="library-toolbar-title">
                <h2>{search ? "搜索结果" : headingTitle}</h2>
                <p aria-live="polite">
                  {pending
                    ? "正在加载书籍…"
                    : failed
                      ? "书籍加载失败"
                      : <>共 <strong>{total}</strong> 部</>}
                </p>
              </div>
              <div className="library-tools">
                {!isFiltered && (
                  <form
                    className="library-search is-compact"
                    role="search"
                    onSubmit={event => {
                      event.preventDefault();
                      onQuery(draft);
                    }}
                  >
                    <Search aria-hidden="true" />
                    <Input
                      type="search"
                      value={draft}
                      maxLength={200}
                      onChange={event => setDraft(event.target.value)}
                      placeholder="搜索书名或作者"
                      aria-label="搜索书名或作者"
                    />
                  </form>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onSort(sort === "updated" ? "title" : "updated")}
                  aria-label={`排序：${sort === "updated" ? "最近更新，点击按书名排序" : "书名顺序，点击按最近更新排序"}`}
                >
                  <ArrowDownWideNarrow aria-hidden="true" />
                  {sort === "updated" ? "最近更新" : "书名顺序"}
                </Button>
              </div>
            </div>

            {books.length ? (
              <AnimatePresence mode="wait" initial={false}>
                <motion.div
                  className="library-books"
                  key={`${sort}:${currentPage}:${tagId || "all"}:${authorId || "all"}`}
                  initial={{ opacity: reducedMotion ? 1 : 0, y: reducedMotion ? 0 : 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: reducedMotion ? 1 : 0, y: reducedMotion ? 0 : -3 }}
                  transition={{ duration: reducedMotion ? 0 : 0.18, ease: [0.22, 1, 0.36, 1] }}
                >
                  {books.map(book => (
                    <motion.article
                      layout={reducedMotion ? false : "position"}
                      transition={{ layout: { duration: 0.28, ease: [0.22, 1, 0.36, 1] } }}
                      className="library-book"
                      key={book.id}
                    >
                      <Link
                        href={`/library/books/${encodeURIComponent(book.id)}`}
                        className="library-cover-link"
                        aria-label={`查看 ${book.title}`}
                      >
                        <NovelCover src={book.cover} title={book.title} fallback="暂无封面" />
                      </Link>
                      <div className="library-book-body">
                        <h3>
                          <Link href={`/library/books/${encodeURIComponent(book.id)}`} title={book.title}>
                            {book.title}
                          </Link>
                        </h3>
                        <p className="library-book-author" title={book.author}>
                          {book.authorId === null ? (
                            book.author
                          ) : (
                            <Link className="novel-author-link" href={`/library?authorId=${book.authorId}`}>
                              {book.author}
                            </Link>
                          )}
                        </p>
                      </div>
                    </motion.article>
                  ))}
                </motion.div>
              </AnimatePresence>
            ) : pending ? (
              <div className="novel-page-loading" role="status">正在加载书籍…</div>
            ) : failed ? null : (
              <div className="panel">
                <Empty>
                  <EmptyHeader>
                    <EmptyMedia variant="icon">{search ? <Search /> : <Library />}</EmptyMedia>
                    <EmptyTitle>
                      {search ? "没有找到匹配的作品" : authorName ? "该作者暂无作品" : tagName ? "该标签下暂无作品" : "书库还没有作品"}
                    </EmptyTitle>
                    {search ? (
                      <EmptyDescription>试试其他书名或作者。</EmptyDescription>
                    ) : (
                      !authorName && !tagName && <EmptyDescription>收录的作品将在这里展示。</EmptyDescription>
                    )}
                  </EmptyHeader>
                </Empty>
              </div>
            )}

            {(pageCount > 1 || currentPage > 1) && (
              <div className="pagination">
                <Button variant="outline" disabled={pending || currentPage === 1} onClick={() => onPage(currentPage - 1)}>
                  上一页
                </Button>
                <span>{currentPage} / {pageCount}</span>
                <Button variant="outline" disabled={pending || currentPage >= pageCount} onClick={() => onPage(currentPage + 1)}>
                  下一页
                </Button>
              </div>
            )}
          </section>
        </div>
      </main>
    </MotionConfig>
  );
}
