"use client"

import { useState } from "react"
import { RefreshCw, Wrench } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"

export function MaintenanceScreen({ unavailable = false }: { unavailable?: boolean }) {
  const [checking, setChecking] = useState(false)
  const [message, setMessage] = useState("")
  async function check() {
    if (checking) return
    setChecking(true)
    try {
      const response = await fetch("/api/v1/maintenance", { cache: "no-store", signal: AbortSignal.timeout(5000) })
      if (!response.ok) throw new Error("unavailable")
      const state = await response.json()
      if (!state.enabled || state.bypass) { window.location.replace(state.bypass ? "/account" : "/"); return }
      setMessage("仍在维护中。恢复后，此页面会自动带你返回。")
    } catch { setMessage("暂时无法连接服务，请稍后再试。") }
    finally { setChecking(false) }
  }

  return <div className="home-shell home-credit-inspired maintenance-shell">
    <a className="home-skip" href="#maintenance-content">跳转到主要内容</a>
    <header className="maintenance-header home-container">
      <span className="maintenance-wordmark">ALBATEXTUS</span>
      <span className="maintenance-header-note">每一次连接，都值得等待。</span>
    </header>
    <main id="maintenance-content" tabIndex={-1} className="maintenance-main">
      <Empty className="maintenance-empty">
        <div className="maintenance-art" aria-hidden="true">
          <span className="maintenance-orbit maintenance-orbit-outer" />
          <span className="maintenance-orbit maintenance-orbit-inner" />
          <span className="maintenance-star maintenance-star-one">+</span>
          <span className="maintenance-star maintenance-star-two">+</span>
          <div className="maintenance-identity"><span className="maintenance-identity-label">ALBATEXTUS</span><span className="maintenance-monogram">AT</span><span className="maintenance-identity-line" /></div>
          <span className="maintenance-tool"><Wrench strokeWidth={1.5} /></span>
          <span className="maintenance-dot" />
        </div>
        <EmptyHeader className="maintenance-heading">
          <p className="maintenance-eyebrow">{unavailable ? "TEMPORARILY UNAVAILABLE" : "UNDER MAINTENANCE"}</p>
          <EmptyTitle><h1>{unavailable ? "连接暂时中断，请稍候。" : "稍作停留，我们正在维护。"}</h1></EmptyTitle>
          <EmptyDescription><p>{unavailable ? "暂时无法连接账户服务。" : "为了让下一次连接更加顺畅，网站正在进行维护。"}<br />感谢你的耐心，恢复后此页面会自动带你返回。</p></EmptyDescription>
        </EmptyHeader>
        <EmptyContent className="maintenance-actions">
          <Button size="lg" disabled={checking} onClick={check}>{checking ? <Spinner data-icon="inline-start" /> : <RefreshCw data-icon="inline-start" />}{checking ? "正在检查…" : "检查恢复状态"}</Button>
          <p className="maintenance-check-message" role="status">{message || "每 15 秒自动检查一次"}</p>
        </EmptyContent>
      </Empty>
    </main>
    <footer className="maintenance-footer home-container">
      <span>© {new Date().getFullYear()} ALBATEXTUS</span>
    </footer>
  </div>
}
