"use client"

import { useState } from "react"
import { Check, Clock3, Copy, Fingerprint, Globe, Monitor } from "lucide-react"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import type { ManagedUser } from "@/lib/users"

function browserLabel(ua: string) {
  const browsers = [["Edge", /(?:Edg|EdgA|EdgiOS)\/(\d+)/], ["Opera", /OPR\/(\d+)/], ["Firefox", /(?:Firefox|FxiOS)\/(\d+)/], ["Chrome", /(?:Chrome|CriOS)\/(\d+)/], ["Safari", /Version\/(\d+).*Safari/]] as const
  const match = browsers.map(([name, pattern]) => ({ name, version: ua.match(pattern)?.[1] })).find(item => item.version)
  const os = /Android/.test(ua) ? "Android" : /iPhone|iPad|iPod/.test(ua) ? "iOS" : /Windows/.test(ua) ? "Windows" : /Macintosh|Mac OS X/.test(ua) ? "macOS" : /Linux/.test(ua) ? "Linux" : "未知系统"
  return `${match ? `${match.name} ${match.version}` : "未知浏览器"} / ${os}`
}

export function UserDeviceCard({ device }: { device: ManagedUser["last_login_device"] }) {
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState("")
  async function copy() {
    if (!device?.fingerprint) return
    try { await navigator.clipboard.writeText(device.fingerprint); setCopied(true); setError("") }
    catch { setError("复制失败，请选中指纹 ID 手动复制。") }
  }
  return <Card className="managed-user-device" aria-label="最近登录设备">
    <CardHeader><CardTitle className="flex items-center gap-3"><Fingerprint aria-hidden="true" />设备指纹</CardTitle></CardHeader>
    <CardContent>
      {device ? <>
        <div className="device-fingerprint-row"><span>指纹 ID</span><div className="device-fingerprint-value"><code title={device.fingerprint || undefined}>{device.fingerprint || "未获取"}</code><Button type="button" variant="ghost" size="icon" aria-label={copied ? "已复制设备指纹" : "复制设备指纹"} disabled={!device.fingerprint} onClick={copy}>{copied ? <Check /> : <Copy />}</Button></div></div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <div className="device-information">
          <p title={device.user_agent}><Monitor aria-hidden="true" /><span>{browserLabel(device.user_agent)}</span></p>
          <p><Monitor aria-hidden="true" /><span>{device.screen || "屏幕尺寸未记录"}</span></p>
          <p><Globe aria-hidden="true" /><span>{device.timezone || "时区未记录"}</span></p>
          <p><Globe aria-hidden="true" /><span>{device.ip || "IP 未获取"}</span></p>
          <p className="device-login-time"><Clock3 aria-hidden="true" /><time dateTime={device.logged_in_at}>{new Date(device.logged_in_at).toLocaleString("zh-CN", {year:"numeric", month:"long", day:"numeric", hour:"2-digit", minute:"2-digit", hour12:false})}</time></p>
        </div>
        <span role="status" className="sr-only">{copied ? "设备指纹已复制" : ""}</span>
      </> : <p className="text-sm text-muted-foreground">暂无登录设备记录，下次成功登录后自动采集。</p>}
    </CardContent>
  </Card>
}
