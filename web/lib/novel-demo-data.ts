import { contentDate, novelStatus, safeContentURL, type ContentAuthor, type ContentPlatform, type ContentTag, type Novel, type TableOfContents } from "@/lib/athub-client"
import type { NovelContributors } from "@/lib/athub-client"

// API mapping only. The view components retain demo-2's original markup.
export type DemoLibraryBook = {
  id: string
  title: string
  author: string
  authorId: number | null
  status: string
  words: string
  updatedAt: string
  tags: string[]
  synopsis: string
  chapterCount: number | null
  cover: string | null
}
export type DemoNovel = Omit<DemoLibraryBook, "synopsis"> & {
  synopsis: string[]
  language: string
  publisher?: string
  chapters: TableOfContents["chapters"]
  volumes: TableOfContents["volumes"]
  contributions?: NovelContributors
}

export function mapDemoBook(book: Novel, tags: ContentTag[], authors: ContentAuthor[] = []): DemoLibraryBook {
  const names = new Map(tags.map(tag => [tag.id, tag.name]))
  const author = authors.find(item => item.id === book.authorId && item.platformId === book.platformId)
  return {
    id: String(book.id),
    title: book.title,
    author: author?.name?.trim() || (book.authorId === null ? "待补充" : `作者 #${book.authorId}`),
    authorId: book.authorId,
    status: novelStatus(book.status),
    words: "待补充",
    updatedAt: contentDate(book.updatedAt),
    tags: (book.tagIds || []).map(id => names.get(id) || `标签 #${id}`),
    synopsis: book.summary || "暂无简介",
    chapterCount: null,
    cover: safeContentURL(book.coverUrl),
  }
}

export function mapDemoNovel(book: Novel, tags: ContentTag[], toc: TableOfContents | null, platform: ContentPlatform | null, author: ContentAuthor | null = null): DemoNovel {
  const synopsis = book.summary?.split(/\r?\n/).map(line => line.trim()).filter(Boolean) || []
  return {
    ...mapDemoBook(book, tags, author ? [author] : []),
    synopsis: synopsis.length ? synopsis : ["暂无简介"],
    language: "待补充",
    publisher: platform?.name,
    words: typeof toc?.wordCount === "number" ? `${Number((toc.wordCount / 10000).toFixed(2))}w` : "待补充",
    chapterCount: toc?.chapters.length ?? null,
    chapters: toc?.chapters || [],
    volumes: toc?.volumes || [],
  }
}

export async function readAllContent<T>(request: <R>(path: string, signal?: AbortSignal) => Promise<R>, path: string, signal: AbortSignal): Promise<T[]> {
  const items: T[] = []
  for (let offset = 0; ; offset += 100) {
    const page = await request<T[]>(`${path}${path.includes("?") ? "&" : "?"}limit=100&offset=${offset}`, signal)
    if (!Array.isArray(page)) throw new Error("小说服务返回的数据格式不正确。")
    items.push(...page)
    if (page.length < 100) return items
    signal.throwIfAborted()
  }
}
