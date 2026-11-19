"use client"

import { useMemo, type ReactNode } from "react"
import { Clock3 } from "lucide-react"
import { IllustrationImage } from "@/components/novel/illustration-image"
import { IllustrationLayoutPreview } from "@/components/novel/illustration-layout-preview"
import { NovelCover } from "@/components/novel/novel-cover"
import { UnifiedDiffCard, buildTextDiffModel } from "@/components/novel/novel-diff-card"
import { novelFieldLabels, useNovelEditor, useNovelPending, type NovelEditField } from "@/components/novel/novel-inline-editor"
import { Badge } from "@/components/novel/ui/badge"
import { novelStatus, safeContentURL, type Novel, type NovelIllustration } from "@/lib/athub/athub-client"
import { pendingAfter, pendingIllustrationLayouts, pendingIllustrationUploads, pendingTime, type PendingRecord } from "@/lib/athub/athub-pending"

// Submissions awaiting review are listed, while the novel is edited, next to
// what they alter: when each was made and what it makes of that place. Nothing
// here tells who made them, and nothing here keeps the place from being edited.
function PendingBlock({ title, label = title, row = false, children }: { title: string; label?: string; row?: boolean; children: ReactNode }) {
  return <div className="novel-pending" role="group" aria-label={label}>
    <p className="novel-pending-title"><Clock3 aria-hidden="true" />{title}</p>
    <ul className={`novel-pending-list${row ? " is-row" : ""}`}>{children}</ul>
  </div>
}
function PendingItem({ createdAt, className, children }: { createdAt: string; className?: string; children: ReactNode }) {
  return <li className={className}><time dateTime={createdAt}>{pendingTime(createdAt)}</time>{children}</li>
}
// Said in place of a value: a field that is emptied, or a submission that did
// not keep what it changes.
function PendingNote({ children }: { children: string }) {
  return <span className="novel-pending-note">{children}</span>
}
const unrecorded = <PendingNote>改动内容未记录</PendingNote>

// A text of a record: null when the submission empties it, undefined when the
// submission did not keep it.
function recordText(after: Record<string, unknown>, name: string) {
  const value = after[name]
  return typeof value === "string" ? value.trim() || null : value === null ? null : undefined
}

function PendingSummaryDiff({
  beforeText,
  afterText,
  cleared,
}: {
  beforeText: string
  afterText: string
  cleared: boolean
}) {
  const spec = useMemo(() => ({
    title: "简介",
    subtitle: "小说条目",
    action: cleared ? "DELETE" : undefined,
    build: () => buildTextDiffModel(beforeText, afterText),
  }), [beforeText, afterText, cleared])

  return <UnifiedDiffCard spec={spec} showHeader={false} />
}

// What a submission makes of one field. An author, a platform and tags are
// named as the submission names them: one it creates has no ID in the library.
function PendingValue({ field, record, current }: { field: NovelEditField; record: PendingRecord; current?: string }) {
  const editor = useNovelEditor()
  const after = pendingAfter(record)
  if (!after) return unrecorded
  switch (field) {
    case "title": return recordText(after, "title") || unrecorded
    case "platformId": return recordText(after, "platformName") || unrecorded
    case "summary": {
      const summary = recordText(after, "summary")
      if (summary === undefined) return unrecorded
      const beforeText = (editor?.catalog?.book?.summary ?? current ?? "").trim()
      if (summary === null) {
        return beforeText ? <PendingSummaryDiff beforeText={beforeText} afterText="" cleared /> : <PendingNote>清空简介</PendingNote>
      }
      return <PendingSummaryDiff beforeText={beforeText} afterText={summary} cleared={false} />
    }
    case "coverUrl": {
      const cover = recordText(after, "coverUrl")
      return cover === undefined ? unrecorded : cover === null ? <PendingNote>移除封面</PendingNote>
        : <span className="novel-pending-cover"><NovelCover src={safeContentURL(cover)} title="待合并的封面" fallback="图片加载失败" /></span>
    }
    case "authorId": {
      const name = recordText(after, "authorName")
      if (name) return name
      if (after.authorId === null) return <PendingNote>清空作者</PendingNote>
      // Only an author the library already has can be without a name.
      return name === null && typeof after.authorId === "number" && Number.isSafeInteger(after.authorId) && after.authorId > 0 ? `作者 #${after.authorId}` : unrecorded
    }
    case "status": {
      const label: unknown = typeof after.status === "string" ? novelStatus(after.status as Novel["status"]) : undefined
      return typeof label === "string" ? label : unrecorded
    }
    case "tagIds": {
      const names: unknown = after.tagNames
      if (!Array.isArray(names) || !names.every((name): name is string => typeof name === "string")) return unrecorded
      return names.length ? <span className="tag-cloud">{names.map((name, index) => <Badge variant="secondary" key={index}>{name}</Badge>)}</span>
        : <PendingNote>清空标签</PendingNote>
    }
  }
}

export function NovelPendingChanges({ field, current }: { field: NovelEditField; current?: string }) {
  const entries = useNovelPending(field)
  if (!entries.length) return null
  return <PendingBlock title="待合并的改动" label={`${novelFieldLabels[field]}：待合并的改动`}>
    {entries.map(({ key, createdAt, record }) => <PendingItem key={key} createdAt={createdAt}>
      <div className="novel-pending-value"><PendingValue field={field} record={record} current={current} /></div>
    </PendingItem>)}
  </PendingBlock>
}

// Illustrations that submissions add. They are not the novel's yet, so they
// are listed apart from its own.
export function NovelPendingUploads() {
  const editor = useNovelEditor()
  const uploads = editor?.mode ? pendingIllustrationUploads(editor.pending) : []
  if (!uploads.length) return null
  return <PendingBlock title="待合并的上传" row>
    {uploads.map(({ key, createdAt, image }) => <PendingItem key={key} createdAt={createdAt} className="novel-pending-upload">
      {image ? <div className="novel-pending-upload-image"><IllustrationImage image={image} label="待合并的插图" /></div> : unrecorded}
    </PendingItem>)}
  </PendingBlock>
}

// Layouts that submissions put in place of the canvas, drawn with the novel's
// illustrations by ID.
export function NovelPendingLayouts({ images }: { images: ReadonlyMap<number, Pick<NovelIllustration, "url" | "width" | "height">> }) {
  const editor = useNovelEditor()
  const layouts = editor?.mode ? pendingIllustrationLayouts(editor.pending) : []
  if (!layouts.length) return null
  return <PendingBlock title="待合并的布局" row>
    {layouts.map(({ key, createdAt, blocks }) => <PendingItem key={key} createdAt={createdAt} className="novel-pending-layout">
      {blocks ? <IllustrationLayoutPreview blocks={blocks} images={images} label={`待合并的插图布局 ${pendingTime(createdAt)}`} /> : unrecorded}
    </PendingItem>)}
  </PendingBlock>
}
