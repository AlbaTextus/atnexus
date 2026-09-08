"use client"

import { useEffect, useState } from "react"
import { Wrench } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import type { MaintenanceStatus } from "@/lib/maintenance"
import { Button } from "@/components/ui/button"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Spinner } from "@/components/ui/spinner"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"

export function MaintenanceControl() {
  const [open, setOpen] = useState(false)
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  useEffect(() => {
    const controller = new AbortController()
    setEnabled(null)
    setError("")
    api<MaintenanceStatus>("/api/v1/maintenance", { signal: controller.signal }).then(state => setEnabled(state.enabled)).catch(() => {
      if (!controller.signal.aborted) setError("无法读取维护状态，请关闭后重试。")
    })
    return () => controller.abort()
  }, [open])

  async function save() {
    if (busy || enabled === null) return
    setBusy(true)
    setError("")
    try {
      const state = await api<MaintenanceStatus>("/api/v1/admin/maintenance", { method: "PUT", body: JSON.stringify({ enabled: !enabled }) })
      setEnabled(state.enabled)
      setOpen(false)
      feedback.success(state.enabled ? "维护模式已开启，仅超级管理员可以访问" : "维护已结束，网站恢复访问")
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : "保存失败，请重试。"
      setError(message)
      feedback.error(message)
    } finally { setBusy(false) }
  }

  return <Dialog open={open} onOpenChange={value => { if (!busy) setOpen(value) }}>
    <DialogTrigger render={<Button variant={enabled ? "secondary" : "ghost"} size="icon" />} aria-label="维护模式" aria-busy={enabled === null && !error} title={enabled ? "维护中 · 点击结束维护" : "维护模式"} data-maintenance-active={enabled || undefined}>
      <Wrench />
    </DialogTrigger>
    <DialogContent className="center-dialog" showCloseButton={!busy}>
      <DialogHeader>
        <DialogTitle>{enabled ? "结束维护，恢复访问？" : "开启网站维护？"}</DialogTitle>
        <DialogDescription>{enabled ? "关闭后，所有用户可以正常访问，维护页会自动恢复。" : "开启后，访客、普通用户和管理员都会进入维护页，仅超级管理员可继续访问。已打开的页面将在 15 秒内切换。"}</DialogDescription>
      </DialogHeader>
      <p className="text-sm text-muted-foreground">维护状态会保存，服务重启后仍然有效。</p>
      {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
      <DialogFooter>
        <Button variant="outline" disabled={busy} onClick={() => setOpen(false)}>取消</Button>
        <Button disabled={busy || enabled === null} onClick={save}>{busy && <Spinner data-icon="inline-start" />}{enabled === null ? "读取状态中…" : enabled ? "结束维护" : "开启维护"}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
