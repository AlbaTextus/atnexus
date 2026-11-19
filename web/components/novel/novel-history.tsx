"use client"

import { Suspense, useCallback, useEffect, useId, useMemo, useRef, useState } from "react"
import { usePathname, useSearchParams } from "next/navigation"
import { Link } from "next-view-transitions"
import {
  ArrowLeft, Check, ChevronDown, ChevronLeft, ChevronRight, ChevronsDownUp, ChevronsUpDown,
  Code, Copy, GitCommitHorizontal, History, MoreHorizontal,
} from "lucide-react"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel/novel-access"
import { ContributorAvatar } from "@/components/novel/contributors-card"
import {
  UnifiedDiffCard,
  buildBodyHunksModel,
  buildTextDiffModel,
  type UnifiedDiffModel,
} from "@/components/novel/novel-diff-card"
import { NovelAutoHeight } from "@/components/novel/novel-motion"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Novel, NovelChange, NovelHistory, NovelRevision, NovelRevisionChanges } from "@/lib/athub/athub-client"

const fields: Record<string, string> = {
  title: "标题", summary: "简介", coverUrl: "封面", status: "连载状态",
  platformId: "平台 ID", authorId: "作者 ID", authorName: "作者名称", platformName: "平台名称",
  tagIds: "标签 ID", tagNames: "标签", order: "顺序", volumeId: "所属卷",
  contentHash: "章节正文", contentLength: "正文字数",
  url: "图片链接", sha256: "图片校验值", width: "宽度", height: "高度",
  blocks: "图片块布局",
}
const actions: Record<string, string> = { INSERT: "新增", UPDATE: "修改", DELETE: "删除" }
const entities: Record<string, string> = { Novel: "小说条目", Volume: "卷", Chapter: "章节", Illustration: "插图", IllustrationLayout: "插图布局" }
const knownEntities = new Set(["Novel", "Volume", "Chapter", "Illustration", "IllustrationLayout"])
const novelFieldGroups: [string[], string][] = [
  [["title"], "标题"],
  [["summary"], "简介"],
  [["coverUrl"], "封面"],
  [["status"], "连载状态"],
  [["authorId", "authorName"], "作者"],
  [["platformId", "platformName"], "平台"],
  [["tagIds", "tagNames"], "标签"],
]
const redundantIdPairs: [string, string][] = [
  ["authorId", "authorName"],
  ["platformId", "platformName"],
  ["tagIds", "tagNames"],
]

function valueText(value: unknown): string {
  if (value === null || value === undefined || value === "") return "未设置"
  if (Array.isArray(value)) return value.map(valueText).join("、") || "无"
  if (value === "ONGOING") return "连载中"
  if (value === "COMPLETED") return "已完结"
  if (value === "ABANDONED") return "太监"
  return typeof value === "object" ? JSON.stringify(value) : String(value)
}

function diffFieldText(value: unknown): string {
  if (value === null || value === undefined || value === "") return ""
  if (Array.isArray(value)) {
    if (value.length === 0) return ""
    if (value.every(item => item === null || typeof item !== "object")) return value.map(valueText).join("、")
    return JSON.stringify(value, null, 2)
  }
  if (value === "ONGOING") return "连载中"
  if (value === "COMPLETED") return "已完结"
  if (value === "ABANDONED") return "太监"
  return typeof value === "object" ? JSON.stringify(value, null, 2) : String(value)
}

function titleOf(record?: Record<string, unknown> | null, fallback?: Record<string, unknown> | null): string {
  const raw = record?.title ?? fallback?.title
  return typeof raw === "string" ? raw.trim() : ""
}

function summarizeNovelUpdates(updates: NovelChange[]): string {
  const changedKeys = new Set<string>()
  for (const update of updates) {
    const before = update.before || {}, after = update.after || {}
    for (const key of Object.keys({ ...before, ...after })) {
      if (key !== "id" && key !== "novelId" && JSON.stringify(before[key]) !== JSON.stringify(after[key])) {
        changedKeys.add(key)
      }
    }
  }
  const labels: string[] = []
  const covered = new Set<string>()
  for (const [keys, label] of novelFieldGroups) {
    if (keys.some(key => changedKeys.has(key))) {
      labels.push(label)
      keys.forEach(key => covered.add(key))
    }
  }
  for (const key of changedKeys) {
    if (!covered.has(key)) labels.push(fields[key] || key)
  }
  if (labels.length === 0) return "修改小说资料"
  if (labels.length === 1) return `修改小说${labels[0]}`
  if (labels.length === 2) return `修改小说${labels[0]}与${labels[1]}`
  if (labels.length === 3) return `修改小说${labels[0]}、${labels[1]}与${labels[2]}`
  return `修改小说${labels.slice(0, 3).join("、")}等 ${labels.length} 项资料`
}

// total is the real number of such changes; items may be only a sample of them.
function summarizeNamedBatch(items: NovelChange[], unit: string, verb: string, total = items.length): string {
  const first = titleOf(items[0]?.after, items[0]?.before)
  if (total === 1) return first ? `${verb}${unit}「${first}」` : `${verb}${unit}`
  return first ? `${verb}「${first}」等 ${total} 个${unit}` : `${verb} ${total} 个${unit}`
}

function summarizeChapterUpdates(updates: NovelChange[], total = updates.length): string {
  if (total === 1 && updates.length === 1) {
    const before = updates[0].before || {}, after = updates[0].after || {}
    const title = titleOf(after, before)
    const target = title ? `章节「${title}」` : "章节"
    const bodyChanged = before.contentHash !== after.contentHash
    const titleChanged = before.title !== after.title
    const orderChanged = before.order !== after.order || before.volumeId !== after.volumeId
    if (bodyChanged && !titleChanged && !orderChanged) return `修改${target}正文`
    if (titleChanged && !bodyChanged && !orderChanged) return title ? `修改章节标题为「${title}」` : "修改章节标题"
    if (orderChanged && !bodyChanged && !titleChanged) return `调整${target}目录顺序`
    return `修改${target}`
  }
  const onlyOrder = updates.every(update => {
    const before = update.before || {}, after = update.after || {}
    const orderChanged = before.order !== after.order || before.volumeId !== after.volumeId
    return orderChanged && before.contentHash === after.contentHash && before.title === after.title
  })
  if (onlyOrder && total === updates.length) return `调整 ${total} 个章节的目录顺序`
  return summarizeNamedBatch(updates, "章节", "修改", total)
}

function summarizeRevision(item: NovelRevision): string {
  const { changes } = item
  if (!changes.length) return "提交变更"
  // Counts come from the server when it sent them, since changes may be a sample.
  const stats = item.changeStats
  const count = (entity: string, action: string, sample: NovelChange[]) => stats
    ? stats.find(stat => stat.entity === entity && stat.action === action)?.count ?? 0
    : sample.length
  const novelInsertCount = count("Novel", "INSERT", []) || 0

  const novelInserts = changes.filter(c => c.entity === "Novel" && c.action === "INSERT")
  const novelDeletes = changes.filter(c => c.entity === "Novel" && c.action === "DELETE")
  const novelUpdates = changes.filter(c => c.entity === "Novel" && c.action === "UPDATE")
  const volInserts = changes.filter(c => c.entity === "Volume" && c.action === "INSERT")
  const volUpdates = changes.filter(c => c.entity === "Volume" && c.action === "UPDATE")
  const volDeletes = changes.filter(c => c.entity === "Volume" && c.action === "DELETE")
  const chapInserts = changes.filter(c => c.entity === "Chapter" && c.action === "INSERT")
  const chapUpdates = changes.filter(c => c.entity === "Chapter" && c.action === "UPDATE")
  const chapDeletes = changes.filter(c => c.entity === "Chapter" && c.action === "DELETE")
  const illInserts = changes.filter(c => c.entity === "Illustration" && c.action === "INSERT")
  const illUpdates = changes.filter(c => c.entity === "Illustration" && c.action === "UPDATE")
  const illDeletes = changes.filter(c => c.entity === "Illustration" && c.action === "DELETE")
  const layoutChanges = changes.filter(c => c.entity === "IllustrationLayout")
  const others = changes.filter(c => !knownEntities.has(c.entity))

  const clauses: string[] = []
  const createdNovel = novelInserts.length > 0 || novelInsertCount > 0
  const volInsertN = count("Volume", "INSERT", volInserts), volUpdateN = count("Volume", "UPDATE", volUpdates), volDeleteN = count("Volume", "DELETE", volDeletes)
  const chapInsertN = count("Chapter", "INSERT", chapInserts), chapUpdateN = count("Chapter", "UPDATE", chapUpdates), chapDeleteN = count("Chapter", "DELETE", chapDeletes)
  const illInsertN = count("Illustration", "INSERT", illInserts), illUpdateN = count("Illustration", "UPDATE", illUpdates), illDeleteN = count("Illustration", "DELETE", illDeletes)

  if (createdNovel) {
    const imported: string[] = []
    if (volInsertN > 0) imported.push(`${volInsertN} 卷`)
    if (chapInsertN > 0) imported.push(`${chapInsertN} 章`)
    if (illInsertN > 0) imported.push(`${illInsertN} 张插图`)
    clauses.push(imported.length ? `创建小说条目，收录 ${imported.join("、")}` : "创建小说条目")
  }

  if (novelDeletes.length > 0) clauses.push("删除小说条目")
  else if (novelUpdates.length > 0) clauses.push(summarizeNovelUpdates(novelUpdates))

  if (!createdNovel && volInsertN > 0) clauses.push(summarizeNamedBatch(volInserts, "分卷", "新增", volInsertN))
  if (volUpdateN > 0) clauses.push(summarizeNamedBatch(volUpdates, "分卷", "修改", volUpdateN))
  if (volDeleteN > 0) clauses.push(summarizeNamedBatch(volDeletes, "分卷", "删除", volDeleteN))

  if (!createdNovel && chapInsertN > 0) clauses.push(summarizeNamedBatch(chapInserts, "章节", "新增", chapInsertN))
  if (chapUpdateN > 0) clauses.push(summarizeChapterUpdates(chapUpdates, chapUpdateN))
  if (chapDeleteN > 0) clauses.push(summarizeNamedBatch(chapDeletes, "章节", "删除", chapDeleteN))

  if (!createdNovel && illInsertN > 0) clauses.push(illInsertN === 1 ? "上传插图" : `上传 ${illInsertN} 张插图`)
  if (illUpdateN > 0) clauses.push("更新插图信息")
  if (illDeleteN > 0) clauses.push(illDeleteN === 1 ? "删除插图" : `删除 ${illDeleteN} 张插图`)

  if (layoutChanges.length > 0) {
    const lastLayout = layoutChanges[layoutChanges.length - 1]
    clauses.push(lastLayout.action === "DELETE" ? "重置插图布局" : "调整插图布局")
  }

  for (const other of others) {
    const text = `${actions[other.action] || other.action}${entities[other.entity] || other.entity}`
    if (!clauses.includes(text)) clauses.push(text)
  }

  return clauses.join("，") || `${changes.length} 处变更`
}

function formatRelativeCommitTime(iso: string, now: number): string {
  const timestamp = Date.parse(iso)
  if (Number.isNaN(timestamp)) return "提交"
  const elapsedSeconds = Math.floor((now - timestamp) / 1000)
  if (elapsedSeconds < 60) return "刚刚提交"
  const minutes = Math.floor(elapsedSeconds / 60)
  if (minutes < 60) return `${minutes} 分钟前提交`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前提交`
  const days = Math.floor(hours / 24)
  if (days === 1) return "昨天提交"
  if (days === 2) return "前天提交"
  if (days < 30) return `${days} 天前提交`
  const months = Math.floor(days / 30)
  if (months < 12) return `${months} 个月前提交`
  const years = Math.floor(days / 365)
  return `${years} 年前提交`
}

function formatExactTime(iso: string): string {
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString("zh-CN", { hour12: false })
}

type RevisionGroup = {
  key: string
  label: string
  items: NovelRevision[]
}

function groupRevisionsByDate(revisions: NovelRevision[]): RevisionGroup[] {
  const groups: RevisionGroup[] = []
  for (const item of revisions) {
    const date = new Date(item.createdAt)
    const valid = !Number.isNaN(date.getTime())
    const key = valid
      ? `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
      : "unknown"
    const label = valid
      ? date.toLocaleDateString("zh-CN", { year: "numeric", month: "long", day: "numeric" })
      : "未知日期"
    const last = groups[groups.length - 1]
    if (last && last.key === key) last.items.push(item)
    else groups.push({ key, label, items: [item] })
  }
  return groups
}

const HISTORY_PAGE_SIZE = 20
// Changes of one entry are read this many at a time once it is opened.
const REVISION_CHANGES_PAGE_SIZE = 100
const DIFF_CARDS_PAGE_SIZE = 10

type DiffCardCategory = "Novel" | "Volume" | "Chapter" | "Illustration" | "Other"

const categoryLabels: Record<DiffCardCategory, string> = {
  Novel: "小说资料",
  Volume: "分卷",
  Chapter: "章节",
  Illustration: "插图",
  Other: "其他",
}
const categoryOrder: DiffCardCategory[] = ["Novel", "Volume", "Chapter", "Illustration", "Other"]

function entityCategory(entity: string): DiffCardCategory {
  if (entity === "Novel") return "Novel"
  if (entity === "Volume") return "Volume"
  if (entity === "Chapter") return "Chapter"
  if (entity === "Illustration" || entity === "IllustrationLayout") return "Illustration"
  return "Other"
}

type DiffCardSpec = {
  key: string
  category: DiffCardCategory
  title: string
  subtitle: string
  action?: string
  build: () => UnifiedDiffModel
}

function buildChangeCards(change: NovelChange, changeIndex: number): DiffCardSpec[] {
  const before = change.before || {}, after = change.after || {}
  const rawChanged = Object.keys({ ...before, ...after }).filter(key =>
    key !== "id" && key !== "novelId" && key !== "contentHash" &&
    JSON.stringify(before[key]) !== JSON.stringify(after[key]))
  const changedSet = new Set(rawChanged)
  const changed = rawChanged.filter(key =>
    !redundantIdPairs.some(([idKey, nameKey]) => key === idKey && changedSet.has(nameKey)))

  const category = entityCategory(change.entity)
  const entityName = entities[change.entity] || change.entity
  const itemTitle = (change.entity === "Volume" || change.entity === "Chapter") ? titleOf(after, before) : ""
  const contextPath = itemTitle ? `${entityName} · ${itemTitle}` : entityName
  const bodyChanged = before.contentHash !== after.contentHash
  const cards: DiffCardSpec[] = []

  if (change.action === "INSERT" || change.action === "DELETE") {
    const isInsert = change.action === "INSERT"
    const source = isInsert ? after : before
    const scalarKeys = changed.filter(key => key !== "summary" && key !== "blocks")
    if (scalarKeys.length > 0 || (bodyChanged && !change.bodyDiff?.length)) {
      cards.push({
        key: `${changeIndex}-entity`,
        category,
        title: itemTitle || entityName,
        subtitle: itemTitle ? entityName : (actions[change.action] || change.action),
        action: change.action,
        build: () => {
          const rows = scalarKeys.map(key => `${fields[key] || key}: ${valueText(source[key])}`)
          if (bodyChanged && !change.bodyDiff?.length) {
            rows.push(`章节正文: ${isInsert ? "已添加正文内容" : "已删除正文内容"}`)
          }
          const text = rows.join("\n")
          return buildTextDiffModel(isInsert ? "" : text, isInsert ? text : "")
        },
      })
    }
    for (const richKey of ["summary", "blocks"] as const) {
      if (changedSet.has(richKey)) {
        const text = diffFieldText(source[richKey])
        if (text) {
          cards.push({
            key: `${changeIndex}-${richKey}`,
            category,
            title: fields[richKey] || richKey,
            subtitle: contextPath,
            action: change.action,
            build: () => buildTextDiffModel(isInsert ? "" : text, isInsert ? text : ""),
          })
        }
      }
    }
    if (bodyChanged && change.bodyDiff?.length) {
      const bodyDiff = change.bodyDiff
      cards.push({
        key: `${changeIndex}-body`,
        category,
        title: "章节正文",
        subtitle: contextPath,
        action: change.action,
        build: () => buildBodyHunksModel(bodyDiff),
      })
    }
    return cards
  }

  for (const key of changed) {
    cards.push({
      key: `${changeIndex}-${key}`,
      category,
      title: fields[key] || key,
      subtitle: contextPath,
      build: () => buildTextDiffModel(diffFieldText(before[key]), diffFieldText(after[key])),
    })
  }

  if (bodyChanged) {
    cards.push({
      key: `${changeIndex}-body`,
      category,
      title: "章节正文",
      subtitle: contextPath,
      build: () => change.bodyDiff?.length
        ? buildBodyHunksModel(change.bodyDiff)
        : { blocks: [], added: 0, removed: 0, note: "已修改章节正文（此记录未保存行级差异）" },
    })
  }

  return cards
}

type PageItem = number | "ellipsis-start" | "ellipsis-end"

function buildPageItems(current: number, total: number): PageItem[] {
  if (total <= 7) return Array.from({ length: total }, (_, index) => index + 1)
  if (current <= 4) return [1, 2, 3, 4, 5, "ellipsis-end", total]
  if (current >= total - 3) return [1, "ellipsis-start", total - 4, total - 3, total - 2, total - 1, total]
  return [1, "ellipsis-start", current - 1, current, current + 1, "ellipsis-end", total]
}

function scrollAnchorIntoView(element: HTMLElement | null) {
  if (!element) return
  const top = element.getBoundingClientRect().top
  if (top < 76) {
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches
    window.scrollBy({ top: top - 84, behavior: reduced ? "auto" : "smooth" })
  }
}

function HistoryPagination({
  label,
  page,
  pageSize,
  total,
  itemLabel,
  totalLabel,
  compact = false,
  disabled = false,
  onPageChange,
}: {
  label: string
  page: number
  pageSize: number
  total: number
  itemLabel: string
  totalLabel: string
  compact?: boolean
  disabled?: boolean
  onPageChange: (page: number) => void
}) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  if (totalPages <= 1 && page <= 1) return null
  const start = total === 0 ? 0 : Math.min(total, (page - 1) * pageSize + 1)
  const end = Math.min(total, page * pageSize)
  const items = buildPageItems(page, totalPages)

  return <nav className={compact ? "novel-diff-pagination" : "novel-history-pagination"} aria-label={label}>
    <span className="novel-history-pagination-info" aria-live="polite">
      {compact ? `第 ${start}–${end} ${itemLabel}` : `显示第 ${start}–${end} ${itemLabel}`}，共 {total} {totalLabel}
    </span>
    <div className="novel-history-pagination-controls">
      <button type="button" className="novel-history-page-btn is-nav"
        disabled={disabled || page <= 1} aria-label="上一页" onClick={() => onPageChange(page - 1)}>
        <ChevronLeft aria-hidden="true"/>
        <span>上一页</span>
      </button>
      <div className="novel-history-pagination-pages">
        {items.map(entry => typeof entry === "number"
          ? <button key={entry} type="button" className="novel-history-page-btn"
              data-active={entry === page || undefined}
              aria-current={entry === page ? "page" : undefined}
              aria-label={`第 ${entry} 页`}
              disabled={disabled}
              onClick={() => { if (entry !== page) onPageChange(entry) }}>
              {entry}
            </button>
          : <span key={entry} className="novel-history-page-ellipsis" aria-hidden="true">…</span>)}
      </div>
      <span className="novel-history-pagination-compact-status" aria-hidden="true">{page} / {totalPages}</span>
      <button type="button" className="novel-history-page-btn is-nav"
        disabled={disabled || page >= totalPages} aria-label="下一页" onClick={() => onPageChange(page + 1)}>
        <span>下一页</span>
        <ChevronRight aria-hidden="true"/>
      </button>
    </div>
  </nav>
}

export function NovelHistoryPage({ id }: { id: number }) {
  return <div className="novel-demo novel-history-view">
    <NovelAccessGate>
      <Suspense fallback={<div className="novel-loading" role="status">正在加载…</div>}>
        <HistoryData key={id} id={id}/>
      </Suspense>
    </NovelAccessGate>
  </div>
}

function HistoryRevision({ item, now }: { item: NovelRevision; now: number }) {
  const detailId = useId()
  const itemRef = useRef<HTMLLIElement>(null)
  const [opened, setOpened] = useState(false)
  const [rendered, setRendered] = useState(false)
  const [copied, setCopied] = useState(false)
  const [activeCategory, setActiveCategory] = useState<"ALL" | DiffCardCategory>("ALL")
  const [cardPage, setCardPage] = useState(1)
  const [defaultCardOpen, setDefaultCardOpen] = useState(true)
  const [cardOpenOverrides, setCardOpenOverrides] = useState<Record<string, boolean>>({})
  const copyTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const { request } = useNovelAccess()
  const changeTotal = item.changeCount ?? item.changes.length
  // The list omits chapter body diffs and everything past its first changes.
  // An entry it describes completely is shown as is; others are read in pages.
  const listedCompletely = changeTotal <= item.changes.length && !item.changes.some(change =>
    change.entity === "Chapter" && change.action === "UPDATE" && change.before?.contentHash !== change.after?.contentHash)
  const [loaded, setLoaded] = useState<NovelChange[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState("")
  const details = listedCompletely ? item.changes : loaded
  const loader = useRef<AbortController | null>(null)

  useEffect(() => () => { clearTimeout(copyTimer.current); loader.current?.abort() }, [])

  function loadChanges() {
    if (loading || listedCompletely) return
    const offset = loaded?.length ?? 0
    if (loaded && offset >= changeTotal) return
    const controller = new AbortController()
    loader.current = controller
    setLoading(true); setLoadError("")
    void request<NovelRevisionChanges>(
      `/api/v4/history?novelId=${item.novelId}&revision=${item.id}&limit=${REVISION_CHANGES_PAGE_SIZE}&offset=${offset}`, controller.signal)
      .then(page => {
        if (controller.signal.aborted) return
        setLoaded(current => [...(current ?? []), ...page.changes])
        setLoading(false)
      })
      .catch(reason => {
        if (controller.signal.aborted) return
        setLoadError(reason instanceof Error ? reason.message : "加载失败。")
        setLoading(false)
      })
  }

  const summary = useMemo(() => summarizeRevision(item), [item])
  const cards = useMemo(() => rendered && details ? details.flatMap((change, index) => buildChangeCards(change, index)) : [], [details, rendered])
  const remaining = details ? Math.max(0, changeTotal - details.length) : 0
  const categoryTabs = useMemo(() => {
    const counts = new Map<DiffCardCategory, number>()
    for (const card of cards) counts.set(card.category, (counts.get(card.category) || 0) + 1)
    return categoryOrder
      .filter(category => (counts.get(category) || 0) > 0)
      .map(category => ({ category, count: counts.get(category)! }))
  }, [cards])

  const effectiveCategory = activeCategory !== "ALL" && categoryTabs.some(tab => tab.category === activeCategory)
    ? activeCategory
    : "ALL"
  const filteredCards = useMemo(
    () => effectiveCategory === "ALL" ? cards : cards.filter(card => card.category === effectiveCategory),
    [cards, effectiveCategory],
  )
  const totalCardPages = Math.max(1, Math.ceil(filteredCards.length / DIFF_CARDS_PAGE_SIZE))
  const safeCardPage = Math.min(cardPage, totalCardPages)
  const visibleCards = useMemo(
    () => filteredCards.slice((safeCardPage - 1) * DIFF_CARDS_PAGE_SIZE, safeCardPage * DIFF_CARDS_PAGE_SIZE),
    [filteredCards, safeCardPage],
  )
  const openVisibleCount = visibleCards.reduce((sum, spec) => sum + ((cardOpenOverrides[spec.key] ?? defaultCardOpen) ? 1 : 0), 0)
  const shouldCollapseAll = visibleCards.length > 0 && openVisibleCount >= Math.ceil(visibleCards.length / 2)

  const hasChanges = item.changes.length > 0
  const shortId = item.id.slice(0, 7)
  const fullTimestamp = item.startedAt && item.startedAt !== item.createdAt
    ? `开始于 ${formatExactTime(item.startedAt)} · 完成于 ${formatExactTime(item.createdAt)}`
    : formatExactTime(item.createdAt)

  function toggle() {
    if (!hasChanges) return
    if (!opened) {
      setRendered(true)
      if (!details) loadChanges()
    }
    setOpened(value => !value)
  }

  function selectCategory(next: "ALL" | DiffCardCategory) {
    setActiveCategory(next)
    setCardPage(1)
  }

  function toggleAllCards() {
    const nextOpen = !shouldCollapseAll
    setDefaultCardOpen(nextOpen)
    setCardOpenOverrides({})
  }

  function setCardOpen(key: string, nextOpen: boolean) {
    setCardOpenOverrides(current => ({ ...current, [key]: nextOpen }))
  }

  function changeCardPage(nextPage: number) {
    setCardPage(nextPage)
    scrollAnchorIntoView(itemRef.current)
  }

  async function copyRevisionId() {
    try {
      await navigator.clipboard.writeText(item.id)
      setCopied(true)
      clearTimeout(copyTimer.current)
      copyTimer.current = setTimeout(() => setCopied(false), 2000)
    } catch {
      /* Clipboard access may be unavailable in restricted contexts. */
    }
  }

  return <li ref={itemRef} className="novel-history-item" data-open={opened || undefined}>
    <div className="novel-history-row">
      <div className="novel-history-main">
        <div className="novel-history-title-row">
          {hasChanges ? <>
            <button type="button" className="novel-history-title" title={summary}
              aria-expanded={opened} aria-controls={detailId} onClick={toggle}>
              {summary}
            </button>
            <Tooltip>
              <TooltipTrigger render={<button type="button" className="novel-history-more" data-open={opened || undefined}
                aria-expanded={opened} aria-controls={detailId}
                aria-label={opened ? "收起变更详情" : `展开 ${changeTotal} 处变更详情`}
                onClick={toggle} />}>
                <MoreHorizontal aria-hidden="true"/>
              </TooltipTrigger>
              <TooltipContent>{opened ? "收起变更详情" : `展开 ${changeTotal} 处变更详情`}</TooltipContent>
            </Tooltip>
          </> : <span className="novel-history-title is-static" title={summary}>{summary}</span>}
        </div>
        <div className="novel-history-meta">
          <ContributorAvatar contributor={item.contributor} size="tiny"/>
          <span className="novel-history-author">{item.contributor?.name || "未记录账号"}</span>
          <Tooltip>
            <TooltipTrigger render={<time className="novel-history-time" dateTime={item.createdAt} tabIndex={0} />}>
              {formatRelativeCommitTime(item.createdAt, now)}
            </TooltipTrigger>
            <TooltipContent>{fullTimestamp}</TooltipContent>
          </Tooltip>
          {(item.revisionCount ?? 1) > 1 && <span className="novel-history-batch">· 含 {item.revisionCount} 次连续提交</span>}
        </div>
      </div>

      <div className="novel-history-actions">
        <Tooltip>
          <TooltipTrigger render={hasChanges
            ? <button type="button" className="novel-history-sha" aria-expanded={opened} aria-controls={detailId}
                aria-label={`记录编号 ${shortId}，点击查看变更详情`} onClick={toggle} />
            : <span className="novel-history-sha is-static" tabIndex={0} aria-label={`记录编号 ${item.id}`} />}>
            <code>{shortId}</code>
          </TooltipTrigger>
          <TooltipContent className="break-all font-mono">{item.id}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger render={<button type="button" className="novel-history-icon-btn"
            aria-label={`复制完整记录编号 ${item.id}`} onClick={copyRevisionId} />}>
            {copied ? <Check className="novel-history-copied" aria-hidden="true"/> : <Copy aria-hidden="true"/>}
          </TooltipTrigger>
          <TooltipContent>{copied ? "已复制完整编号" : "复制完整记录编号"}</TooltipContent>
        </Tooltip>
        <span className="sr-only" role="status">{copied ? `记录编号 ${shortId} 已复制` : ""}</span>

        {hasChanges && <Tooltip>
          <TooltipTrigger render={<button type="button" className="novel-history-icon-btn" data-active={opened || undefined}
            aria-expanded={opened} aria-controls={detailId}
            aria-label={opened ? "收起变更差异" : "查看变更差异"} onClick={toggle} />}>
            <Code aria-hidden="true"/>
          </TooltipTrigger>
          <TooltipContent>{opened ? "收起变更差异" : "查看变更差异"}</TooltipContent>
        </Tooltip>}
      </div>
    </div>

    {hasChanges && <NovelAutoHeight open={opened}>
      <div id={detailId} className="novel-history-details">
        {rendered && !details && <div className="novel-history-details-body">
          <div className="novel-diff-card"><div className="novel-diff-note" role="status">
            {loadError ? <>{loadError} <button type="button" className="novel-history-bulk-btn" onClick={loadChanges}>重试</button></> : "正在加载变更详情…"}
          </div></div>
        </div>}
        {rendered && details && (cards.length > 0 ? <>
          {cards.length > 1 && <div className="novel-history-details-toolbar">
            {categoryTabs.length > 1 ? <div className="novel-history-filters" role="group" aria-label="按变更类型筛选">
              <button type="button" className="novel-history-filter-pill"
                data-active={effectiveCategory === "ALL" || undefined}
                aria-pressed={effectiveCategory === "ALL"}
                onClick={() => selectCategory("ALL")}>
                <span>全部</span>
                <span className="novel-history-filter-count">{cards.length}</span>
              </button>
              {categoryTabs.map(({ category, count }) => <button key={category} type="button"
                className="novel-history-filter-pill"
                data-active={effectiveCategory === category || undefined}
                aria-pressed={effectiveCategory === category}
                onClick={() => selectCategory(category)}>
                <span>{categoryLabels[category]}</span>
                <span className="novel-history-filter-count">{count}</span>
              </button>)}
            </div> : <span className="novel-history-details-summary">
              共 {cards.length} 项{categoryTabs[0] ? categoryLabels[categoryTabs[0].category] : ""}变更
            </span>}

            <div className="novel-history-details-tools">
              {totalCardPages > 1 && <span className="novel-history-page-indicator">第 {safeCardPage} / {totalCardPages} 页</span>}
              <button type="button" className="novel-history-bulk-btn" onClick={toggleAllCards}>
                {shouldCollapseAll ? <ChevronsDownUp aria-hidden="true"/> : <ChevronsUpDown aria-hidden="true"/>}
                <span>{shouldCollapseAll ? "全部折叠" : "全部展开"}</span>
              </button>
            </div>
          </div>}

          <div className="novel-history-details-body">
            {visibleCards.map(spec => <UnifiedDiffCard
              key={spec.key}
              spec={spec}
              open={cardOpenOverrides[spec.key] ?? defaultCardOpen}
              onOpenChange={nextOpen => setCardOpen(spec.key, nextOpen)}
            />)}
          </div>

          {totalCardPages > 1 && <HistoryPagination
            compact
            label="提交内变更差异分页"
            page={safeCardPage}
            pageSize={DIFF_CARDS_PAGE_SIZE}
            total={filteredCards.length}
            itemLabel="项"
            totalLabel={effectiveCategory === "ALL" ? "项变更" : `项${categoryLabels[effectiveCategory]}变更`}
            onPageChange={changeCardPage}
          />}
          {remaining > 0 && <div className="novel-history-details-toolbar">
            <span className="novel-history-details-summary" role="status">
              {loadError || `已加载 ${details.length} / ${changeTotal} 处变更`}
            </span>
            <div className="novel-history-details-tools">
              <button type="button" className="novel-history-bulk-btn" disabled={loading} onClick={loadChanges}>
                <span>{loading ? "正在加载…" : loadError ? "重试" : "加载更多变更"}</span>
              </button>
            </div>
          </div>}
        </> : <div className="novel-history-details-body">
          <div className="novel-diff-card"><div className="novel-diff-note">此记录未包含可展示的字段差异</div></div>
        </div>)}
      </div>
    </NovelAutoHeight>}
  </li>
}

function HistorySkeleton() {
  return <div className="novel-history-list is-loading" role="status" aria-label="正在加载变更历史…">
    <div className="novel-history-group">
      <div className="novel-history-group-header">
        <span className="novel-history-node" aria-hidden="true"><GitCommitHorizontal/></span>
        <span className="novel-history-skeleton-line w-36 h-3.5"/>
      </div>
      <div className="novel-history-revisions">
        {[0, 1, 2, 3].map(row => <div className="novel-history-row" key={row}>
          <div className="novel-history-main">
            <span className="novel-history-skeleton-line h-4" style={{ width: `${68 - (row % 3) * 14}%` }}/>
            <div className="novel-history-meta">
              <span className="novel-history-skeleton-circle"/>
              <span className="novel-history-skeleton-line w-28 h-3"/>
            </div>
          </div>
          <div className="novel-history-actions">
            <span className="novel-history-skeleton-line w-20 h-6"/>
          </div>
        </div>)}
      </div>
    </div>
  </div>
}

function HistoryData({ id }: { id: number }) {
  const { request } = useNovelAccess()
  const params = useSearchParams()
  const pathname = usePathname()
  const listTopRef = useRef<HTMLOListElement>(null)

  const rawPage = Number(params.get("page") || 1)
  const currentPage = Number.isSafeInteger(rawPage) && rawPage > 0 && rawPage <= 1_000_000 ? rawPage : 1

  const [book, setBook] = useState<Novel | null>(null)
  const [history, setHistory] = useState<NovelHistory | null>(null)
  const [pending, setPending] = useState(true)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const [now, setNow] = useState(() => Date.now())

  const queryString = params.toString()
  const updatePage = useCallback((nextPage: number) => {
    const next = new URLSearchParams(queryString)
    if (nextPage <= 1) next.delete("page")
    else next.set("page", String(nextPage))
    window.history.replaceState(null, "", `${pathname}${next.size ? `?${next}` : ""}`)
  }, [pathname, queryString])

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void request<Novel>(`/api/v5/novelinfo?id=${id}`, controller.signal)
      .then(novelInfo => { if (!controller.signal.aborted) setBook(novelInfo) })
      .catch(() => { /* History error state is driven by the history endpoint below. */ })
    return () => controller.abort()
  }, [request, id, revision])

  useEffect(() => {
    const controller = new AbortController()
    setPending(true); setError("")
    const offset = (currentPage - 1) * HISTORY_PAGE_SIZE
    void request<NovelHistory>(`/api/v4/history?novelId=${id}&limit=${HISTORY_PAGE_SIZE}&offset=${offset}`, controller.signal)
      .then(historyPage => {
        if (controller.signal.aborted) return
        const lastPage = Math.max(1, Math.ceil(historyPage.total / HISTORY_PAGE_SIZE))
        if (currentPage > lastPage) {
          updatePage(lastPage)
          return
        }
        setHistory(historyPage)
        setNow(Date.now())
        setPending(false)
      })
      .catch(reason => {
        if (!controller.signal.aborted) {
          setError(reason instanceof Error ? reason.message : "加载失败。")
          setPending(false)
        }
      })
    return () => controller.abort()
  }, [request, id, currentPage, revision, updatePage])

  function handlePageChange(nextPage: number) {
    updatePage(nextPage)
    scrollAnchorIntoView(listTopRef.current)
  }

  const groups = useMemo(() => history ? groupRevisionsByDate(history.revisions) : [], [history])

  return <main className="page-shell novel-history">
    <Link className="novel-history-back" href={`/library/books/${id}`}><ArrowLeft aria-hidden="true"/>返回作品</Link>
    <header className="novel-history-heading">
      <div className="novel-history-heading-main">
        <h1><History aria-hidden="true"/>变更历史</h1>
        {book && <p>{book.title}</p>}
      </div>
      {history && history.total > 0 && <span className="novel-history-total">共 {history.total} 次提交记录</span>}
    </header>
    {error ? <ContentFailure message={error} retry={() => setRevision(value => value + 1)}/> :
      pending && !history ? <HistorySkeleton/> :
      !history || history.revisions.length === 0 ? <section className="panel novel-history-empty" role="status">暂无变更记录</section> :
      <>
        <ol ref={listTopRef} className="novel-history-list novel-content-ready" aria-busy={pending || undefined}>
          {groups.map(group => <li className="novel-history-group" key={group.key}>
            <div className="novel-history-group-header">
              <span className="novel-history-node" aria-hidden="true"><GitCommitHorizontal/></span>
              <h2 className="novel-history-date">提交于 {group.label}</h2>
            </div>
            <ol className="novel-history-revisions">
              {group.items.map(item => <HistoryRevision key={item.id} item={item} now={now}/>)}
            </ol>
          </li>)}
        </ol>
        <HistoryPagination
          label="变更历史分页"
          page={currentPage}
          pageSize={HISTORY_PAGE_SIZE}
          total={history.total}
          itemLabel="条"
          totalLabel="次提交"
          disabled={pending}
          onPageChange={handlePageChange}
        />
      </>}
  </main>
}
