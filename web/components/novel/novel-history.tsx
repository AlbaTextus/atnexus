"use client"

import { useEffect, useId, useMemo, useRef, useState } from "react"
import { diffChars } from "diff"
import { Link } from "next-view-transitions"
import { ArrowLeft, Check, Code, Copy, GitCommitHorizontal, History, MoreHorizontal } from "lucide-react"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel/novel-access"
import { ContributorAvatar } from "@/components/novel/contributors-card"
import { NovelAutoHeight } from "@/components/novel/novel-motion"
import { Button } from "@/components/novel/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { ChapterBodyHunk, Novel, NovelChange, NovelHistory, NovelRevision } from "@/lib/athub/athub-client"

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

function valueText(value: unknown): string {
  if (value === null || value === undefined || value === "") return "未设置"
  if (Array.isArray(value)) return value.map(valueText).join("、") || "无"
  if (value === "ONGOING") return "连载中"
  if (value === "COMPLETED") return "已完结"
  if (value === "ABANDONED") return "太监"
  return typeof value === "object" ? JSON.stringify(value) : String(value)
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

function summarizeNamedBatch(items: NovelChange[], unit: string, verb: string): string {
  const first = titleOf(items[0]?.after, items[0]?.before)
  if (items.length === 1) return first ? `${verb}${unit}「${first}」` : `${verb}${unit}`
  return first ? `${verb}「${first}」等 ${items.length} 个${unit}` : `${verb} ${items.length} 个${unit}`
}

function summarizeChapterUpdates(updates: NovelChange[]): string {
  if (updates.length === 1) {
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
  if (onlyOrder) return `调整 ${updates.length} 个章节的目录顺序`
  return summarizeNamedBatch(updates, "章节", "修改")
}

function summarizeRevision(item: NovelRevision): string {
  const { changes } = item
  if (!changes.length) return "提交变更"

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
  const createdNovel = novelInserts.length > 0

  if (createdNovel) {
    const imported: string[] = []
    if (volInserts.length > 0) imported.push(`${volInserts.length} 卷`)
    if (chapInserts.length > 0) imported.push(`${chapInserts.length} 章`)
    if (illInserts.length > 0) imported.push(`${illInserts.length} 张插图`)
    clauses.push(imported.length ? `创建小说条目，收录 ${imported.join("、")}` : "创建小说条目")
  }

  if (novelDeletes.length > 0) clauses.push("删除小说条目")
  else if (novelUpdates.length > 0) clauses.push(summarizeNovelUpdates(novelUpdates))

  if (!createdNovel && volInserts.length > 0) clauses.push(summarizeNamedBatch(volInserts, "分卷", "新增"))
  if (volUpdates.length > 0) clauses.push(summarizeNamedBatch(volUpdates, "分卷", "修改"))
  if (volDeletes.length > 0) clauses.push(summarizeNamedBatch(volDeletes, "分卷", "删除"))

  if (!createdNovel && chapInserts.length > 0) clauses.push(summarizeNamedBatch(chapInserts, "章节", "新增"))
  if (chapUpdates.length > 0) clauses.push(summarizeChapterUpdates(chapUpdates))
  if (chapDeletes.length > 0) clauses.push(summarizeNamedBatch(chapDeletes, "章节", "删除"))

  if (!createdNovel && illInserts.length > 0) clauses.push(illInserts.length === 1 ? "上传插图" : `上传 ${illInserts.length} 张插图`)
  if (illUpdates.length > 0) clauses.push("更新插图信息")
  if (illDeletes.length > 0) clauses.push(illDeletes.length === 1 ? "删除插图" : `删除 ${illDeletes.length} 张插图`)

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

function TextChange({ before, after }: { before: string; after: string }) {
  const parts = useMemo(() => diffChars(before, after, { timeout: 50 }), [before, after])
  return <>
    <del className="novel-history-text-before" aria-label="修改前"><span className="novel-history-diff-sign" aria-hidden="true">−</span><span>{parts
      ? parts.filter(part => !part.added).map((part, index) => part.removed ? <mark key={index}>{part.value}</mark> : <span key={index}>{part.value}</span>)
      : <mark>{before}</mark>}</span></del>
    <ins className="novel-history-text-after" aria-label="修改后"><span className="novel-history-diff-sign" aria-hidden="true">+</span><span>{parts
      ? parts.filter(part => !part.removed).map((part, index) => part.added ? <mark key={index}>{part.value}</mark> : <span key={index}>{part.value}</span>)
      : <mark>{after}</mark>}</span></ins>
  </>
}

function BodyChange({ hunks }: { hunks: ChapterBodyHunk[] }) {
  return <div className="novel-history-body-diff">{hunks.map((hunk, index) => <div className="novel-history-body-hunk" key={index}>
    <div className="novel-history-hunk-location">@@ 原文第 {hunk.beforeLine} 行 → 修改后第 {hunk.afterLine} 行 @@</div>
    <del className="novel-history-text-before" aria-label="正文修改前"><span className="novel-history-diff-sign" aria-hidden="true">−</span><span>{hunk.parts.filter(part => part.kind !== "insert").map((part, i) =>
      part.kind === "delete" ? <mark key={i}>{part.text}</mark> : <span key={i}>{part.text}</span>)}</span></del>
    <ins className="novel-history-text-after" aria-label="正文修改后"><span className="novel-history-diff-sign" aria-hidden="true">+</span><span>{hunk.parts.filter(part => part.kind !== "delete").map((part, i) =>
      part.kind === "insert" ? <mark key={i}>{part.text}</mark> : <span key={i}>{part.text}</span>)}</span></ins>
  </div>)}</div>
}

function ChangeDetail({ change }: { change: NovelChange }) {
  const before = change.before || {}, after = change.after || {}
  const changed = Object.keys({ ...before, ...after }).filter(key =>
    key !== "id" && key !== "novelId" && key !== "contentHash" &&
    JSON.stringify(before[key]) !== JSON.stringify(after[key]))
  const bodyChanged = before.contentHash !== after.contentHash
  return <div className="novel-history-change">
    <div className="novel-history-change-header">
      <span className="novel-history-action-badge" data-action={change.action}>{actions[change.action] || change.action}</span>
      <strong>{entities[change.entity] || change.entity}{(change.entity === "Volume" || change.entity === "Chapter") && ` · ${valueText(after.title ?? before.title)}`}</strong>
    </div>
    <dl>{changed.map(key => <div key={key}>
      <dt>{fields[key] || key}</dt>
      <dd>{change.action === "UPDATE" && typeof before[key] === "string" && typeof after[key] === "string"
        ? <TextChange before={valueText(before[key])} after={valueText(after[key])}/>
        : <>{change.action !== "INSERT" && <del>{valueText(before[key])}</del>}{change.action !== "DELETE" && <ins>{valueText(after[key])}</ins>}</>}</dd>
    </div>)}
    {bodyChanged && <div><dt>章节正文</dt><dd>{change.bodyDiff?.length
      ? <BodyChange hunks={change.bodyDiff}/>
      : change.action === "INSERT" ? "已添加" : change.action === "DELETE" ? "已删除" : "已修改（此记录未保存正文差异）"}</dd></div>}
    </dl>
  </div>
}

export function NovelHistoryPage({ id }: { id: number }) {
  return <div className="novel-demo novel-history-view"><NovelAccessGate><HistoryData key={id} id={id}/></NovelAccessGate></div>
}

function HistoryRevision({ item, now }: { item: NovelRevision; now: number }) {
  const detailId = useId()
  const [opened, setOpened] = useState(false)
  const [rendered, setRendered] = useState(false)
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => () => clearTimeout(copyTimer.current), [])

  const summary = useMemo(() => summarizeRevision(item), [item])
  const hasChanges = item.changes.length > 0
  const shortId = item.id.slice(0, 7)
  const fullTimestamp = item.startedAt && item.startedAt !== item.createdAt
    ? `开始于 ${formatExactTime(item.startedAt)} · 完成于 ${formatExactTime(item.createdAt)}`
    : formatExactTime(item.createdAt)

  function toggle() {
    if (!hasChanges) return
    if (!opened) setRendered(true)
    setOpened(value => !value)
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

  return <li className="novel-history-item" data-open={opened || undefined}>
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
                aria-label={opened ? "收起变更详情" : `展开 ${item.changes.length} 处变更详情`}
                onClick={toggle} />}>
                <MoreHorizontal aria-hidden="true"/>
              </TooltipTrigger>
              <TooltipContent>{opened ? "收起变更详情" : `展开 ${item.changes.length} 处变更详情`}</TooltipContent>
            </Tooltip>
          </> : <span className="novel-history-title is-static" title={summary}>{summary}</span>}
        </div>
        <div className="novel-history-meta">
          <ContributorAvatar contributor={item.contributor} size="tiny"/>
          <span className="novel-history-author">{item.contributor.name || "未记录账号"}</span>
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
        {rendered && <div className="novel-history-details-body">
          {item.changes.map((change, index) => <ChangeDetail key={index} change={change}/>)}
        </div>}
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
  const [page, setPage] = useState(0)
  const [book, setBook] = useState<Novel | null>(null)
  const [history, setHistory] = useState<NovelHistory | null>(null)
  const [pending, setPending] = useState(true)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    setPending(true); setError("")
    void Promise.all([
      request<Novel>(`/api/v5/novelinfo?id=${id}`, controller.signal),
      request<NovelHistory>(`/api/v4/history?novelId=${id}&limit=20&offset=${page * 20}`, controller.signal),
    ]).then(([novelInfo, historyPage]) => {
      if (!controller.signal.aborted) {
        setBook(novelInfo)
        setHistory(historyPage)
        setNow(Date.now())
      }
    }).catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    }).finally(() => {
      if (!controller.signal.aborted) setPending(false)
    })
    return () => controller.abort()
  }, [request, id, page, revision])

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
        <ol className="novel-history-list novel-content-ready" aria-busy={pending || undefined}>
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
        {(history.total > 20 || page > 0) && <nav className="novel-history-pagination" aria-label="变更历史分页">
          <Button variant="outline" disabled={pending || page === 0} onClick={() => setPage(value => value - 1)}>上一页</Button>
          <span>{page + 1} / {Math.max(1, Math.ceil(history.total / 20))}</span>
          <Button variant="outline" disabled={pending || (page + 1) * 20 >= history.total} onClick={() => setPage(value => value + 1)}>下一页</Button>
        </nav>}
      </>}
  </main>
}
