"use client"

import { type Communication, channelNames, messageDate } from "@/lib/communications"
import { MessageTemplatePreview } from "@/components/email-template-preview"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"

export function MessageRecordDialog({ item, busy, onClose, onArchive, onEdit }: {
  item: Communication | null
  busy: boolean
  onClose: () => void
  onArchive: () => void
  onEdit: () => void
}) {
  const announcement = item?.kind === "announcement" || item?.kind === "both"
  const canArchive = announcement && (item?.status === "published" || item?.status === "queued")
  return <Dialog open={Boolean(item)} onOpenChange={open => { if (!open && !busy) onClose() }}>
    <DialogContent className="message-record-dialog sm:max-w-xl" showCloseButton={!busy}>
      <DialogHeader><DialogTitle>消息详情</DialogTitle><DialogDescription>{item && <>{messageDate(item.updated_at)} · {channelNames[item.kind]}{item.status === "draft" ? " · 草稿" : item.status === "archived" ? " · 公告已撤下" : ""}</>}</DialogDescription></DialogHeader>
      <div className="message-record-dialog-body">
        {item && <MessageTemplatePreview key={item.id + ":" + item.revision} title={item.title} content={item.content} channel={announcement ? "announcement" : "email"} />}
      </div>
      <DialogFooter>
        <Button variant="outline" disabled={busy} onClick={onClose}>关闭</Button>
        {item?.status === "draft" && <Button variant="outline" disabled={busy} onClick={onEdit}>继续编辑</Button>}
        {canArchive && <Button variant="destructive" disabled={busy} onClick={onArchive}>{busy ? "正在撤下…" : "撤下公告"}</Button>}
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
