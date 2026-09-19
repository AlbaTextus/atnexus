"use client"

import { useEffect, useRef, useState } from "react"
import { ArrowLeft, ArrowUpRight, House, Radar, Undo2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { wikiUrl } from "@/lib/wiki"

const echoes = [
  "收到回家的信号。坐标：首页。",
  "发现一颗迷路的星星。它也准备回家了。",
  "宇宙很大，但首页离你只有一次点击。",
]

/** An imaginary listening station in the empty space where a page should be. */
export function NotFoundScreen() {
  const [scans, setScans] = useState(0)
  const [scanning, setScanning] = useState(false)
  const [echo, setEcho] = useState("")
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (timer.current !== null) clearTimeout(timer.current)
  }, [])

  function scan() {
    if (timer.current !== null) return
    setScanning(true)
    setScans(count => count + 1)
    const delay = window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 1400
    timer.current = setTimeout(() => {
      setEcho(echoes[scans % echoes.length])
      setScanning(false)
      timer.current = null
    }, delay)
  }

  function goBack() {
    if (window.history.length > 1) window.history.back()
    else window.location.assign("/")
  }

  return (
    <Empty className="not-found-empty">
      <div className="signal-scene" data-found={Boolean(echo)} data-scanning={scanning}>
        <div className="signal-chart" aria-hidden="true">
          <span className="signal-chart-label">DEEP SPACE / SECTOR 404</span>
          <span className="signal-chart-cross signal-cross-one">+</span>
          <span className="signal-chart-cross signal-cross-two">+</span>
          <span className="signal-star signal-star-one" />
          <span className="signal-star signal-star-two" />
          <span className="signal-star signal-star-three" />
          <span className="signal-orbit" />
          <span className="signal-coordinate">YOU ARE SOMEWHERE HERE</span>
        </div>
        <div className="signal-numerals">
          <span className="signal-four" aria-hidden="true">4</span>
          <button type="button" className="signal-radar" onClick={scan}
            aria-label="扫描迷失的信号" aria-describedby="signal-message" aria-disabled={scanning}>
            <span className="signal-radar-face" aria-hidden="true">
              <span className="signal-grid" />
              <span className="signal-ring signal-ring-outer" />
              <span className="signal-ring signal-ring-inner" />
              <span className="signal-sweep" style={{ transform: `rotate(${scans * 360 - 35}deg)` }} />
              <span className="signal-origin" />
              <span className="signal-echo"><House /><span>HOME</span></span>
              <span className="signal-speck signal-speck-one" />
              <span className="signal-speck signal-speck-two" />
            </span>
            <span className="signal-bearing signal-bearing-north" aria-hidden="true">N</span>
            <span className="signal-bearing signal-bearing-east" aria-hidden="true">E</span>
            <span className="signal-bearing signal-bearing-south" aria-hidden="true">S</span>
            <span className="signal-bearing signal-bearing-west" aria-hidden="true">W</span>
          </button>
          <span className="signal-four" aria-hidden="true">4</span>
        </div>
        <div className="signal-readout">
          <span className="signal-readout-light" aria-hidden="true" />
          <span>{scanning ? "SCANNING THE UNKNOWN" : echo ? "HOME SIGNAL FOUND" : "NO PAGE IN SIGHT"}</span>
          <span className="signal-readout-frequency" aria-hidden="true">404.00 MHz</span>
        </div>
      </div>

      <EmptyHeader className="not-found-heading">
        <p className="not-found-eyebrow">404 · 迷航记录</p>
        <EmptyTitle><h1>这一页，偏离了轨道。</h1></EmptyTitle>
        <EmptyDescription>
          你抵达了一片还没有名字的空白。<br />
          别担心，回家的坐标一直都在。
        </EmptyDescription>
      </EmptyHeader>

      <EmptyContent className="not-found-content">
        <div className="not-found-actions">
          <Button size="lg" nativeButton={false} role="link" render={<a href="/" />}>
            <ArrowLeft data-icon="inline-start" />返回首页
          </Button>
          <Button size="lg" variant="outline" onClick={goBack}>
            <Undo2 data-icon="inline-start" />返回上一页
          </Button>
        </div>
        <div className="signal-message" id="signal-message" role="status" aria-live="polite" aria-atomic="true">
          <Radar aria-hidden="true" />
          <span>{scanning ? "正在听宇宙的回声…" : echo || "点一下雷达，听听这片空白的回声。"}</span>
        </div>
        <nav className="not-found-shortcuts" aria-label="更多去处">
          <a href="/account">用户中心<ArrowUpRight aria-hidden="true" /></a>
          <a href={wikiUrl()}>逛逛 Wiki<ArrowUpRight aria-hidden="true" /></a>
        </nav>
      </EmptyContent>
    </Empty>
  )
}
