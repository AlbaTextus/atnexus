"use client"

import { Fragment, Suspense, useCallback, useEffect, useId, useMemo, useRef, useState } from "react"
import { diffArrays, diffChars } from "diff"
import { usePathname, useSearchParams } from "next/navigation"
import { Link } from "next-view-transitions"
import {
  ArrowLeft, Check, ChevronDown, ChevronLeft, ChevronRight, ChevronsDownUp, ChevronsUpDown, ChevronUp,
  Code, Copy, GitCommitHorizontal, History, MoreHorizontal,
} from "lucide-react"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel/novel-access"
import { ContributorAvatar } from "@/components/novel/contributors-card"
import { NovelAutoHeight } from "@/components/novel/novel-motion"
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

/* Unified line-by-line diff model & builder */

type InlineToken = { text: string; highlight?: boolean }

type DiffLine = {
  kind: "context" | "del" | "add"
  oldLine: number | null
  newLine: number | null
  text: string
  tokens?: InlineToken[]
}

type DiffBlock =
  | { type: "lines"; key: string; lines: DiffLine[] }
  | { type: "fold"; key: string; lines: DiffLine[] }
  | { type: "gap"; key: string; label: string }

type UnifiedDiffModel = {
  blocks: DiffBlock[]
  added: number
  removed: number
  note?: string
}

function splitTextLines(text: string): string[] {
  return text === "" ? [] : text.replace(/\r\n/g, "\n").split("\n")
}

const delimiterPattern = /[，。！？；：、（）【】「」『』《》\[\]()\s]/
const asciiFragmentPattern = /^[^\u4e00-\u9fff\s\[\]()]+$/

function computeInlinePair(oldLine: string, newLine: string): { oldTokens: InlineToken[]; newTokens: InlineToken[] } | null {
  if (!oldLine || !newLine || oldLine.length > 2000 || newLine.length > 2000) return null
  const parts = diffChars(oldLine, newLine, { timeout: 30 })
  if (!parts) return null

  let shared = 0
  for (const part of parts) {
    if (!part.added && !part.removed) shared += part.value.length
  }
  const minLen = Math.min(oldLine.length, newLine.length)
  if (minLen === 0 || (shared / minLen < 0.2 && shared < 4)) return null

  type Segment = { kind: "equal"; text: string } | { kind: "edit"; del: string; add: string }
  const raw: Segment[] = []
  for (const part of parts) {
    if (!part.added && !part.removed) {
      const prev = raw[raw.length - 1]
      if (prev?.kind === "equal") prev.text += part.value
      else raw.push({ kind: "equal", text: part.value })
    } else {
      let prev = raw[raw.length - 1]
      if (!prev || prev.kind !== "edit") {
        prev = { kind: "edit", del: "", add: "" }
        raw.push(prev)
      }
      if (part.removed) prev.del += part.value
      if (part.added) prev.add += part.value
    }
  }

  // Merge tiny unchanged islands sandwiched between edits so URLs and phrases highlight cleanly.
  const merged: Segment[] = []
  for (let i = 0; i < raw.length; i++) {
    const seg = raw[i]
    const prev = merged[merged.length - 1]
    const next = raw[i + 1]
    if (
      seg.kind === "equal" &&
      prev?.kind === "edit" &&
      next?.kind === "edit" &&
      (seg.text.length === 1 || (!delimiterPattern.test(seg.text) && asciiFragmentPattern.test(seg.text) && seg.text.length <= 8))
    ) {
      prev.del += seg.text + next.del
      prev.add += seg.text + next.add
      i++
      continue
    }
    merged.push(seg)
  }

  if (!merged.some(seg => seg.kind === "equal")) return null

  const oldTokens: InlineToken[] = []
  const newTokens: InlineToken[] = []
  for (const seg of merged) {
    if (seg.kind === "equal") {
      oldTokens.push({ text: seg.text })
      newTokens.push({ text: seg.text })
    } else {
      if (seg.del) oldTokens.push({ text: seg.del, highlight: true })
      if (seg.add) newTokens.push({ text: seg.add, highlight: true })
    }
  }
  return { oldTokens, newTokens }
}

function buildDiffLines(oldLines: string[], newLines: string[], startOld = 1, startNew = 1): { lines: DiffLine[]; added: number; removed: number } {
  const chunks = diffArrays(oldLines, newLines, { timeout: 60 }) ?? [
    ...(oldLines.length > 0 ? [{ count: oldLines.length, added: false, removed: true, value: oldLines }] : []),
    ...(newLines.length > 0 ? [{ count: newLines.length, added: true, removed: false, value: newLines }] : []),
  ]
  const lines: DiffLine[] = []
  let oldNo = startOld
  let newNo = startNew
  let added = 0
  let removed = 0

  for (let i = 0; i < chunks.length; i++) {
    const chunk = chunks[i]
    if (!chunk.added && !chunk.removed) {
      for (const text of chunk.value) {
        lines.push({ kind: "context", oldLine: oldNo++, newLine: newNo++, text })
      }
      continue
    }
    const delTexts: string[] = []
    const addTexts: string[] = []
    while (i < chunks.length && (chunks[i].added || chunks[i].removed)) {
      if (chunks[i].removed) delTexts.push(...chunks[i].value)
      if (chunks[i].added) addTexts.push(...chunks[i].value)
      i++
    }
    i--

    const pairLimit = Math.min(delTexts.length, addTexts.length, 24)
    const delTokens: (InlineToken[] | undefined)[] = new Array(delTexts.length)
    const addTokens: (InlineToken[] | undefined)[] = new Array(addTexts.length)
    for (let k = 0; k < pairLimit; k++) {
      const paired = computeInlinePair(delTexts[k], addTexts[k])
      if (paired) {
        delTokens[k] = paired.oldTokens
        addTokens[k] = paired.newTokens
      }
    }

    for (let k = 0; k < delTexts.length; k++) {
      lines.push({ kind: "del", oldLine: oldNo++, newLine: null, text: delTexts[k], tokens: delTokens[k] })
      removed++
    }
    for (let k = 0; k < addTexts.length; k++) {
      lines.push({ kind: "add", oldLine: null, newLine: newNo++, text: addTexts[k], tokens: addTokens[k] })
      added++
    }
  }

  return { lines, added, removed }
}

const CONTEXT_KEEP = 3

function foldContextRuns(lines: DiffLine[], prefix = "b"): DiffBlock[] {
  const blocks: DiffBlock[] = []
  let index = 0
  let seq = 0

  while (index < lines.length) {
    if (lines[index].kind !== "context") {
      const start = index
      while (index < lines.length && lines[index].kind !== "context") index++
      blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: lines.slice(start, index) })
      continue
    }

    const start = index
    while (index < lines.length && lines[index].kind === "context") index++
    const run = lines.slice(start, index)
    const isStart = start === 0
    const isEnd = index === lines.length

    if (isStart && isEnd) {
      if (run.length > CONTEXT_KEEP + 1) {
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run.slice(0, CONTEXT_KEEP) })
        blocks.push({ type: "fold", key: `${prefix}-fold-${seq++}`, lines: run.slice(CONTEXT_KEEP) })
      } else {
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run })
      }
    } else if (isStart) {
      const hidden = run.length - CONTEXT_KEEP
      if (hidden >= 2) {
        blocks.push({ type: "fold", key: `${prefix}-fold-${seq++}`, lines: run.slice(0, hidden) })
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run.slice(hidden) })
      } else {
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run })
      }
    } else if (isEnd) {
      const hidden = run.length - CONTEXT_KEEP
      if (hidden >= 2) {
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run.slice(0, CONTEXT_KEEP) })
        blocks.push({ type: "fold", key: `${prefix}-fold-${seq++}`, lines: run.slice(CONTEXT_KEEP) })
      } else {
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run })
      }
    } else {
      const hidden = run.length - CONTEXT_KEEP * 2
      if (hidden >= 2) {
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run.slice(0, CONTEXT_KEEP) })
        blocks.push({ type: "fold", key: `${prefix}-fold-${seq++}`, lines: run.slice(CONTEXT_KEEP, run.length - CONTEXT_KEEP) })
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run.slice(run.length - CONTEXT_KEEP) })
      } else {
        blocks.push({ type: "lines", key: `${prefix}-${seq++}`, lines: run })
      }
    }
  }

  return blocks
}

function buildTextDiffModel(beforeText: string, afterText: string): UnifiedDiffModel {
  const { lines, added, removed } = buildDiffLines(splitTextLines(beforeText), splitTextLines(afterText), 1, 1)
  return { blocks: foldContextRuns(lines), added, removed }
}

function buildBodyHunksModel(hunks: ChapterBodyHunk[]): UnifiedDiffModel {
  const blocks: DiffBlock[] = []
  let totalAdded = 0
  let totalRemoved = 0
  let prevOldEnd = 0
  let prevOldRuneEnd = 0

  hunks.forEach((hunk, hunkIndex) => {
    const beforeText = hunk.parts.filter(part => part.kind !== "insert").map(part => part.text).join("")
    const afterText = hunk.parts.filter(part => part.kind !== "delete").map(part => part.text).join("")
    const skipped = hunk.beforeLine - prevOldEnd - 1
    const skippedRunes = hunk.beforeStart - prevOldRuneEnd
    if (skipped > 0) {
      blocks.push({
        type: "gap",
        key: `gap-${hunkIndex}`,
        label: hunkIndex === 0 ? `${skipped} 行未修改内容` : `省略中间 ${skipped} 行未修改内容`,
      })
    } else if (hunkIndex > 0 && skippedRunes > 0) {
      blocks.push({
        type: "gap",
        key: `gap-${hunkIndex}`,
        label: `省略同段 ${skippedRunes} 字未修改内容`,
      })
    }
    const { lines, added, removed } = buildDiffLines(
      splitTextLines(beforeText),
      splitTextLines(afterText),
      Math.max(1, hunk.beforeLine),
      Math.max(1, hunk.afterLine),
    )
    totalAdded += added
    totalRemoved += removed
    const lastOld = [...lines].reverse().find(line => line.oldLine !== null)?.oldLine
    prevOldEnd = lastOld ?? Math.max(prevOldEnd, hunk.beforeLine - 1)
    prevOldRuneEnd = Math.max(prevOldRuneEnd, hunk.beforeStart + Array.from(beforeText).length)
    blocks.push(...foldContextRuns(lines, `h${hunkIndex}`))
  })

  return { blocks, added: totalAdded, removed: totalRemoved }
}

const HISTORY_PAGE_SIZE = 20
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

function DiffLineRow({ line }: { line: DiffLine }) {
  const lineNumber = line.kind === "del" ? line.oldLine : (line.newLine ?? line.oldLine)
  return <div className="novel-diff-line" data-kind={line.kind}>
    <span className="novel-diff-bar" aria-hidden="true"/>
    <span className="novel-diff-num" aria-hidden="true">{lineNumber ?? ""}</span>
    <span className="novel-diff-code">
      {line.tokens
        ? line.tokens.map((token, i) => token.highlight ? <mark key={i}>{token.text}</mark> : <Fragment key={i}>{token.text}</Fragment>)
        : (line.text || "\u00A0")}
    </span>
  </div>
}

function UnifiedDiffCard({
  spec,
  open,
  onOpenChange,
}: {
  spec: DiffCardSpec
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const bodyId = useId()
  const [expandedFolds, setExpandedFolds] = useState<Record<string, boolean>>({})
  const model = useMemo(() => spec.build(), [spec])

  function toggleFold(key: string) {
    setExpandedFolds(current => ({ ...current, [key]: !current[key] }))
  }

  return <section className="novel-diff-card" data-collapsed={!open || undefined}>
    <button type="button" className="novel-diff-header" aria-expanded={open} aria-controls={bodyId} onClick={() => onOpenChange(!open)}>
      <span className="novel-diff-header-main">
        <ChevronDown className="novel-diff-chevron" aria-hidden="true"/>
        <Code className="novel-diff-file-icon" aria-hidden="true"/>
        <strong className="novel-diff-title">{spec.title}</strong>
        <span className="novel-diff-subtitle">{spec.subtitle}</span>
        {spec.action && spec.action !== "UPDATE" && <span className="novel-history-action-badge" data-action={spec.action}>{actions[spec.action] || spec.action}</span>}
      </span>
      {(model.added > 0 || model.removed > 0) && <span className="novel-diff-stats" aria-label={`新增 ${model.added} 行，删除 ${model.removed} 行`}>
        {model.added > 0 && <span className="novel-diff-stat-add">+{model.added}</span>}
        {model.removed > 0 && <span className="novel-diff-stat-del">−{model.removed}</span>}
      </span>}
    </button>

    {open && <div id={bodyId} className="novel-diff-body">
      {model.note ? <div className="novel-diff-note">{model.note}</div> : model.blocks.map(block => {
        if (block.type === "gap") {
          return <div className="novel-diff-gap" key={block.key}><span>{block.label}</span></div>
        }
        if (block.type === "lines") {
          return block.lines.map((line, i) => <DiffLineRow key={`${block.key}-${i}`} line={line}/>)
        }
        const unfolded = Boolean(expandedFolds[block.key])
        return <Fragment key={block.key}>
          <button type="button" className="novel-diff-fold" data-expanded={unfolded || undefined} onClick={() => toggleFold(block.key)}>
            {unfolded ? <ChevronUp aria-hidden="true"/> : <ChevronDown aria-hidden="true"/>}
            <span>{unfolded ? `收起 ${block.lines.length} 行未修改内容` : `${block.lines.length} 行未修改内容`}</span>
          </button>
          {unfolded && block.lines.map((line, i) => <DiffLineRow key={`${block.key}-${i}`} line={line}/>)}
        </Fragment>
      })}
    </div>}
  </section>
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

  useEffect(() => () => clearTimeout(copyTimer.current), [])

  const summary = useMemo(() => summarizeRevision(item), [item])
  const cards = useMemo(() => rendered ? item.changes.flatMap((change, index) => buildChangeCards(change, index)) : [], [item.changes, rendered])
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
    if (!opened) setRendered(true)
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
        {rendered && (cards.length > 0 ? <>
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
