"use client"

import { useCallback, useEffect, useState } from "react"
import { ArrowLeft, CheckCheck, Mail } from "lucide-react"
import { api } from "@/lib/api"
import { MessageTemplatePreview } from "@/components/email-template-preview"
import { MessageScrollList } from "@/components/message-scroll-list"
import { type Communication, messageDate } from "@/lib/communications"
import { Button } from "@/components/ui/button"
import { IconButton } from "@albatextus/ui"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { Alert, AlertDescription } from "@/components/ui/alert"

type ReadState = Record<string, number>

export function AnnouncementCenter() {
  const [items, setItems] = useState<Communication[]>([])
  const [read, setRead] = useState<ReadState>({})
  const [key, setKey] = useState("")
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const load = useCallback(async () => {
    try {
      const data = await api<{ items: Communication[]; viewer: string }>("/api/v1/announcements", { cache: "no-store" })
      const storageKey = `albatextus.announcements.${data.viewer}`
      let saved: ReadState = {}
      try {
        const parsed = JSON.parse(localStorage.getItem(storageKey) || "{}")
        if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) saved = parsed
      } catch { /* Reading announcements also works when storage is unavailable. */ }
      setRead(previous => storageKey === key ? { ...saved, ...previous } : saved)
      setKey(storageKey); setItems(data.items); setError(false)
    } catch { setError(true) } finally { setLoading(false) }
  }, [key])
  useEffect(() => {
    void load()
    const timer = window.setInterval(() => { if (document.visibilityState === "visible") void load() }, 60000)
    const sync = () => { void load() }
    window.addEventListener("focus", sync)
    window.addEventListener("storage", sync)
    window.addEventListener("albatextus:announcements", sync)
    return () => { clearInterval(timer); window.removeEventListener("focus", sync); window.removeEventListener("storage", sync); window.removeEventListener("albatextus:announcements", sync) }
  }, [load])
  const unread = items.filter(item => read[item.id] !== item.revision).length
  const current = items.find(item => item.id === selected)
  function mark(itemsToRead: Communication[]) {
    const next = { ...read }
    itemsToRead.forEach(item => { next[item.id] = item.revision })
    setRead(next)
    try { localStorage.setItem(key, JSON.stringify(next)) } catch { /* Keep read state for this session. */ }
  }
  return <Dialog open={open} onOpenChange={value => { setOpen(value); if (value) { setSelected(null); void load() } }}>
    <DialogTrigger render={<IconButton className="announcement-trigger" label={unread ? `全站公告，${unread} 条未读` : "全站公告"} />}>
      <Mail />{unread > 0 && <span className="announcement-dot" aria-hidden="true" />}
    </DialogTrigger>
    <DialogContent className="announcement-dialog sm:max-w-2xl">
      <DialogHeader><DialogTitle>全站公告</DialogTitle><DialogDescription>关于 ALBATEXTUS 的新消息，都在这里。</DialogDescription></DialogHeader>
      {error && <Alert variant="destructive"><AlertDescription>暂时无法加载公告。<Button variant="link" onClick={() => void load()}>重试</Button></AlertDescription></Alert>}
      {loading ? <div className="flex flex-col gap-3"><Skeleton className="h-20 w-full" /><Skeleton className="h-20 w-full" /></div> : current ? <div className="announcement-detail">
        <Button variant="ghost" size="sm" onClick={() => setSelected(null)}><ArrowLeft data-icon="inline-start" />全部公告</Button>
        <p className="message-meta">{messageDate(current.published_at || current.updated_at)}</p>
        <MessageTemplatePreview key={current.id + ":" + current.revision} channel="announcement" announcementId={current.id} />
      </div> : items.length ? <>
        <div className="flex items-center justify-between"><span className="message-meta">{unread ? `${unread} 条未读` : "已读完所有公告"}</span><Button variant="ghost" size="sm" disabled={!unread} onClick={() => mark(items)}><CheckCheck data-icon="inline-start" />全部已读</Button></div>
        <MessageScrollList label="全站公告列表">{items.map(item => <button type="button" key={item.id} className="communications-record" title={item.title} onClick={() => { setSelected(item.id); mark([item]) }}>
          <strong>{item.title}</strong><time dateTime={item.published_at || item.updated_at}>{messageDate(item.published_at || item.updated_at)}</time>
        </button>)}</MessageScrollList>
      </> : !error && <Empty><EmptyHeader><EmptyMedia variant="icon"><Mail /></EmptyMedia><EmptyTitle>暂时没有新公告</EmptyTitle><EmptyDescription>有新消息时，信封会亮起红点。</EmptyDescription></EmptyHeader></Empty>}
    </DialogContent>
  </Dialog>
}
