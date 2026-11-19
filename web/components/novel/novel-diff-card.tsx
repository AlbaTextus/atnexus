"use client"

import { Fragment, useId, useMemo, useState } from "react"
import { diffArrays, diffChars } from "diff"
import { ChevronDown, ChevronUp, Code } from "lucide-react"
import type { ChapterBodyHunk } from "@/lib/athub/athub-client"

export type InlineToken = { text: string; highlight?: boolean }

export type DiffLine = {
  kind: "context" | "del" | "add"
  oldLine: number | null
  newLine: number | null
  text: string
  tokens?: InlineToken[]
}

export type DiffBlock =
  | { type: "lines"; key: string; lines: DiffLine[] }
  | { type: "fold"; key: string; lines: DiffLine[] }
  | { type: "gap"; key: string; label: string }

export type UnifiedDiffModel = {
  blocks: DiffBlock[]
  added: number
  removed: number
  note?: string
}

export type UnifiedDiffCardSpec = {
  title: string
  subtitle?: string
  action?: string
  build: () => UnifiedDiffModel
}

const diffActionLabels: Record<string, string> = {
  INSERT: "新增",
  UPDATE: "修改",
  DELETE: "删除",
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

export function buildTextDiffModel(beforeText: string, afterText: string): UnifiedDiffModel {
  const { lines, added, removed } = buildDiffLines(splitTextLines(beforeText), splitTextLines(afterText), 1, 1)
  return { blocks: foldContextRuns(lines), added, removed }
}

export function buildBodyHunksModel(hunks: ChapterBodyHunk[]): UnifiedDiffModel {
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

export function UnifiedDiffCard({
  spec,
  open,
  onOpenChange,
  defaultOpen = true,
  showHeader = true,
}: {
  spec: UnifiedDiffCardSpec
  open?: boolean
  onOpenChange?: (open: boolean) => void
  defaultOpen?: boolean
  showHeader?: boolean
}) {
  const bodyId = useId()
  const [internalOpen, setInternalOpen] = useState(defaultOpen)
  const [expandedFolds, setExpandedFolds] = useState<Record<string, boolean>>({})
  const isOpen = showHeader ? (open ?? internalOpen) : true
  const model = useMemo(() => spec.build(), [spec])

  function handleToggleOpen() {
    const next = !isOpen
    if (onOpenChange) onOpenChange(next)
    else setInternalOpen(next)
  }

  function toggleFold(key: string) {
    setExpandedFolds(current => ({ ...current, [key]: !current[key] }))
  }

  return <section className="novel-diff-card" data-collapsed={!isOpen || undefined}>
    {showHeader && <button type="button" className="novel-diff-header" aria-expanded={isOpen} aria-controls={bodyId} onClick={handleToggleOpen}>
      <span className="novel-diff-header-main">
        <ChevronDown className="novel-diff-chevron" aria-hidden="true"/>
        <Code className="novel-diff-file-icon" aria-hidden="true"/>
        <strong className="novel-diff-title">{spec.title}</strong>
        {spec.subtitle && <span className="novel-diff-subtitle">{spec.subtitle}</span>}
        {spec.action && spec.action !== "UPDATE" && (
          <span className="novel-history-action-badge" data-action={spec.action}>{diffActionLabels[spec.action] || spec.action}</span>
        )}
      </span>
      {(model.added > 0 || model.removed > 0) && <span className="novel-diff-stats" aria-label={`新增 ${model.added} 行，删除 ${model.removed} 行`}>
        {model.added > 0 && <span className="novel-diff-stat-add">+{model.added}</span>}
        {model.removed > 0 && <span className="novel-diff-stat-del">−{model.removed}</span>}
      </span>}
    </button>}

    {isOpen && <div id={bodyId} className="novel-diff-body">
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
