"use client"

import { useEffect, useState } from "react"
import { Link } from "next-view-transitions"
import { ArrowLeft, Check, Circle, CircleCheck, CircleX, ClipboardCheck, Clock3, FileClock, RefreshCw, TriangleAlert, X } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import type { IllustrationBlock, NovelIllustration } from "@/lib/athub/athub-client"
import { contentErrorMessage } from "@/lib/athub/athub-errors"
import { type ReviewItem, type RolePolicy, allowsEndpoint, endpointNames, hasOperation, operationNames } from "@/lib/athub/athub-permissions"
import { ILLUSTRATION_CANVAS_WIDTH as WIDTH } from "@/lib/novel/illustration-layout"
import { readAllContent } from "@/lib/novel/novel-view-models"
import type { AccountUser } from "@/components/account/account-shell"
import { IllustrationImage } from "@/components/novel/illustration-image"
import { useNovelAccess } from "@/components/novel/novel-access"
import { UserAvatar } from "@/components/shared/user-avatar"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"

// Reviewers see every submission and decide it; a submitter sees only their
// own and can only read them.
const views = {
  reviews: {
    title: "审核中心", icon: ClipboardCheck, endpoint: "/api/v4/admin/athub/reviews",
    login: "请先登录后查看审核中心。", list: "审核提交列表", refresh: "刷新审核列表", pages: "审核列表分页",
  },
  submissions: {
    title: "我的提交", icon: FileClock, endpoint: "/api/v4/account/athub/submissions",
    login: "请先登录后查看我的提交。", list: "我的提交列表", refresh: "刷新提交列表", pages: "提交列表分页",
  },
} as const
const statuses = { pending: "待审核", approved: "已通过", rejected: "已驳回", failed: "生效失败" } as const
const statusStyles = {
  pending: {
    icon: Clock3,
    color: "text-amber-800 dark:text-amber-400",
    badge: "border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950",
    indicator: "[&>[data-slot=toggle-group-indicator]]:bg-amber-100 dark:[&>[data-slot=toggle-group-indicator]]:bg-amber-950",
  },
  approved: {
    icon: CircleCheck,
    color: "text-emerald-700 dark:text-emerald-400",
    badge: "border-emerald-200 bg-emerald-50 dark:border-emerald-800 dark:bg-emerald-950",
    indicator: "[&>[data-slot=toggle-group-indicator]]:bg-emerald-100 dark:[&>[data-slot=toggle-group-indicator]]:bg-emerald-950",
  },
  rejected: {
    icon: CircleX,
    color: "text-red-700 dark:text-red-400",
    badge: "border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-950",
    indicator: "[&>[data-slot=toggle-group-indicator]]:bg-red-100 dark:[&>[data-slot=toggle-group-indicator]]:bg-red-950",
  },
  failed: {
    icon: TriangleAlert,
    color: "text-orange-700 dark:text-orange-400",
    badge: "border-orange-200 bg-orange-50 dark:border-orange-800 dark:bg-orange-950",
    indicator: "[&>[data-slot=toggle-group-indicator]]:bg-orange-100 dark:[&>[data-slot=toggle-group-indicator]]:bg-orange-950",
  },
} as const
const filterNames = { all: "全部", ...statuses } as const
const filterStyles = { all: { icon: Circle, color: "text-muted-foreground", indicator: "" }, ...statusStyles }
type ReviewFilter = keyof typeof filterNames
function ReviewStatusLabel({ status }: { status: ReviewItem["status"] }) {
  const { icon: Icon, color } = statusStyles[status]
  return <span className={`inline-flex items-center gap-1.5 ${color}`}><Icon aria-hidden="true" className="size-4 shrink-0" />{statuses[status]}</span>
}
function ReviewStatusBadge({ status }: { status: ReviewItem["status"] }) {
  return <Badge variant="outline" className={`h-6 ${statusStyles[status].badge}`}><ReviewStatusLabel status={status} /></Badge>
}
const approveButton = "bg-emerald-600 text-white hover:bg-emerald-700 dark:bg-emerald-600 dark:hover:bg-emerald-500"
const rejectButton = "border-red-200 text-red-700 hover:bg-red-50 hover:text-red-800 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950 dark:hover:text-red-300"
const fields: Record<string, string> = {
  id: "编号", novelId: "小说编号", novel: "小说资料", platformId: "平台编号", authorId: "作者编号", authorName: "作者名称",
  tagIds: "标签", tagNames: "标签名称", volumes: "分卷", chapters: "章节", volumeId: "分卷编号", volumeOrder: "分卷顺序",
  order: "顺序", title: "标题", name: "名称", summary: "简介", coverUrl: "封面", url: "地址",
  status: "连载状态", content: "正文", sha256: "校验值", width: "宽度", height: "高度",
  revision: "布局版本", blocks: "图片块", imageId: "插图编号", x: "横向位置", y: "纵向位置",
}
function SubmittedValue({ name, value, depth = 0 }: { name: string; value: unknown; depth?: number }) {
  const [open, setOpen] = useState(false)
  if (depth > 6) return <p className="text-sm text-muted-foreground">内容层级过深</p>
  if (value !== null && typeof value === "object") {
    const entries = Array.isArray(value) ? value.map((item, index) => [String(index + 1), item] as const) : Object.entries(value)
    return <details className="rounded-lg border p-3" onToggle={event => setOpen(event.currentTarget.open)}>
      <summary className="cursor-pointer text-sm font-medium">{name} · {entries.length} 项</summary>
      {open && <div className="mt-3 space-y-3">{entries.map(([key, item]) => <SubmittedValue key={key} name={fields[key] || key} value={item} depth={depth + 1} />)}</div>}
    </details>
  }
  const text = value === null ? "未设置" : String(value)
  return <div className="min-w-0 text-sm"><p className="mb-1 font-medium">{name}</p>{text.length > 10000
    ? <details onToggle={event => setOpen(event.currentTarget.open)}><summary className="cursor-pointer">展开内容（{text.length.toLocaleString()} 字符）</summary>{open && <pre className="mt-2 max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted p-3 font-sans">{text}</pre>}</details>
    : <p className="whitespace-pre-wrap break-words text-muted-foreground">{text}</p>}</div>
}

type IllustrationChange =
  | { kind: "upload"; image: Pick<NovelIllustration, "url" | "width" | "height"> }
  | { kind: "delete"; novelId: number; id: number }
  | { kind: "layout"; novelId: number; blocks: IllustrationBlock[] }
const positive = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value) && value > 0
function isBlock(value: unknown): value is IllustrationBlock {
  if (typeof value !== "object" || value === null) return false
  const { imageId, x, y, width, height } = value as Record<string, unknown>
  return positive(imageId) && positive(width) && positive(height) && [x, y].every(edge => edge === 0 || positive(edge))
}
// An illustration submission is only a URL, a pair of ids or coordinates. It is
// shown as the picture or layout it stands for; one without the expected shape
// is left to the plain field list.
function illustrationChange(item: ReviewItem): IllustrationChange | null {
  if (item.path === "/api/v2/illustrations" && item.method === "POST") {
    const { url, width, height } = item.payload || {}
    return typeof url === "string" && positive(width) && positive(height) ? { kind: "upload", image: { url, width, height } } : null
  }
  const query = new URLSearchParams(item.query)
  const novelId = Number(query.get("novelId")), id = Number(query.get("id"))
  if (!Number.isSafeInteger(novelId) || novelId <= 0) return null
  if (item.path === "/api/v2/illustrations" && item.method === "DELETE") {
    return Number.isSafeInteger(id) && id > 0 ? { kind: "delete", novelId, id } : null
  }
  if (item.path === "/api/v2/illustration-layout" && item.method === "PUT") {
    const blocks = item.payload?.blocks
    return Array.isArray(blocks) && blocks.every(isBlock) ? { kind: "layout", novelId, blocks } : null
  }
  return null
}
function useNovelIllustrations(novelId: number | null) {
  const { request } = useNovelAccess()
  const [loaded, setLoaded] = useState<{ novelId: number; images?: NovelIllustration[]; error?: string } | null>(null)
  useEffect(() => {
    if (novelId === null) return
    const controller = new AbortController()
    readAllContent<NovelIllustration>(request, `/api/v4/account/athub/novels/${novelId}/illustrations`, controller.signal)
      .then(images => { if (!controller.signal.aborted) setLoaded({ novelId, images }) })
      .catch(reason => { if (!controller.signal.aborted) setLoaded({ novelId, error: contentErrorMessage(reason, "插图加载失败。") }) })
    return () => controller.abort()
  }, [novelId, request])
  return loaded?.novelId === novelId ? loaded : null
}
const picture = "[&>img]:h-auto [&>img]:max-h-80 [&>img]:w-auto [&>img]:max-w-full [&>img]:rounded-lg"
function SubmittedIllustration({ item }: { item: ReviewItem }) {
  const change = illustrationChange(item)
  const library = useNovelIllustrations(change && change.kind !== "upload" ? change.novelId : null)
  if (!change) return null
  if (change.kind === "upload") return <div className={picture}><IllustrationImage image={change.image} label="提交的插图" preview /></div>
  if (!library) return <Skeleton className="h-40 w-full" />
  if (!library.images) return <p role="alert" className="text-sm text-destructive">{library.error}</p>
  const images = new Map(library.images.map(image => [image.id, image]))
  if (change.kind === "delete") {
    const image = images.get(change.id)
    return image ? <div className={picture}><IllustrationImage image={image} label="申请删除的插图" preview /></div>
      : <p className="text-sm text-muted-foreground">这张插图已不在作品中。</p>
  }
  if (!change.blocks.length) return <p className="text-sm text-muted-foreground">这次提交清空了展示画布。</p>
  const height = Math.max(...change.blocks.map(block => block.y + block.height))
  // A layout may be five times as tall as it is wide, so it scrolls in place.
  return <div className="max-h-[32rem] overflow-y-auto rounded-lg border" role="group" aria-label="提交的插图布局" tabIndex={0}>
    <div className="illustration-canvas" style={{ aspectRatio: `${WIDTH} / ${height}` }}>
      {change.blocks.map((block, index) => {
        const image = images.get(block.imageId)
        return <div key={index} className="illustration-block"
          style={{ cursor: "default", left: `${block.x / WIDTH * 100}%`, top: `${block.y / height * 100}%`, width: `${block.width / WIDTH * 100}%`, height: `${block.height / height * 100}%` }}>
          {image ? <IllustrationImage image={image} label={`插图 #${block.imageId}`} />
            : <span className="grid size-full place-items-center border border-dashed text-xs text-muted-foreground">插图已不在作品中</span>}
        </div>
      })}
    </div>
  </div>
}

const date = (value: string) => new Date(value).toLocaleString("zh-CN", { hour12: false })
const failure = (reason: unknown, login: string) => contentErrorMessage(reason, "请求失败，请重试。", { 401: login })
const subject = (item: ReviewItem) => `${operationNames[item.operation]} · ${endpointNames[item.path] || "内容"}`
// The content API explains a refused change only in English, so the status it
// answered with is put into words; its own sentence follows for the exact cause.
function failureReason(item: ReviewItem) {
  const status = item.result_status
  const cause = typeof item.result === "object" && item.result !== null && "error" in item.result ? String(item.result.error) : ""
  const reason = status === 404 ? "提交涉及的内容已不存在"
    : status === 409 ? "与书库中的现有内容冲突"
    : status === 413 ? "提交内容过大"
    : status === 422 ? "提交引用的封面或插图已不在图床中"
    : status !== undefined && status >= 400 && status < 500 ? "内容校验或引用检查未通过"
    : "内容未能写入书库"
  return cause ? `${reason}（${cause}）。` : `${reason}，请核对提交内容后重新提交。`
}

function ReviewBoard({ view }: { view: keyof typeof views }) {
  const { title, icon: TitleIcon, endpoint, login, list, refresh, pages } = views[view]
  const own = view === "submissions"
  const [user, setUser] = useState<AccountUser | null>(null)
  const [policy, setPolicy] = useState<RolePolicy | null>(null)
  const [denied, setDenied] = useState(false)
  const [status, setStatus] = useState<ReviewFilter>("all")
  const [offset, setOffset] = useState(0)
  const [items, setItems] = useState<ReviewItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<string | null>(null)
  const [detail, setDetail] = useState<ReviewItem | null>(null)
  const [detailError, setDetailError] = useState("")
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  const [decision, setDecision] = useState<"approve" | "reject" | null>(null)
  const [note, setNote] = useState("")
  const [busy, setBusy] = useState(false)
  const [decisionError, setDecisionError] = useState("")
  useEffect(() => {
    if (own) return
    const controller = new AbortController()
    void Promise.all([
      api<AccountUser>("/api/v1/user", { signal: controller.signal }),
      api<RolePolicy>("/api/v1/account/athub/permissions", { signal: controller.signal }),
    ]).then(([actor, permissions]) => {
      if (controller.signal.aborted) return
      setUser(actor); setPolicy(permissions)
      setDenied(actor.level < 3 || !hasOperation(permissions, "review"))
    }).catch(reason => { if (!controller.signal.aborted) { setDenied(true); setError(failure(reason, login)) } })
    return () => controller.abort()
  }, [own, login])
  useEffect(() => {
    if (!own && (!policy || denied)) { if (denied) setLoading(false); return }
    const controller = new AbortController()
    setLoading(true); setError("")
    const query = new URLSearchParams({ offset: String(offset) })
    if (status !== "all") query.set("status", status)
    api<{ items: ReviewItem[]; total: number }>(`${endpoint}?${query}`, { signal: controller.signal })
      .then(result => { if (!controller.signal.aborted) { setItems(result.items); setTotal(result.total) } })
      .catch(reason => { if (!controller.signal.aborted) setError(failure(reason, login)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [own, endpoint, login, policy, denied, status, offset, revision])
  useEffect(() => {
    setDetail(null); setDetailError(""); setNote("")
    if (!selected) return
    const controller = new AbortController()
    api<ReviewItem>(`${endpoint}/${selected}`, { signal: controller.signal })
      .then(item => { if (!controller.signal.aborted) setDetail(item) })
      .catch(reason => { if (!controller.signal.aborted) setDetailError(failure(reason, login)) })
    return () => controller.abort()
  }, [endpoint, login, selected, revision])
  async function decide() {
    if (own || !detail || !decision || busy) return
    if (decision === "reject" && !note.trim()) { setDecisionError("请填写驳回原因。"); return }
    setBusy(true); setDecisionError("")
    try {
      const updated = await api<ReviewItem>(`${endpoint}/${detail.id}`, { method: "POST", body: JSON.stringify({ decision, note: note.trim() }) })
      setDetail(updated); setDecision(null); setRevision(value => value + 1)
      feedback.success(updated.status === "approved" ? "审核通过，内容已生效" : updated.status === "rejected" ? "已驳回提交" : "内容未能生效，请查看处理结果")
    } catch (reason) { setDecisionError(failure(reason, login)) }
    finally { setBusy(false) }
  }
  const canApprove = detail && policy && allowsEndpoint(policy, detail.operation, detail.method, detail.path) && detail.user_id !== user?.id
  return <div className="mx-auto max-w-6xl space-y-6 py-7">
    <div className="flex flex-wrap items-center justify-between gap-4">
      <div className="flex items-center gap-3"><TitleIcon aria-hidden="true" /><h1 className="text-2xl font-semibold">{title}</h1></div>
      <div className="flex gap-3"><Button nativeButton={false} role="link" render={<Link href="/library" />} variant="outline"><ArrowLeft />返回书库</Button>
        <Button variant="outline" size="icon" aria-label={refresh} disabled={loading || busy} onClick={() => setRevision(value => value + 1)}><RefreshCw /></Button></div>
    </div>
    {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
    {denied ? <Card><CardContent className="py-6">此页面仅供拥有审核权限的 LT3、LT4 用户使用。</CardContent></Card> : <>
      <TooltipProvider><ToggleGroup value={[status]} disabled={busy} variant="pill" aria-label="审核状态" onValueChange={values => { if (values[0]) { setStatus(values[0] as ReviewFilter); setOffset(0); setSelected(null) } }}>
        {(Object.keys(filterNames) as ReviewFilter[]).map(value => {
          const Icon = filterStyles[value].icon
          return <Tooltip key={value}>
            <TooltipTrigger render={<ToggleGroupItem value={value} aria-label={filterNames[value]} className={`size-8 p-0 ${filterStyles[value].indicator}`} />}>
              <Icon aria-hidden="true" className={`size-4 ${filterStyles[value].color}`} />
            </TooltipTrigger>
            <TooltipContent>{filterNames[value]}</TooltipContent>
          </Tooltip>
        })}
      </ToggleGroup></TooltipProvider>
      <div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <div className="min-w-0 space-y-3" aria-label={list} aria-busy={loading}>
          {loading ? <Skeleton className="h-60 w-full" /> : !error && !items.length ? <Card><CardContent className="py-6 text-center text-muted-foreground">{status === "all" ? "暂无提交" : `暂无${statuses[status]}提交`}</CardContent></Card> :
            items.map(item => <Card key={item.id} className={selected === item.id ? "ring-1 ring-primary" : ""}><CardContent className="space-y-3 py-4">
              {own ? <div className="flex items-center gap-3"><span className="min-w-0 flex-1 truncate font-medium">{subject(item)}</span><ReviewStatusBadge status={item.status} /></div> : <>
                <div className="flex items-center gap-3"><UserAvatar src={item.contributor.avatar} name={item.contributor.name} decorative className="size-8 shrink-0" /><span className="min-w-0 flex-1 truncate font-medium">{item.contributor.name}</span><ReviewStatusBadge status={item.status} /></div>
                <p className="text-sm">{subject(item)}</p>
              </>}
              <div className="flex flex-wrap items-center justify-between gap-2"><time className="text-xs text-muted-foreground">{date(item.created_at)}</time><Button variant="outline" size="sm" disabled={busy} onClick={() => setSelected(item.id)}>查看提交</Button></div>
            </CardContent></Card>)}
          <nav aria-label={pages} className="flex items-center justify-between gap-2 text-sm"><Button variant="outline" disabled={busy || loading || offset === 0} onClick={() => setOffset(value => value - 20)}>上一页</Button><span>{Math.floor(offset / 20) + 1} / {Math.max(1, Math.ceil(total / 20))}</span><Button variant="outline" disabled={busy || loading || offset + 20 >= total} onClick={() => setOffset(value => value + 20)}>下一页</Button></nav>
        </div>
        <Card className="min-w-0 self-start"><CardHeader><CardTitle><h2>提交详情</h2></CardTitle></CardHeader><CardContent className="space-y-5">
          {!selected ? <p className="text-sm text-muted-foreground">选择一项提交查看内容。</p> : detailError ? <Alert variant="destructive"><AlertDescription>{detailError}</AlertDescription></Alert> : !detail ? <Skeleton className="h-48 w-full" /> : <>
            <p className="text-sm">{own ? subject(detail) : `${detail.contributor.name} · ${subject(detail)}`}</p>
            {detail.query && <dl className="space-y-2">{Array.from(new URLSearchParams(detail.query)).map(([key, value]) => <div key={key} className="flex gap-3 text-sm"><dt>{key === "id" && detail.path === "/api/v2/illustrations" ? fields.imageId : fields[key] || key}</dt><dd>{value}</dd></div>)}</dl>}
            <SubmittedIllustration key={detail.id} item={detail} />
            {detail.payload && <div className="space-y-4">{Object.entries(detail.payload).map(([key, value]) => <SubmittedValue key={key} name={fields[key] || key} value={value} />)}</div>}
            {!own && detail.status === "pending" && detail.user_id === user?.id && <p className="text-sm text-muted-foreground">不能审核自己的提交。</p>}
            {detail.status === "pending" ? own ? <div className="border-t pt-4 text-sm"><ReviewStatusBadge status={detail.status} /></div> : <div className="flex flex-wrap justify-end gap-3">
              <Button variant="outline" className={rejectButton} disabled={busy || detail.user_id === user?.id} onClick={() => { setDecision("reject"); setDecisionError("") }}><X />驳回</Button>
              <Button className={approveButton} disabled={busy || !canApprove} onClick={() => { setDecision("approve"); setDecisionError("") }}><Check />通过并生效</Button>
            </div> : <div className="space-y-2 border-t pt-4 text-sm"><div className="flex flex-wrap items-center gap-2">
              <ReviewStatusBadge status={detail.status} />
              <span aria-hidden="true">·</span>
              <span className="inline-flex min-w-0 items-center gap-2">
                <UserAvatar src={detail.reviewer?.avatar} name={detail.reviewer?.name || "审核人员"} decorative className="size-6 shrink-0" />
                <span className="truncate">{detail.reviewer?.name || "审核人员"}</span>
              </span>
              {detail.reviewed_at && <><span aria-hidden="true">·</span><time dateTime={detail.reviewed_at}>{date(detail.reviewed_at)}</time></>}
            </div>{detail.note && <p className="whitespace-pre-wrap break-words">{detail.note}</p>}{detail.status === "failed" && <p className="break-words text-destructive">{failureReason(detail)}</p>}</div>}
          </>}
        </CardContent></Card>
      </div>
    </>}
    {!own && <Dialog open={Boolean(decision)} onOpenChange={open => { if (!open && !busy) setDecision(null) }}>
      <DialogContent showCloseButton={!busy}><DialogHeader><DialogTitle>{decision === "approve" ? "通过这项提交？" : "驳回这项提交？"}</DialogTitle><DialogDescription>{decision === "approve" ? "通过后，提交内容将写入正式书库并记录变更历史。" : "提交内容不会写入书库，提交者可以查看驳回原因。"}</DialogDescription></DialogHeader>
        <label className="space-y-2 text-sm"><span>{decision === "reject" ? "驳回原因" : "审核备注（可选）"}</span><Textarea aria-label={decision === "reject" ? "驳回原因" : "审核备注"} maxLength={1000} value={note} disabled={busy} onChange={event => setNote(event.target.value)} /></label>
        {decisionError && <Alert variant="destructive"><AlertDescription>{decisionError}</AlertDescription></Alert>}
        <DialogFooter><Button variant="outline" disabled={busy} onClick={() => setDecision(null)}>取消</Button><Button variant={decision === "reject" ? "destructive" : "default"} className={decision === "approve" ? approveButton : undefined} disabled={busy} onClick={() => void decide()}>{busy ? "处理中…" : decision === "approve" ? "确认通过" : "确认驳回"}</Button></DialogFooter>
      </DialogContent>
    </Dialog>}
  </div>
}

export function NovelReviews() { return <ReviewBoard view="reviews" /> }
export function NovelSubmissions() { return <ReviewBoard view="submissions" /> }
