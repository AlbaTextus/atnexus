"use client"

import { useEffect, useRef, useState } from "react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "@/components/ui/button"

export function MessageTemplatePreview({ title = "", content = "", channel = "email", announcementId }: { title?: string; content?: string; channel?: "email" | "announcement"; announcementId?: string }) {
  const [html, setHTML] = useState("")
  const [error, setError] = useState("")
  const [retry, setRetry] = useState(0)
  const [height, setHeight] = useState(640)
  const frame = useRef<HTMLIFrameElement>(null)
  const observer = useRef<ResizeObserver | null>(null)
  useEffect(() => () => observer.current?.disconnect(), [])
  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      const path = announcementId ? "/api/v1/announcements/" + encodeURIComponent(announcementId) : "/api/v1/admin/communications/preview"
      const options = announcementId ? { signal: controller.signal } : { method: "POST", body: JSON.stringify({ title, content, channel }), signal: controller.signal }
      api<{ html: string }>(path, options).then(data => {
        if (!controller.signal.aborted) { setHTML(data.html); setError("") }
      }).catch(reason => { if (!controller.signal.aborted) setError(managementError(reason)) })
    }, announcementId ? 0 : 250)
    return () => { clearTimeout(timer); controller.abort() }
  }, [title, content, channel, announcementId, retry])
  function resizeFrame() {
    observer.current?.disconnect()
    const body = frame.current?.contentDocument?.body
    if (!body) return
    const resize = () => setHeight(Math.max(360, Math.min(1600, Math.ceil(body.scrollHeight) + 2)))
    resize()
    observer.current = new ResizeObserver(resize)
    observer.current.observe(body)
  }
  return <div className="email-template-preview">
    {error ? <Alert variant="destructive"><AlertDescription>{error}<Button variant="link" onClick={() => setRetry(value => value + 1)}>重试</Button></AlertDescription></Alert> : html ? <iframe ref={frame} title={channel === "announcement" ? "公告模板预览" : "邮件模板预览"} sandbox="allow-same-origin" srcDoc={html} onLoad={resizeFrame} style={{ height }} className="email-template-frame" /> : <Skeleton className="h-96 w-full" />}
  </div>
}
