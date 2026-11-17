"use client"

import { useEffect, useRef, useState } from "react"
import { House } from "lucide-react"
import { Empty } from "@/components/ui/empty"

/** An imaginary listening station in the empty space where a page should be. */
export function NotFoundScreen() {
  const [scans, setScans] = useState(0)
  const [scanning, setScanning] = useState(false)
  const [found, setFound] = useState(false)
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
      setFound(true)
      setScanning(false)
      timer.current = null
    }, delay)
  }

  return (
    <Empty className="not-found-empty" role="group" aria-label="404 页面未找到">
      <div className="signal-scene" data-found={found} data-scanning={scanning}>
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
        <div className="signal-readout" id="signal-message" role="status" aria-live="polite" aria-atomic="true">
          <span className="signal-readout-light" aria-hidden="true" />
          <span>{scanning ? "SCANNING THE UNKNOWN" : found ? "HOME SIGNAL FOUND" : "NO PAGE IN SIGHT"}</span>
          <span className="signal-readout-frequency" aria-hidden="true">404.00 MHz</span>
        </div>
      </div>
    </Empty>
  )
}
