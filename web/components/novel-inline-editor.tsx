"use client"

import { createContext, useContext, useCallback, useEffect, useRef, useState, type ReactNode } from "react"
import { useTransitionRouter } from "next-view-transitions"
import { Check, Loader2, Pencil, X } from "lucide-react"
import { useNovelAccess } from "@/components/novel-access"
import { NovelInlineSelect } from "@/components/novel-inline-select"
import { NovelAuthorField, NovelTagsField } from "@/components/novel-reference-fields"
import { NovelAutoHeight } from "@/components/novel-motion"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, ApiError } from "@/lib/api"
import { allowsEndpoint, type RolePolicy } from "@/lib/athub-permissions"
import { novelStatus, safeContentURL, type ContentAuthor, type ContentPlatform, type ContentTag, type Novel } from "@/lib/athub-client"
import { readAllContent, type DemoNovel } from "@/lib/novel-demo-data"

type Draft = Pick<Novel, "platformId" | "authorId" | "title" | "summary" | "coverUrl" | "status"> & {
  tagIds: number[]
  authorName: string | null
  tagNames: string[] | null
}
export type NovelEditField = Exclude<keyof Draft, "authorName" | "tagNames">
type Catalog = { book: Novel; authors: ContentAuthor[]; platforms: ContentPlatform[]; tags: ContentTag[] }
type EditResult = Novel | { status: "pending"; review_id: string }
const canEdit = (policy: RolePolicy) => allowsEndpoint(policy, "edit", "PUT", "/api/v2/novelinfo")
const canManage = (policy: RolePolicy) => canEdit(policy) ||
  allowsEndpoint(policy, "edit", "PUT", "/api/v2/illustration-layout") ||
  allowsEndpoint(policy, "upload", "POST", "/api/v2/illustrations") ||
  allowsEndpoint(policy, "delete", "DELETE", "/api/v2/illustrations")
const toDraft = (book: Novel): Draft => ({
  platformId: book.platformId, authorId: book.authorId, title: book.title,
  summary: book.summary, coverUrl: book.coverUrl, status: book.status, tagIds: [...book.tagIds || []].sort((a, b) => a - b),
  authorName: null, tagNames: null,
})
const labels: Record<NovelEditField, string> = {
  title: "作品名称", authorId: "作者", platformId: "来源平台", status: "连载状态",
  tagIds: "标签", summary: "简介", coverUrl: "封面链接",
}

type LayoutEditorActions = { dirty: boolean; busy: boolean; save: () => Promise<"saved" | "pending" | null>; cancel: () => void }
type Editor = {
  mode: boolean
  loading: boolean
  busy: boolean
  active: NovelEditField | null
  allowed: boolean
  policy: RolePolicy | null
  catalog: Catalog | null
  draft: Draft | null
  preview: Partial<DemoNovel> | null
  error: string
  notice: string
  dirty: boolean
  layoutDirty: boolean
  registerLayout: (actions: LayoutEditorActions | null) => void
  begin: (field?: NovelEditField) => void
  change: <K extends keyof Draft>(field: K, value: Draft[K]) => void
  closeField: (cancel?: boolean) => void
  cancel: () => void
  save: () => void
}
const Context = createContext<Editor | null>(null)
export const useNovelEditor = () => useContext(Context)

export function NovelEditorProvider({ id, children }: { id: number; children: ReactNode }) {
  const { request } = useNovelAccess()
  const [mode, setMode] = useState(false)
  const [policy, setPolicy] = useState<RolePolicy | null>(null)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [active, setActive] = useState<NovelEditField | null>(null)
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const starting = useRef(false)
  const submitting = useRef(false)
  const fieldSnapshot = useRef<Draft | null>(null)
  const controller = useRef<AbortController | null>(null)
  const layout = useRef<LayoutEditorActions | null>(null)
  const [layoutState, setLayoutState] = useState({ dirty: false, busy: false })
  const registerLayout = useCallback((actions: LayoutEditorActions | null) => {
    layout.current = actions
    const next = { dirty: Boolean(actions?.dirty), busy: Boolean(actions?.busy) }
    setLayoutState(previous => previous.dirty === next.dirty && previous.busy === next.busy ? previous : next)
  }, [])
  const metadataDirty = Boolean(draft && catalog && JSON.stringify(draft) !== JSON.stringify(toDraft(catalog.book)))
  const dirty = metadataDirty || layoutState.dirty

  useEffect(() => {
    if (!notice) return
    const timer = window.setTimeout(() => setNotice(""), 3000)
    return () => window.clearTimeout(timer)
  }, [notice])

  useEffect(() => {
    const abort = new AbortController()
    api<RolePolicy>("/api/v1/account/athub/permissions", { signal: abort.signal, cache: "no-store" })
      .then(value => { if (!abort.signal.aborted) setPolicy(value) })
      .catch(() => { /* Entering edit mode retries the permission read. */ })
    return () => { abort.abort(); controller.current?.abort() }
  }, [])

  useEffect(() => {
    if (!dirty) return
    const leave = (event: BeforeUnloadEvent) => { event.preventDefault() }
    window.addEventListener("beforeunload", leave)
    return () => window.removeEventListener("beforeunload", leave)
  }, [dirty])

  async function begin(field?: NovelEditField) {
    if (starting.current || submitting.current) return
    if (mode && draft && catalog) {
      if (field && policy && canEdit(policy)) {
        fieldSnapshot.current = draft
        setActive(field)
      }
      return
    }
    starting.current = true
    const abort = new AbortController()
    controller.current = abort
    setMode(true); setLoading(true); setError(""); setNotice("")
    try {
      const permissions = await api<RolePolicy>("/api/v1/account/athub/permissions", { signal: abort.signal, cache: "no-store" })
      if (abort.signal.aborted) return
      setPolicy(permissions)
      if (!canManage(permissions)) throw new Error("当前账户没有可用的编辑权限。")
      if (field && !canEdit(permissions)) throw new Error("当前账户没有编辑作品资料的权限。")
      if (!canEdit(permissions)) return
      const [book, authors, platforms, tags] = await Promise.all([
        request<Novel>(`/api/v5/novelinfo?id=${id}`, abort.signal),
        readAllContent<ContentAuthor>(request, "/api/v5/authors", abort.signal),
        readAllContent<ContentPlatform>(request, "/api/v5/platforms", abort.signal),
        readAllContent<ContentTag>(request, "/api/v5/tags", abort.signal),
      ])
      if (abort.signal.aborted) return
      const value = toDraft(book)
      setCatalog({ book, authors, platforms, tags }); setDraft(value)
      fieldSnapshot.current = value
      setActive(field ?? null)
    } catch (reason) {
      if (!abort.signal.aborted) setError(reason instanceof Error ? reason.message : "作品资料加载失败，请重试。")
    } finally {
      starting.current = false
      if (!abort.signal.aborted) setLoading(false)
    }
  }

  function change<K extends keyof Draft>(field: K, value: Draft[K]) {
    if (submitting.current) return
    setDraft(current => current && {
      ...current, [field]: value,
      ...(field === "platformId" && value !== current.platformId ? { authorId: null, authorName: null } : {}),
      ...(field === "authorName" ? { authorId: null } : {}),
      ...(field === "tagNames" ? { tagIds: [] } : {}),
    })
    setError(""); setNotice("")
  }
  function closeField(cancel = false) {
    if (cancel && active && fieldSnapshot.current) {
      const snapshot = fieldSnapshot.current
      setDraft(current => current && {
        ...current, [active]: snapshot[active],
        ...(active === "platformId" || active === "authorId" ? { authorId: snapshot.authorId, authorName: snapshot.authorName } : {}),
        ...(active === "tagIds" ? { tagNames: snapshot.tagNames } : {}),
      })
    }
    setActive(null)
  }
  function cancel() {
    if (submitting.current || layout.current?.busy) return
    layout.current?.cancel()
    controller.current?.abort()
    setMode(false); setActive(null); setDraft(null); setCatalog(null); setLoading(false); setError("")
  }
  async function save() {
    if (submitting.current || layout.current?.busy) return
    let layoutPending = false
    if (layout.current?.dirty) {
      submitting.current = true; setBusy(true)
      let outcome: "saved" | "pending" | null
      try { outcome = await layout.current.save() }
      finally { submitting.current = false; setBusy(false) }
      if (!outcome) return
      layoutPending = outcome === "pending"
      if (!metadataDirty) {
        setNotice(layoutPending ? "插图布局已提交审核，通过后生效。" : "插图布局已保存。")
        setMode(false); setActive(null); setDraft(null); setCatalog(null)
        return
      }
    }
    if (!draft || !catalog || !policy || !canEdit(policy) || !metadataDirty) return
    setError("")
    const { authorName, tagNames, ...metadata } = draft
    const payload = {
      ...metadata, title: draft.title.trim(), summary: draft.summary?.trim() || null, coverUrl: draft.coverUrl?.trim() || null,
      ...(authorName !== null ? { authorName: authorName.trim() || null } : {}),
      ...(tagNames !== null ? { tagNames: [...new Set(tagNames.map(name => name.trim()).filter(Boolean))] } : {}),
    }
    if (!payload.title) { setError("请填写作品名称。"); setActive("title"); return }
    if (!catalog.platforms.some(platform => platform.id === payload.platformId)) { setError("请选择有效的来源平台。"); setActive("platformId"); return }
    if (payload.authorId !== null && !catalog.authors.some(author => author.id === payload.authorId && author.platformId === payload.platformId)) {
      setError("请选择来源平台下的作者。"); setActive("authorId"); return
    }
    if (payload.tagIds.some(tagId => !catalog.tags.some(tag => tag.id === tagId))) { setError("部分标签已不可用，请重新选择。"); setActive("tagIds"); return }
    if (payload.authorName && (Array.from(payload.authorName).length > 200 || payload.authorName.includes("\0"))) {
      setError("作者名字不能超过 200 个字符，且不能包含无效字符。"); setActive("authorId"); return
    }
    if (payload.tagNames?.some(name => name.includes("\0"))) { setError("标签中含有无效字符，请移除后重试。"); setActive("tagIds"); return }
    if (payload.coverUrl && !safeContentURL(payload.coverUrl)) { setError("封面链接需要使用有效的 HTTP 或 HTTPS 地址。"); setActive("coverUrl"); return }
    if ([payload.title, payload.summary, payload.coverUrl].some(value => value?.includes("\0"))) { setError("资料中含有无效字符，请移除后重试。"); return }
    const body = JSON.stringify(payload)
    if (new TextEncoder().encode(body).length > 8 * 1024 * 1024) { setError("作品资料超过 8 MiB，请缩短后重试。"); return }
    submitting.current = true; setBusy(true)
    try {
      const response = await api<EditResult>(`/api/v4/account/athub/novels/${id}`, { method: "PUT", body })
      if (!response || !("id" in response && response.id === id) &&
        !("status" in response && response.status === "pending" && response.review_id)) {
        throw new Error("未能确认提交结果，请刷新查看后再操作。")
      }
      setNotice("review_id" in response || layoutPending ? "已提交审核，通过后生效。" : "作品资料已保存。")
      setMode(false); setActive(null); setDraft(null); setCatalog(null)
    } catch (reason) {
      setError(reason instanceof ApiError && reason.status === 403 ? "当前账户没有编辑权限，或权限已发生变化。"
        : reason instanceof ApiError && reason.status === 401 ? "登录已失效，请重新登录后再提交。"
        : reason instanceof Error ? reason.message : "未能确认提交结果，请刷新查看后再操作。")
    } finally {
      submitting.current = false; setBusy(false)
    }
  }
  const author = catalog?.authors.find(item => item.id === draft?.authorId && item.platformId === draft?.platformId)
  const preview: Partial<DemoNovel> | null = mode && draft && catalog ? {
    title: draft.title,
    author: draft.authorName !== null ? draft.authorName.trim() || "待补充" : author?.name || (draft.authorId === null ? "待补充" : `作者 #${draft.authorId}`),
    authorId: draft.authorId,
    cover: safeContentURL(draft.coverUrl),
    status: novelStatus(draft.status),
    publisher: catalog.platforms.find(item => item.id === draft.platformId)?.name,
    tags: draft.tagNames !== null ? [...new Set(draft.tagNames.map(name => name.trim()).filter(Boolean))]
      : draft.tagIds.map(tagId => catalog.tags.find(tag => tag.id === tagId)?.name || `标签 #${tagId}`),
    synopsis: draft.summary?.split(/\r?\n/).filter(line => line.trim()) || [],
  } : null
  return <Context.Provider value={{
    mode, loading, busy: busy || layoutState.busy, active, allowed: Boolean(policy && canEdit(policy)), policy, catalog, draft, preview,
    error, notice, dirty, layoutDirty: layoutState.dirty, registerLayout, begin, change, closeField, cancel, save,
  }}>{children}</Context.Provider>
}

export function NovelEdit() {
  const editor = useNovelEditor()
  if (!editor) return null
  const denied = Boolean(editor.policy && !canManage(editor.policy))
  return <Tooltip>
    <TooltipTrigger render={<span className="inline-flex" />}>
      <Button variant="ghost" size="icon-sm" className="text-red-500 hover:bg-red-500/10 hover:text-red-600"
        aria-label="编辑作品资料" aria-pressed={editor.mode} disabled={editor.loading || editor.busy || denied}
        onClick={() => editor.begin()}>
        <Pencil className="size-4" aria-hidden="true" />
      </Button>
    </TooltipTrigger>
    <TooltipContent>{denied ? "当前账户没有可用的编辑权限" : "进入编辑模式"}</TooltipContent>
  </Tooltip>
}

export function NovelEditToolbar() {
  const editor = useNovelEditor()
  if (!editor) return null
  return <NovelAutoHeight open={editor.mode || Boolean(editor.notice)} className="novel-inline-toolbar-motion">
  <div className="novel-inline-toolbar" aria-label="作品编辑操作">
    <div className="novel-inline-feedback">
      {editor.mode && <span>{editor.loading ? "正在加载编辑资料…" : "编辑模式"}</span>}
      {editor.error && <span role="alert">{editor.error} {!editor.draft && !editor.loading && <Button variant="ghost" size="sm" onClick={() => editor.begin()}>重试</Button>}</span>}
      {editor.notice && <span role="status">{editor.notice}</span>}
    </div>
    {editor.mode && <div className="flex shrink-0 items-center gap-2">
      <Button variant="ghost" size="sm" disabled={editor.busy} onClick={editor.cancel}>{editor.dirty ? "取消修改" : "退出编辑"}</Button>
      {(editor.allowed || editor.layoutDirty) && <Button size="sm" disabled={editor.loading || editor.busy || !editor.dirty} onClick={editor.save}>
        {editor.busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
        {editor.busy ? "正在提交…" : editor.policy?.review_required ? "提交审核" : "保存修改"}
      </Button>}
    </div>}
  </div>
  </NovelAutoHeight>
}

export function NovelEditable({ field, children, block = false, viewAction }: {
  field: NovelEditField; children: ReactNode; block?: boolean; viewAction?: () => void;
}) {
  const editor = useNovelEditor()
  const router = useTransitionRouter()
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const host = useRef<HTMLDivElement>(null)
  const enabled = Boolean(editor && (!editor.policy || editor.allowed))
  const editable = Boolean(editor?.mode && editor.allowed && !editor.loading && editor.draft)
  const Element = block ? "div" : "span"
  const active = Boolean(editor?.mode && editor.active === field && editor.draft && editor.catalog)
  useEffect(() => {
    if (active) host.current?.querySelector<HTMLElement>("input, textarea, select, button")?.focus({ preventScroll: true })
  }, [active])
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])
  if (!editor) return children
  const start = () => { if (timer.current) clearTimeout(timer.current); editor.begin(field) }
  return <Element ref={host} className={`novel-inline-field${block ? " is-block" : ""}`}
    data-editable={editable || undefined} data-active={active || undefined}
    role={!active && editable ? "button" : undefined}
    tabIndex={!active && editable ? 0 : undefined}
    aria-label={!active && editable ? `编辑${labels[field]}` : undefined}
    onClickCapture={event => {
      if (active || editor.busy || editor.loading) return
      if (enabled && editor.mode) { event.preventDefault(); event.stopPropagation(); start(); return }
      const link = (event.target as HTMLElement).closest("a")
      if (enabled && !event.ctrlKey && !event.metaKey && !event.shiftKey && (link || viewAction)) {
        event.preventDefault(); event.stopPropagation()
        if (timer.current) clearTimeout(timer.current)
        const href = link?.getAttribute("href")
        timer.current = setTimeout(() => { if (href) router.push(href); else viewAction?.() }, 280)
      }
    }}
    onDoubleClick={event => { if (enabled && !active) { event.preventDefault(); event.stopPropagation(); start() } }}
    onKeyDown={event => {
      if (active && event.key === "Escape") { event.preventDefault(); event.stopPropagation(); editor.closeField(true) }
      else if (active && !event.nativeEvent.isComposing && event.key === "Enter" && (field !== "summary" && event.target instanceof HTMLInputElement || event.ctrlKey || event.metaKey)) {
        event.preventDefault(); editor.closeField()
      } else if (!active && enabled && editor.mode && (event.key === "Enter" || event.key === " ")) {
        event.preventDefault(); start()
      }
    }}>
    <NovelAutoHeight inline>
    {active ? <span className="novel-inline-control novel-motion-enter">
      <FieldControl field={field} editor={editor} />
      <span className="novel-inline-field-actions">
        <Button variant="ghost" size="icon-sm" aria-label={`完成编辑${labels[field]}`} disabled={editor.busy} onClick={() => editor.closeField()}><Check className="size-4" aria-hidden="true" /></Button>
        <Button variant="ghost" size="icon-sm" aria-label={`取消编辑${labels[field]}`} disabled={editor.busy} onClick={() => editor.closeField(true)}><X className="size-4" aria-hidden="true" /></Button>
      </span>
    </span> : <span className="novel-inline-display">{children}</span>}
    </NovelAutoHeight>
  </Element>
}

function FieldControl({ field, editor }: { field: NovelEditField; editor: Editor }) {
  const { draft, catalog, busy, change } = editor
  if (!draft || !catalog) return null
  const props = { "aria-label": labels[field], disabled: busy }
  switch (field) {
    case "title": return <Input {...props} value={draft.title} required onChange={event => change(field, event.target.value)} />
    case "summary": return <Textarea {...props} rows={8} value={draft.summary || ""} onChange={event => change(field, event.target.value)} />
    case "coverUrl": return <Input {...props} type="url" value={draft.coverUrl || ""} onChange={event => change(field, event.target.value)} />
    case "platformId": return <NovelInlineSelect label={labels[field]} disabled={busy} value={String(draft.platformId)}
      items={catalog.platforms.map(item => ({ value: String(item.id), label: item.name }))}
      onValueChange={value => change(field, Number(value))} />
    case "authorId": return <NovelAuthorField disabled={busy}
      value={draft.authorName ?? catalog.authors.find(item => item.id === draft.authorId)?.name ?? ""}
      authors={catalog.authors}
      onChange={value => change("authorName", value)} />
    case "status": return <NovelInlineSelect label={labels[field]} disabled={busy} value={draft.status}
      items={[{ value: "ONGOING", label: "连载中" }, { value: "COMPLETED", label: "已完结" }, { value: "ABANDONED", label: "太监" }]}
      onValueChange={value => change(field, value as Novel["status"])} />
    case "tagIds": return <NovelTagsField disabled={busy} tags={catalog.tags}
      values={draft.tagNames ?? draft.tagIds.map(id => catalog.tags.find(tag => tag.id === id)?.name || "")}
      onChange={values => change("tagNames", values)} />
  }
}
