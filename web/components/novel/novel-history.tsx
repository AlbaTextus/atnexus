"use client"

import { useEffect, useMemo, useState } from "react"
import { diffChars } from "diff"
import { Link } from "next-view-transitions"
import { ArrowLeft, GitCommitHorizontal, History } from "lucide-react"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel/novel-access"
import { ContributorAvatar } from "@/components/novel/contributors-card"
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

function valueText(value: unknown): string {
  if (value === null || value === undefined || value === "") return "未设置"
  if (Array.isArray(value)) return value.map(valueText).join("、") || "无"
  if (value === "ONGOING") return "连载中"
  if (value === "COMPLETED") return "已完结"
  if (value === "ABANDONED") return "太监"
  return typeof value === "object" ? JSON.stringify(value) : String(value)
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
    <div className="novel-history-hunk-location">原文第 {hunk.beforeLine} 行 → 修改后第 {hunk.afterLine} 行</div>
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
    <strong>{actions[change.action] || change.action}{entities[change.entity] || change.entity}{(change.entity === "Volume" || change.entity === "Chapter") && ` · ${valueText(after.title ?? before.title)}`}</strong>
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
  return <div className="novel-demo"><NovelAccessGate><HistoryData key={id} id={id}/></NovelAccessGate></div>
}

function HistoryRevision({ item }: { item: NovelRevision }) {
  const [opened, setOpened] = useState(false)
  return <li>
    <GitCommitHorizontal className="novel-history-node" aria-hidden="true"/>
    <article className="panel">
      <div className="novel-history-meta"><ContributorAvatar contributor={item.contributor}/><strong>{item.contributor.name || "未记录账号"}</strong><time dateTime={item.createdAt} title={item.startedAt && item.startedAt !== item.createdAt ? `开始于 ${new Date(item.startedAt).toLocaleString("zh-CN", { hour12: false })}` : undefined}>{new Date(item.createdAt).toLocaleString("zh-CN", { hour12: false })}</time><Tooltip><TooltipTrigger render={<code />} tabIndex={0} aria-label={`最新记录编号：${item.id}`}>{item.id.slice(0, 8)}</TooltipTrigger><TooltipContent className="break-all">{item.id}</TooltipContent></Tooltip></div>
      <details onToggle={event => { if (event.currentTarget.open) setOpened(true) }}><summary>{(item.revisionCount ?? 1) > 1 && `${item.revisionCount} 次提交 · `}{item.changes.length} 处变更</summary>{opened && <div>{item.changes.map((change, index) => <ChangeDetail key={index} change={change}/>)}</div>}</details>
    </article>
  </li>
}

function HistoryData({ id }: { id: number }) {
  const { request } = useNovelAccess()
  const [page, setPage] = useState(0)
  const [data, setData] = useState<{ book: Novel; history: NovelHistory } | null>(null)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setData(null); setError("")
    void Promise.all([
      request<Novel>(`/api/v5/novelinfo?id=${id}`, controller.signal),
      request<NovelHistory>(`/api/v4/history?novelId=${id}&limit=20&offset=${page * 20}`, controller.signal),
    ]).then(([book, history]) => {
      if (!controller.signal.aborted) setData({ book, history })
    }).catch(reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败。")
    })
    return () => controller.abort()
  }, [request, id, page, revision])
  return <main className="page-shell novel-history">
    <Link className="novel-history-back" href={`/library/books/${id}`}><ArrowLeft aria-hidden="true"/>返回作品</Link>
    <header className="novel-history-heading"><h1><History aria-hidden="true"/>变更历史</h1>{data && <p>{data.book.title}</p>}</header>
    {error ? <ContentFailure message={error} retry={() => setRevision(value => value + 1)}/> : !data ? <div role="status">正在加载变更历史…</div> :
      data.history.revisions.length === 0 ? <section className="panel" role="status">暂无变更记录</section> :
      <><ol className="novel-history-list novel-content-ready">{data.history.revisions.map(item => <HistoryRevision key={item.id} item={item}/>)}</ol>
      <nav className="novel-history-pagination" aria-label="变更历史分页"><Button variant="outline" disabled={page === 0} onClick={() => setPage(value => value - 1)}>上一页</Button><span>{page + 1} / {Math.max(1, Math.ceil(data.history.total / 20))}</span><Button variant="outline" disabled={(page + 1) * 20 >= data.history.total} onClick={() => setPage(value => value + 1)}>下一页</Button></nav></>}
  </main>
}
