"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { RefreshCw, Sun } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ThemeToggle } from "@/components/layout/theme-toggle"
import { HomeHeader } from "@/components/home/home-header"
import { SiteFooter } from "@/components/layout/site-footer"
import { Spinner } from "@/components/ui/spinner"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { maintenanceDestination } from "@/lib/maintenance/maintenance-navigation"

export function MaintenanceScreen({ unavailable = false, returnTo }: { unavailable?: boolean; returnTo?: string }) {
  const [checking, setChecking] = useState(false)
  const [message, setMessage] = useState("")
  const [lights, setLights] = useState(0)
  const [beaming, setBeaming] = useState(false)
  const lightTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (lightTimer.current !== null) clearTimeout(lightTimer.current)
  }, [])

  function sendLight() {
    setLights(count => count + 1)
    setBeaming(true)
    if (lightTimer.current !== null) clearTimeout(lightTimer.current)
    lightTimer.current = setTimeout(() => setBeaming(false), 1400)
  }

  const lightMessage = lights === 0 ? "点一下小太阳，给维修站送一束光。" : [
    "收到这束光了。谢谢你，让等待也亮了一点。",
    "太阳能板亮起来了。小小的心意，已接收。",
    "这束光先存好，留给下一次出发。",
  ][(lights - 1) % 3]
  const check = useCallback(async () => {
    if (checking) return
    setChecking(true)
    try {
      const response = await fetch("/api/v1/maintenance", { cache: "no-store", signal: AbortSignal.timeout(5000) })
      if (!response.ok) throw new Error("unavailable")
      const state = await response.json()
      if (typeof state.enabled !== "boolean" || typeof state.bypass !== "boolean") throw new Error("unavailable")
      if (!state.enabled || state.bypass) { window.location.replace(maintenanceDestination(returnTo, state.bypass)); return }
      setMessage("仍在维护中。恢复后，此页面会自动带你返回。")
    } catch { setMessage("暂时无法连接服务，请稍后再试。") }
    finally { setChecking(false) }
  }, [checking, returnTo])

  useEffect(() => {
    const timer = window.setInterval(() => { void check() }, 15000)
    return () => window.clearInterval(timer)
  }, [check])

  return <div className="home-shell home-credit-inspired maintenance-shell">
    <a className="home-skip" href="#maintenance-content">跳转到主要内容</a>
    <HomeHeader actions={<ThemeToggle />} />
    <main id="maintenance-content" tabIndex={-1} className="maintenance-main">
      <Empty className="maintenance-empty">
        <div className="station-scene" data-energized={lights > 0} data-beaming={beaming}>
          <div className="station-stars" aria-hidden="true" />
          <span className="station-chart-label" aria-hidden="true">ALBATEXTUS / ORBITAL SERVICE BAY</span>
          <svg className="station-illustration" viewBox="0 0 560 320" fill="none" aria-hidden="true">
            <ellipse className="station-orbit" cx="280" cy="162" rx="208" ry="91" transform="rotate(-18 280 162)" />
            <ellipse className="station-orbit station-orbit-inner" cx="280" cy="162" rx="150" ry="116" transform="rotate(24 280 162)" />
            <path className="station-guide" d="M72 263H488M280 34V289" />
            <path className="station-cross" d="M85 80v10m-5-5h10M463 232v10m-5-5h10" />
            <circle className="station-star" cx="118" cy="211" r="3" />
            <circle className="station-star" cx="365" cy="43" r="2" />
            <circle className="station-star" cx="432" cy="172" r="2" />
            <path className="station-light-beam" d="M447 65Q423 107 355 124M442 71Q393 104 323 124" />
            <g className="station-satellite">
              <g transform="translate(280 156) rotate(-12)">
                <path className="station-strut" d="M-174 0H174M0-55V-87M0 57V77" />
                <circle className="station-antenna" cy="-88" r="5" />
                {[-1, 1].map(side => (
                  <g key={side} transform={`translate(${side < 0 ? -176 : 66} -39)`}>
                    <rect className="station-panel-frame" width="110" height="78" rx="5" />
                    {Array.from({ length: 12 }, (_, cell) => (
                      <rect key={cell} className="station-solar-cell" x={7 + (cell % 4) * 25} y={7 + Math.floor(cell / 4) * 22} width="21" height="18" rx="2" style={{ transitionDelay: `${(side < 0 ? cell : 11 - cell) * 35}ms` }} />
                    ))}
                  </g>
                ))}
                <rect className="station-body-shadow" x="-44" y="-55" width="88" height="118" rx="24" />
                <rect className="station-body" x="-44" y="-62" width="88" height="118" rx="24" />
                <path className="station-body-seam" d="M-43-28H43M-43 25H43" />
                <text className="station-module-label" textAnchor="middle" y="-41">CONNECT</text>
                <text className="station-monogram" textAnchor="middle" y="12">AT</text>
                <circle className="station-port" cx="-12" cy="40" r="3" />
                <circle className="station-port" cy="40" r="3" />
                <circle className="station-port" cx="12" cy="40" r="3" />
                <path className="station-dock" d="M-19 72V82H19V72M-27 88H27" />
              </g>
            </g>
            <path className="station-leader" d="M323 216L366 255H429" />
            <text className="station-caption" x="371" y="270">REST. REPAIR. RECONNECT.</text>
          </svg>
          <button type="button" className="station-sun" onClick={sendLight} aria-label="给维修站送一束光" aria-describedby="station-light-message">
            <Sun aria-hidden="true" />
            {lights > 0 && <span key={lights} className="station-sun-ripple" aria-hidden="true" />}
          </button>
          <div className="station-readout" aria-hidden="true"><span />{unavailable ? "AWAITING SIGNAL" : "DOCKED FOR A LITTLE CARE"}<span className="station-readout-code">AT—01</span></div>
        </div>
        <EmptyHeader className="maintenance-heading">
          <p className="maintenance-eyebrow">{unavailable ? "信号暂歇 · 等待重连" : "轨道维修站 · 暂停靠泊"}</p>
          <EmptyTitle><h1>{unavailable ? "信号绕了点远路。" : "暂停一下，为下一程蓄光。"}</h1></EmptyTitle>
          <EmptyDescription>{unavailable ? "暂时无法连接账户服务，我们正在等待信号归来。" : "网站正在进站保养，让下一次连接更加顺畅。"}<br />你可以在这里歇一会儿，恢复后会自动带你返回。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent className="maintenance-actions">
          <Button size="lg" disabled={checking} onClick={check}>{checking ? <Spinner data-icon="inline-start" /> : <RefreshCw data-icon="inline-start" />}{checking ? "正在检查…" : "检查恢复状态"}</Button>
          <p className="maintenance-check-message" role="status">{message || "每 15 秒自动检查一次"}</p>
          <p className="station-light-message" id="station-light-message" role="status" aria-live="polite" aria-atomic="true"><Sun aria-hidden="true" />{lightMessage}</p>
        </EmptyContent>
      </Empty>
    </main>
    <SiteFooter />
  </div>
}
