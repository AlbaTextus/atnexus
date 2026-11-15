"use client"

import { useEffect, useRef, useState, type FormEvent } from "react"
import { Check, CircleCheck, Clock3, Loader2, Pencil } from "lucide-react"
import { useNovelAccess } from "@/components/novel-access"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, ApiError } from "@/lib/api"
import { allowsEndpoint, type RolePolicy } from "@/lib/athub-permissions"
import { safeContentURL, type ContentAuthor, type ContentPlatform, type ContentTag, type Novel } from "@/lib/athub-client"
import { readAllContent } from "@/lib/novel-demo-data"

type Draft = Pick<Novel, "platformId" | "authorId" | "title" | "summary" | "coverUrl" | "status"> & { tagIds: number[] }
type Catalog = { book: Novel; authors: ContentAuthor[]; platforms: ContentPlatform[]; tags: ContentTag[] }
type EditResult = Novel | { status: "pending"; review_id: string }
const canEdit = (policy: RolePolicy) => allowsEndpoint(policy, "edit", "PUT", "/api/v2/novelinfo")
const toDraft = (book: Novel): Draft => ({
  platformId: book.platformId, authorId: book.authorId, title: book.title,
  summary: book.summary, coverUrl: book.coverUrl, status: book.status, tagIds: [...book.tagIds || []].sort((a, b) => a - b),
})
const selectClass = "h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"

export function NovelEdit({ id }: { id: number }) {
  const { request } = useNovelAccess()
  const [open, setOpen] = useState(false)
  const [policy, setPolicy] = useState<RolePolicy | null>(null)
  const [checking, setChecking] = useState(true)
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [loadError, setLoadError] = useState("")
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const [result, setResult] = useState<EditResult | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    api<RolePolicy>("/api/v1/account/athub/permissions", { signal: controller.signal, cache: "no-store" })
      .then(value => { if (!controller.signal.aborted) setPolicy(value) })
      .catch(() => { /* Opening the editor provides a retry for failed permission reads. */ })
      .finally(() => { if (!controller.signal.aborted) setChecking(false) })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!open || result) return
    const controller = new AbortController()
    setCatalog(null); setDraft(null); setLoadError(""); setError("")
    async function load() {
      const permissions = await api<RolePolicy>("/api/v1/account/athub/permissions", { signal: controller.signal, cache: "no-store" })
      if (controller.signal.aborted) return
      setPolicy(permissions)
      if (!canEdit(permissions)) { setLoadError("当前账户没有编辑作品资料的权限。"); return }
      const [book, authors, platforms, tags] = await Promise.all([
        request<Novel>(`/api/v2/novelinfo?id=${id}`, controller.signal),
        readAllContent<ContentAuthor>(request, "/api/v2/authors", controller.signal),
        readAllContent<ContentPlatform>(request, "/api/v2/platforms", controller.signal),
        readAllContent<ContentTag>(request, "/api/v2/tags", controller.signal),
      ])
      if (!controller.signal.aborted) {
        setCatalog({ book, authors, platforms, tags })
        setDraft(toDraft(book))
      }
    }
    void load().catch(reason => {
      if (!controller.signal.aborted) setLoadError(reason instanceof Error ? reason.message : "作品资料加载失败。")
    })
    return () => controller.abort()
  }, [open, id, request, revision, result])

  function change<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft(current => current && { ...current, [key]: value })
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitting.current || !draft || !catalog || !policy || !canEdit(policy) || result) return
    setError("")
    const payload: Draft = { ...draft, title: draft.title.trim(), summary: draft.summary?.trim() || null, coverUrl: draft.coverUrl?.trim() || null }
    if (!payload.title) { setError("请填写作品名称。"); return }
    if (!catalog.platforms.some(platform => platform.id === payload.platformId)) { setError("请选择有效的来源平台。"); return }
    if (payload.authorId !== null && !catalog.authors.some(author => author.id === payload.authorId && author.platformId === payload.platformId)) {
      setError("请选择来源平台下的作者。"); return
    }
    if (payload.coverUrl && !safeContentURL(payload.coverUrl)) { setError("封面链接需要使用有效的 HTTP 或 HTTPS 地址。"); return }
    if ([payload.title, payload.summary, payload.coverUrl].some(value => value?.includes("\0"))) { setError("资料中含有无效字符，请移除后重试。"); return }
    const body = JSON.stringify(payload)
    if (new TextEncoder().encode(body).length > 8 * 1024 * 1024) { setError("作品资料超过 8 MiB，请缩短后重试。"); return }
    submitting.current = true; setBusy(true)
    try {
      const response = await api<EditResult>(`/api/v1/account/athub/novels/${id}`, { method: "PUT", body })
      if (!response || !("id" in response && response.id === id) &&
        !("status" in response && response.status === "pending" && "review_id" in response && response.review_id)) {
        throw new Error("未能确认提交结果，请稍后确认作品资料后再操作。")
      }
      setResult(response)
    } catch (reason) {
      setError(reason instanceof ApiError
        ? reason.status === 403 ? "当前账户没有编辑权限，或权限已发生变化。"
          : reason.status === 401 ? "登录已失效，请重新登录后再提交。"
            : reason.status === 400 ? "作品资料未通过校验，请检查填写内容。"
              : reason.status === 404 ? "作品已不存在，请刷新书库。"
                : reason.status === 429 ? "请求过于频繁，请稍后重试。"
                  : reason.message
        : "未能确认提交结果，请稍后确认作品资料后再操作，避免重复提交。")
    } finally {
      submitting.current = false; setBusy(false)
    }
  }

  const denied = Boolean(policy && !canEdit(policy))
  const pending = result && "review_id" in result
  const dirty = Boolean(draft && catalog && JSON.stringify(draft) !== JSON.stringify(toDraft(catalog.book)))
  return <>
    <Tooltip>
      <TooltipTrigger render={<span className="inline-flex" />}>
        <Button variant="outline" size="sm" disabled={checking || denied} onClick={() => { setResult(null); setOpen(true) }}>
          <Pencil aria-hidden="true" />编辑
        </Button>
      </TooltipTrigger>
      <TooltipContent>{checking ? "正在检查编辑权限…" : denied ? "当前账户没有编辑作品资料的权限" : "编辑作品资料"}</TooltipContent>
    </Tooltip>
    <Dialog open={open} onOpenChange={value => { if (!submitting.current) setOpen(value) }}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>{result ? pending ? "已提交审核" : "作品资料已更新" : "编辑作品资料"}</DialogTitle>
          <DialogDescription>{result ? pending ? "审核通过后生效，当前作品资料保持原样。" : "修改已生效并记录到作品变更历史。"
            : policy?.review_required ? "修改需审核通过后生效。" : "修改将按当前账户权限保存。"}</DialogDescription>
        </DialogHeader>
        {result ? <>
          <div className="flex items-center gap-3 py-5" role="status">
            {pending ? <Clock3 aria-hidden="true" className="size-6 text-amber-600" /> : <CircleCheck aria-hidden="true" className="size-6 text-emerald-600" />}
            <p>{draft?.title}</p>
          </div>
          <DialogFooter><Button onClick={() => setOpen(false)}>完成</Button></DialogFooter>
        </> : loadError ? <>
          <Alert variant="destructive"><AlertDescription>{loadError}</AlertDescription></Alert>
          <DialogFooter><Button variant="outline" onClick={() => setOpen(false)}>关闭</Button><Button onClick={() => setRevision(value => value + 1)}>重试</Button></DialogFooter>
        </> : !draft || !catalog ? <div className="flex items-center gap-2 py-8" role="status"><Loader2 aria-hidden="true" className="size-4 animate-spin" />正在加载作品资料…</div>
          : <form onSubmit={submit} className="space-y-5" aria-busy={busy}>
            <fieldset disabled={busy} className="grid grid-cols-2 gap-4">
              <label className="col-span-2 space-y-2 text-sm" htmlFor="edit-novel-title"><span>作品名称</span><Input id="edit-novel-title" aria-label="作品名称" required value={draft.title} onChange={event => change("title", event.target.value)} /></label>
              <label className="space-y-2 text-sm" htmlFor="edit-novel-platform"><span>来源平台</span>
                <select className={selectClass} id="edit-novel-platform" aria-label="来源平台" value={draft.platformId} onChange={event => {
                  setDraft(current => current && { ...current, platformId: Number(event.target.value), authorId: null })
                }}>{catalog.platforms.map(platform => <option key={platform.id} value={platform.id}>{platform.name}</option>)}</select>
              </label>
              <label className="space-y-2 text-sm" htmlFor="edit-novel-author"><span>作者</span>
                <select className={selectClass} id="edit-novel-author" aria-label="作者" value={draft.authorId ?? ""} onChange={event => change("authorId", event.target.value ? Number(event.target.value) : null)}>
                  <option value="">不关联作者</option>{catalog.authors.filter(author => author.platformId === draft.platformId).map(author => <option key={author.id} value={author.id}>{author.name || `作者 #${author.id}`}</option>)}
                </select>
              </label>
              <label className="space-y-2 text-sm" htmlFor="edit-novel-status"><span>连载状态</span>
                <select className={selectClass} id="edit-novel-status" aria-label="连载状态" value={draft.status} onChange={event => change("status", event.target.value as Novel["status"])}><option value="ONGOING">连载中</option><option value="COMPLETED">已完结</option></select>
              </label>
              <label className="col-span-2 space-y-2 text-sm" htmlFor="edit-novel-cover"><span>封面链接</span><Input id="edit-novel-cover" aria-label="封面链接" type="url" value={draft.coverUrl || ""} onChange={event => change("coverUrl", event.target.value)} /></label>
              <label className="col-span-2 space-y-2 text-sm" htmlFor="edit-novel-summary"><span>简介</span><Textarea id="edit-novel-summary" aria-label="简介" rows={6} value={draft.summary || ""} onChange={event => change("summary", event.target.value)} /></label>
              {catalog.tags.length > 0 && <fieldset className="col-span-2 space-y-2"><legend className="mb-2 text-sm">标签</legend>
                <div className="flex max-h-36 flex-wrap gap-2 overflow-y-auto">{catalog.tags.map(tag => <Button type="button" key={tag.id} size="sm" variant={draft.tagIds.includes(tag.id) ? "secondary" : "outline"}
                  aria-pressed={draft.tagIds.includes(tag.id)} onClick={() => change("tagIds", (draft.tagIds.includes(tag.id) ? draft.tagIds.filter(value => value !== tag.id) : [...draft.tagIds, tag.id]).sort((a, b) => a - b))}>
                  {draft.tagIds.includes(tag.id) && <Check aria-hidden="true" />}{tag.name}
                </Button>)}</div>
              </fieldset>}
            </fieldset>
            {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
            <DialogFooter>
              <Button type="button" variant="outline" disabled={busy} onClick={() => setOpen(false)}>取消</Button>
              <Button type="submit" disabled={busy || !dirty || !policy || !canEdit(policy)}>{busy && <Loader2 aria-hidden="true" className="animate-spin" />}{busy ? "正在提交…" : policy?.review_required ? "提交审核" : "保存修改"}</Button>
            </DialogFooter>
          </form>}
      </DialogContent>
    </Dialog>
  </>
}
