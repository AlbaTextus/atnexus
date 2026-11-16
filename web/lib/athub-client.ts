export type Novel = {
  id: number
  platformId: number
  authorId: number | null
  title: string
  summary: string | null
  coverUrl: string | null
  status: "ONGOING" | "COMPLETED" | "ABANDONED"
  tagIds?: number[]
  createdAt: string
  updatedAt: string
}
export type ContentTag = { id: number; name: string }
export type NovelIllustration = { id: number; novelId: number; url: string; sha256: string; width: number; height: number; createdAt: string }
export type IllustrationBlock = { imageId: number; x: number; y: number; width: number; height: number }
export type IllustrationLayout = { novelId: number; revision: number; blocks: IllustrationBlock[] }
export type ContentAuthor = { id: number; platformId: number; name: string | null }
export type ContentPlatform = { id: number; name: string; url: string | null }
export type NovelContributor = { id: string; name: string; avatar: string; createdAt: string; edits: number }
export type NovelContributors = { creator: NovelContributor | null; contributors: NovelContributor[] }
export type ChapterBodyHunk = {
  beforeStart: number; afterStart: number; beforeLine: number; afterLine: number
  parts: { kind: "equal" | "delete" | "insert"; text: string }[]
}
export type NovelChange = { entity: string; action: string; before: Record<string, unknown> | null; after: Record<string, unknown> | null; bodyDiff?: ChapterBodyHunk[] }
export type NovelRevision = {
  id: string; novelId: number
  contributor: { id: string; name: string; avatar: string }
  createdAt: string; startedAt?: string; revisionCount?: number; changes: NovelChange[]
}
export type NovelHistory = { revisions: NovelRevision[]; total: number }
export type ChapterSummary = { id: number; novelId: number; volumeId: number | null; order: number; title: string; updatedAt: string }
export type Chapter = ChapterSummary & { content: string }
export type TableOfContents = {
  novelId: number
  wordCount?: number
  volumes: { id: number; order: number; title: string }[]
  chapters: ChapterSummary[]
}

// The account backend supplies a short-lived, read-only session credential.
export const contentApiURL = (process.env.NEXT_PUBLIC_ATHUB_API_URL || "https://api.albatextus.com").replace(/\/+$/, "")

export class ContentApiError extends Error {
  constructor(public status: number) {
    super(status === 401 ? "小说库访问已失效，或已被管理员禁用。" :
      status === 403 ? "当前账户无权访问小说库。" :
      status === 404 ? "没有找到这项内容。" :
      status === 429 ? "请求过于频繁，请稍后重试。" : "小说服务暂时不可用，请稍后重试。")
  }
}

export async function fetchContent<T>(path: string, credential: string, signal?: AbortSignal): Promise<T> {
  if (!path.startsWith("/api/v2/") && !/^\/api\/v3\/(history|contributors)\?/.test(path)) throw new Error("无效的内容接口路径。")
  let response: Response
  try {
    response = await fetch(`${contentApiURL}${path}`, {
      method: "GET",
      headers: { Authorization: `Bearer ${credential}` },
      credentials: "omit",
      cache: "no-store",
      referrerPolicy: "no-referrer",
      redirect: "error",
      signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(20000)]) : AbortSignal.timeout(20000),
    })
  } catch (error) {
    if (signal?.aborted) throw error
    throw new Error("无法连接小说服务，请检查网络与服务地址。")
  }
  if (!response.ok) throw new ContentApiError(response.status)
  if (!response.headers.get("content-type")?.includes("application/json")) throw new Error("小说服务返回了非 JSON 响应。")
  return response.json() as Promise<T>
}

export function novelStatus(status: Novel["status"]) {
  const labels: Record<Novel["status"], string> = {
    ONGOING: "连载中",
    COMPLETED: "已完结",
    ABANDONED: "太监",
  }
  return labels[status]
}

export function contentDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "未提供" : date.toISOString().slice(0, 10)
}

export function safeContentURL(value: string | null | undefined) {
  if (!value) return null
  try {
    const url = new URL(value)
    return (url.protocol === "https:" || url.protocol === "http:") && !url.username && !url.password ? url.href : null
  } catch { return null }
}
