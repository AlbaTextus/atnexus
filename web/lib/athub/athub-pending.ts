import { api } from "@/lib/api"
import type { IllustrationBlock, NovelIllustration } from "@/lib/athub/athub-client"
import { isIllustrationBlock, isPositiveNumber } from "@/lib/novel/illustration-layout"

// What a submission awaiting review does to one record of a novel. `after` holds
// what the altered fields become, or the record that is created. A removal has
// none, and neither has a submission queued before later values were kept.
export type PendingRecord = {
  entity: "Novel" | "Volume" | "Chapter" | "Illustration" | "IllustrationLayout"
  action: "INSERT" | "UPDATE" | "DELETE"
  id?: number
  fields?: string[]
  after?: Record<string, unknown>
}
// One submission awaiting review, as far as it alters the novel. It does not
// say who submitted it.
export type PendingChange = { id: string; created_at: string; changes: PendingRecord[] }
// One record of a submission, with the time the submission was made.
export type PendingEntry = { key: string; createdAt: string; record: PendingRecord }
export type PendingUpload = { key: string; createdAt: string; image: Pick<NovelIllustration, "url" | "width" | "height"> | null }
export type PendingLayout = { key: string; createdAt: string; blocks: IllustrationBlock[] | null }

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === "object" && value !== null && !Array.isArray(value)
function isPendingChange(value: unknown): value is PendingChange {
  return isObject(value) && typeof value.id === "string" && typeof value.created_at === "string" && Array.isArray(value.changes) &&
    value.changes.every(record => isObject(record) && typeof record.entity === "string" && typeof record.action === "string")
}

// Reads every submission awaiting review that alters the novel, oldest first.
// The server returns at most 50 at a time.
export async function readPendingChanges(novelId: number, signal: AbortSignal): Promise<PendingChange[]> {
  const changes: PendingChange[] = []
  const seen = new Set<string>()
  for (let offset = 0; ;) {
    const page = await api<unknown>(`/api/v4/account/athub/novels/${novelId}/pending-changes?offset=${offset}`, { signal, cache: "no-store" })
    if (!isObject(page) || !Array.isArray(page.items) || typeof page.total !== "number") throw new Error("待合并的改动数据格式不正确。")
    const items: unknown[] = page.items
    if (!items.every(isPendingChange)) throw new Error("待合并的改动数据格式不正确。")
    // The queue can move between two reads, and a submission must not be shown twice.
    for (const item of items) if (!seen.has(item.id)) { seen.add(item.id); changes.push(item) }
    offset += items.length
    if (!items.length || offset >= page.total) return changes
    signal.throwIfAborted()
  }
}

export const pendingAfter = (record: PendingRecord) => isObject(record.after) ? record.after : undefined

function entries(pending: PendingChange[], match: (record: PendingRecord) => boolean): PendingEntry[] {
  return pending.flatMap(change => change.changes.flatMap((record, index) =>
    match(record) ? [{ key: `${change.id}:${index}`, createdAt: change.created_at, record }] : []))
}

// The submissions that alter one field of the novel itself.
export function pendingFieldChanges(pending: PendingChange[], field: string) {
  return entries(pending, record => record.entity === "Novel" && record.action === "UPDATE" && Array.isArray(record.fields) && record.fields.includes(field))
}

export function pendingNovelDeletes(pending: PendingChange[]) {
  return entries(pending, record => record.entity === "Novel" && record.action === "DELETE")
}

// `image` and `blocks` are null when the submission did not keep what it adds.
export function pendingIllustrationUploads(pending: PendingChange[]): PendingUpload[] {
  return entries(pending, record => record.entity === "Illustration" && record.action === "INSERT").map(({ key, createdAt, record }) => {
    const { url, width, height } = pendingAfter(record) || {}
    return { key, createdAt, image: typeof url === "string" && isPositiveNumber(width) && isPositiveNumber(height) ? { url, width, height } : null }
  })
}

export function pendingIllustrationDeletes(pending: PendingChange[]) {
  const deletes = new Map<number, PendingEntry[]>()
  // A novel deletion is shown once, not as a separate removal on each illustration.
  const withoutNovelDeletes = pending.filter(change => !change.changes.some(record => record.entity === "Novel" && record.action === "DELETE"))
  for (const entry of entries(withoutNovelDeletes, record => record.entity === "Illustration" && record.action === "DELETE")) {
    const id = entry.record.id
    if (typeof id !== "number") continue
    const existing = deletes.get(id)
    if (existing) existing.push(entry)
    else deletes.set(id, [entry])
  }
  return deletes
}

export function pendingIllustrationLayouts(pending: PendingChange[]): PendingLayout[] {
  return entries(pending, record => record.entity === "IllustrationLayout" && (record.action === "INSERT" || record.action === "UPDATE")).map(({ key, createdAt, record }) => {
    const blocks = pendingAfter(record)?.blocks
    return { key, createdAt, blocks: Array.isArray(blocks) && blocks.every(isIllustrationBlock) ? blocks : null }
  })
}

const timeFormat = new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
export function pendingTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : timeFormat.format(date)
}
