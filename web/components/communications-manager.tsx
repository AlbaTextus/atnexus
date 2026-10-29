"use client"

import { useCallback, useEffect, useState } from "react"
import { CheckCheck, Eye, FileText, Mail, RefreshCw, Save, Send, Users } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { feedback } from "@/lib/feedback"
import { audienceNames, channelNames, type Communication, type CommunicationsResponse, messageDate } from "@/lib/communications"
import { useAccountUser } from "@/components/account-shell"
import { MessageScrollList } from "@/components/message-scroll-list"
import { MessageRecordDialog } from "@/components/message-record-dialog"
import { MessageTemplatePreview } from "@/components/email-template-preview"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle, StatCard } from "@/components/ui/card"
import { ConfirmDialog } from "@/components/ui/dialog"
import { EmptyState } from "@/components/ui/empty"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { Skeleton } from "@/components/ui/skeleton"

type Draft = { id?: string; kind: Communication["kind"] | ""; title: string; content: string; audience: Communication["audience"]; status: Communication["status"]; revision: number; total?: number }
const freshDraft = (): Draft => ({ kind: "both", title: "", content: "", audience: "all", status: "draft", revision: 0 })
const endpoint = "/api/v1/admin/communications"

export function CommunicationsManager() {
  const user = useAccountUser()
  const [data, setData] = useState<CommunicationsResponse | null>(null)
  const [draft, setDraft] = useState<Draft>(freshDraft)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [confirm, setConfirm] = useState(false)
  const [filter, setFilter] = useState("all")
  const [dirty, setDirty] = useState(false)
  const [preview, setPreview] = useState("announcement")
  const [recordId, setRecordId] = useState<string | null>(null)
  const [recordBusy, setRecordBusy] = useState(false)
  const email = draft.kind === "email" || draft.kind === "both"
  const announcement = draft.kind === "announcement" || draft.kind === "both"
  const locked = (email && draft.status !== "draft") || Boolean(draft.total)
  const recipientCount = data?.audience[draft.audience] || 0
  const valid = Boolean(draft.kind && draft.title.trim() && draft.content.trim())
  const channels = [...(announcement ? ["announcement"] : []), ...(email ? ["email"] : [])]
  const load = useCallback(async () => {
    try { setData(await api<CommunicationsResponse>(endpoint, { cache: "no-store" })); setError("") }
    catch (reason) { setError(managementError(reason)) }
  }, [])
  useEffect(() => { void load() }, [load])
  useEffect(() => {
    if (!data?.items.some(item => item.total > item.sent + item.failed)) return
    const timer = window.setInterval(() => { if (document.visibilityState === "visible") void load() }, 5000)
    return () => clearInterval(timer)
  }, [data, load])
  useEffect(() => {
    if (!dirty) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = "" }
    window.addEventListener("beforeunload", warn)
    return () => window.removeEventListener("beforeunload", warn)
  }, [dirty])
  function change(patch: Partial<Draft>) { setDraft(value => ({ ...value, ...patch })); setDirty(true) }
  function choose(value: Draft) {
    if (dirty && !window.confirm("当前内容还未保存，确定切换吗？")) return false
    setDraft(value); setDirty(false); setPreview(value.kind === "email" ? "email" : "announcement")
    return true
  }
  function selectChannels(values: string[]) {
    const kind = values.length === 2 ? "both" : values[0] === "email" ? "email" : values[0] === "announcement" ? "announcement" : ""
    change({ kind })
    if (values.length === 1) setPreview(values[0])
  }
  async function save(status: Draft["status"]): Promise<Communication> {
    const item = await api<Communication>(draft.id ? endpoint + "/" + draft.id : endpoint, { method: draft.id ? "PUT" : "POST", body: JSON.stringify({ kind: draft.kind, title: draft.title.trim(), content: draft.content.trim(), audience: email ? draft.audience : "all", status, revision: draft.revision }) })
    setDraft(item); setDirty(false)
    return item
  }
  async function persist(publish: boolean) {
    setBusy(true)
    try {
      const item = await save(publish && !email ? "published" : "draft")
      if (publish && email) {
        await api(endpoint + "/" + item.id + "/send", { method: "POST", body: JSON.stringify({ revision: item.revision }) })
        setDraft({ ...item, status: "queued", revision: item.revision + 1 })
      }
      setConfirm(false)
      window.dispatchEvent(new Event("albatextus:announcements"))
      feedback.success(publish ? email ? announcement ? "公告已发布，邮件已加入发送队列" : "邮件已加入发送队列" : "公告已发布" : "草稿已保存")
      await load()
    } catch (reason) { feedback.error(managementError(reason)) } finally { setBusy(false) }
  }
  const items = data?.items || []
  const record = items.find(item => item.id === recordId) || null
  async function archiveRecord() {
    if (!record || recordBusy) return
    setRecordBusy(true)
    try {
      await api(endpoint + "/" + record.id + "/archive", { method: "POST", body: JSON.stringify({ revision: record.revision }) })
      setData(value => value ? { ...value, items: value.items.map(item => item.id === record.id ? { ...item, status: "archived", revision: item.revision + 1 } : item) } : value)
      setDraft(value => value.id === record.id ? { ...value, status: "archived", revision: record.revision + 1 } : value)
      window.dispatchEvent(new Event("albatextus:announcements"))
      feedback.success("站内公告已撤下")
      await load()
    } catch (reason) { feedback.error(managementError(reason)) } finally { setRecordBusy(false) }
  }
  const published = items.filter(item => item.kind !== "email" && ["published", "queued"].includes(item.status)).length
  const sent = items.reduce((sum, item) => sum + item.sent, 0)
  const failed = items.reduce((sum, item) => sum + item.failed, 0)
  const visible = items.filter(item => filter === "all" || (filter === "draft" ? item.status === "draft" : item.status !== "draft"))
  return <div className="communications-page">
    {error && <Alert variant="destructive"><AlertDescription>{error}<Button variant="link" onClick={() => void load()}>重新加载</Button></AlertDescription></Alert>}
    <div className="application-stats grid grid-cols-3 gap-3 sm:gap-4">
      {([
        { label: "展示中的公告", count: data ? published : null, icon: Mail, tone: "blue", note: "右上角的信封，随时可见" },
        { label: "已发送邮件", count: data ? sent : null, icon: CheckCheck, tone: "green", note: failed ? failed + " 封发送失败或结果未确认" : "全部消息的邮件发送统计" },
        { label: "可触达邮箱", count: data?.audience.all, icon: Users, tone: "orange", note: "已验证且正常使用的邮箱" },
      ] as const).map(item => <StatCard key={item.label} label={item.label} value={item.count} icon={<item.icon size={18} />} tone={item.tone} note={item.note} />)}
    </div>
    <div className="communications-compose">
      <Card className="communications-editor border">
        <CardHeader><div className="flex items-center justify-between gap-3"><CardTitle className="flex items-center gap-2"><Mail size={18} />{locked ? "消息详情" : "编辑消息"}</CardTitle><Badge variant="secondary">{locked ? "已发布" : dirty ? "未保存" : draft.id ? "已保存" : "新草稿"}</Badge></div><CardDescription>选择发布渠道，公告与邮件使用同一份内容。</CardDescription></CardHeader>
        <CardContent><FieldGroup>
          <FieldGroup className="communications-options">
          <Field><FieldLabel>发布渠道</FieldLabel><ToggleGroup multiple variant="pill" size="lg" spacing={1} value={channels} onValueChange={selectChannels} disabled={busy || locked || draft.status === "published"} aria-label="发布渠道" className="flex-wrap"><ToggleGroupItem value="announcement"><Mail data-icon="inline-start" />站内公告</ToggleGroupItem><ToggleGroupItem value="email"><Mail data-icon="inline-start" />邮箱推送</ToggleGroupItem></ToggleGroup><FieldDescription>{draft.kind === "both" ? "发布一次：站内展示公告，同时发送邮件。" : announcement ? "只在站内展示，未读时亮起小红点。" : email ? "只向选定的收件人发送邮件。" : "请至少选择一个发布渠道。"}</FieldDescription></Field>
          {email && <Field><FieldLabel>邮件收件人</FieldLabel><ToggleGroup variant="pill" size="lg" spacing={1} value={[draft.audience]} onValueChange={values => { if (values[0]) change({ audience: values[0] as Draft["audience"] }) }} disabled={busy || locked} aria-label="邮件收件人" className="flex-wrap">
            {(Object.keys(audienceNames) as Draft["audience"][]).map(audience => <ToggleGroupItem key={audience} value={audience}>{audienceNames[audience]}</ToggleGroupItem>)}</ToggleGroup><FieldDescription>{draft.audience === "self" ? user.email : "预计 " + recipientCount + " 位收件人，以提交时为准。"}{announcement && " 站内公告对全站用户可见。"}{data && !data.smtp_ready && " 邮件服务尚未配置，可先保存草稿。"}</FieldDescription></Field>}
          </FieldGroup>
          <Field><FieldLabel htmlFor="message-title">消息标题</FieldLabel><Input id="message-title" className="h-11" value={draft.title} onChange={event => change({ title: event.target.value })} maxLength={100} placeholder="有什么新消息想告诉大家？" disabled={busy || locked} /></Field>
          <Field><div className="flex items-center justify-between"><FieldLabel htmlFor="message-content">正文</FieldLabel><span className="message-meta">{draft.content.length.toLocaleString()} / 10,000</span></div><Textarea id="message-content" value={draft.content} onChange={event => change({ content: event.target.value })} maxLength={10000} placeholder={"你好，\n\n我们有一些新消息想与你分享……"} className="min-h-60" disabled={busy || locked} /><FieldDescription>支持换行，公告与邮件会分别套用对应的品牌模板。</FieldDescription></Field>
        </FieldGroup></CardContent>
        <CardFooter className="flex-wrap justify-between gap-3"><span className="message-meta">{draft.kind ? channelNames[draft.kind] : "请选择发布渠道"}</span><div className="flex flex-wrap gap-2">
          {!locked && <><Button variant="outline" disabled={busy || !valid || draft.status === "published"} onClick={() => void persist(false)}><Save data-icon="inline-start" />保存草稿</Button>
          <Button disabled={busy || !valid || !data || (email && (!data.smtp_ready || !recipientCount))} onClick={() => email ? setConfirm(true) : void persist(true)}><Send data-icon="inline-start" />{busy ? "处理中…" : draft.status === "published" ? "更新公告" : "发布消息"}</Button></>}
        </div></CardFooter>
      </Card>
      <section className="communications-preview-column" aria-labelledby="message-preview-heading">
        <div className="communications-preview-heading"><div><h2 id="message-preview-heading"><Eye size={17} />内容预览</h2><p className="message-meta">查看用户实际收到的公告与邮件样式。</p></div><ToggleGroup variant="pill" size="lg" spacing={1} value={[preview]} onValueChange={values => { if (values[0]) setPreview(values[0]) }} aria-label="预览渠道"><ToggleGroupItem value="announcement">站内公告</ToggleGroupItem><ToggleGroupItem value="email">邮件</ToggleGroupItem></ToggleGroup></div>
        <MessageTemplatePreview title={draft.title} content={draft.content} channel={preview === "announcement" ? "announcement" : "email"} />
      </section>
    </div>
    <section className="communications-history">
      <div className="communications-history-heading"><div><h2>发布记录</h2><p className="message-meta">全部 {items.length} 条消息，可滚动查看。</p></div><div className="flex items-center gap-2"><ToggleGroup variant="pill" size="lg" spacing={1} value={[filter]} onValueChange={values => { if (values[0]) setFilter(values[0]) }} aria-label="记录筛选"><ToggleGroupItem value="all">全部</ToggleGroupItem><ToggleGroupItem value="draft">草稿</ToggleGroupItem><ToggleGroupItem value="done">已发布</ToggleGroupItem></ToggleGroup><Button variant="ghost" size="icon-sm" aria-label="刷新记录" onClick={() => void load()}><RefreshCw /></Button></div></div>
      {!data && !error ? <Skeleton className="h-32 w-full" /> : visible.length ? <MessageScrollList label="发布记录列表">{visible.map(item => <button type="button" className="communications-record managed-user-row" key={item.id} disabled={busy} onClick={() => setRecordId(item.id)} title={item.title}><strong className="managed-user-name">{item.title}</strong><time dateTime={item.updated_at}>{messageDate(item.updated_at)}</time></button>)}</MessageScrollList> : <EmptyState icon={<FileText />} title={filter === "all" ? "第一条消息，从这里开始" : "暂无相关记录"} description="保存或发布后，公告和邮件会出现在同一份记录里。" />}
    </section>
    <MessageRecordDialog item={record} busy={recordBusy} onClose={() => setRecordId(null)} onArchive={() => void archiveRecord()} onEdit={() => { if (record && choose(record)) setRecordId(null) }} />
    <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={announcement ? "发布公告并推送邮件？" : "确认发送这封邮件？"}
      description={<>{announcement && "公告将对全站用户可见。"}邮件发给「{audienceNames[draft.audience]}」，预计 {recipientCount} 位收件人；提交后邮件无法撤回。</>}
      cancelLabel="继续编辑" confirmLabel={busy ? "正在发布…" : "确认发布"} variant="default" busy={busy} onConfirm={() => persist(true)}>
      <p className="break-words font-medium">{draft.title}</p>
    </ConfirmDialog>
  </div>
}
