"use client"

import { useCallback, useEffect, useId, useRef, useState, useSyncExternalStore } from "react"
import { AnimatePresence, animate, motion, useMotionValue, useMotionValueEvent, useTransform, type AnimationPlaybackControls } from "framer-motion"
import { ArrowLeft, BookOpen, Hand, Undo2, UserRound, Zap } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { wikiUrl } from "@/lib/wiki"

type Point = { x: number; y: number }
/** Stage geometry in stage-local pixels, refreshed by a ResizeObserver. */
type Stage = { width: number; height: number; rest: Point; plugWidth: number; prong: number; socket: Point; radius: number }

const REST_TILT = -12
const dock = { type: "spring", stiffness: 420, damping: 28, mass: 0.8 } as const
const settle = { type: "spring", stiffness: 240, damping: 15 } as const
const reveal = { duration: 0.28, ease: [0.22, 1, 0.36, 1] } as const
const copy = {
  lost: { eyebrow: "404 · CONNECTION LOST", title: "这一页，断开连接了。", lines: ["你访问的页面不存在，或已经搬到了别处。", "把插头插回去，我们帮你接上下一次连接。"] },
  live: { eyebrow: "RECONNECTED", title: "连接已恢复，欢迎回来。", lines: ["这一页依然不在，但熟悉的地方都还在。", "挑一个方向，继续下一次连接。"] },
}

function subscribeReducedMotion(onChange: () => void) {
  const query = window.matchMedia("(prefers-reduced-motion: reduce)")
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getReducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches
const getServerReducedMotion = () => true

const distance = (a: Point, b: Point) => Math.hypot(a.x - b.x, a.y - b.y)
const px = (value: number) => value.toFixed(1)
/** A point on the plug's centre line, `along` px from its centre, following the plug's tilt. */
function plugPoint(stage: Stage, x: number, y: number, tilt: number, along: number): Point {
  const angle = tilt * Math.PI / 180
  return { x: stage.rest.x + x + Math.cos(angle) * along, y: stage.rest.y + y + Math.sin(angle) * along }
}
const cableEnd = (stage: Stage, x: number, y: number, tilt: number) => plugPoint(stage, x, y, tilt, 8 - stage.plugWidth / 2)
const prongTip = (stage: Stage, x: number, y: number, tilt: number) => plugPoint(stage, x, y, tilt, stage.plugWidth / 2 + stage.prong)
/** Offset from the resting position that seats the plug head just inside the socket rim. */
const seat = (stage: Stage): Point => ({ x: stage.socket.x - stage.radius * 0.18 - stage.plugWidth / 2 - stage.rest.x, y: stage.socket.y - stage.rest.y })
/** Dropping either the prongs or the plug body onto the socket counts as plugging in. */
const withinReach = (stage: Stage, gap: number) => gap < stage.radius * 1.1

/** A loose plug for a lost connection: drag it into the socket, or press it, to power the page back on. */
export function NotFoundScreen() {
  const reducedMotion = useSyncExternalStore(subscribeReducedMotion, getReducedMotion, getServerReducedMotion)
  const stageRef = useRef<HTMLDivElement>(null)
  const restRef = useRef<HTMLDivElement>(null)
  const socketRef = useRef<HTMLDivElement>(null)
  const stage = useRef<Stage | null>(null)
  const connectedRef = useRef(false)
  const draggingRef = useRef(false)
  const suppressClick = useRef(false)
  const idle = useRef<AnimationPlaybackControls | null>(null)
  const [connected, setConnected] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [near, setNear] = useState(false)
  const [pulse, setPulse] = useState(0)
  const [announcement, setAnnouncement] = useState("")
  const [size, setSize] = useState({ width: 0, height: 0 })
  const maskId = useId()
  const x = useMotionValue(0)
  const y = useMotionValue(0)
  const tilt = useMotionValue(REST_TILT)
  const scale = useMotionValue(1)
  const layout = useMotionValue(0)

  useEffect(() => {
    const stageEl = stageRef.current, restEl = restRef.current, socketEl = socketRef.current
    if (!stageEl || !restEl || !socketEl) return
    function measure() {
      const box = stageEl!.getBoundingClientRect(), rest = restEl!.getBoundingClientRect(), socket = socketEl!.getBoundingClientRect()
      stage.current = {
        width: box.width, height: box.height, plugWidth: rest.width,
        rest: { x: rest.left - box.left + rest.width / 2, y: rest.top - box.top + rest.height / 2 },
        prong: parseFloat(getComputedStyle(stageEl!).getPropertyValue("--plug-prong")) || 24,
        socket: { x: socket.left - box.left + socket.width / 2, y: socket.top - box.top + socket.height / 2 },
        radius: socket.width / 2,
      }
      // Keep a seated plug seated when the stage changes size.
      if (connectedRef.current) { const target = seat(stage.current); x.jump(target.x); y.jump(target.y) }
      setSize({ width: box.width, height: box.height })
      layout.set(layout.get() + 1)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(stageEl)
    return () => observer.disconnect()
  }, [x, y, layout])

  // Every motion value is read up front so the derived paths subscribe to all of them.
  const cable = useTransform(() => {
    const [dx, dy, angle] = [x.get(), y.get(), tilt.get(), layout.get()]
    const current = stage.current
    if (!current) return ""
    // The cord hangs in from below the stage; slack sags while the plug is loose and pulls taut once seated.
    const anchor = { x: current.width * 0.08, y: current.height + 56 }
    const end = cableEnd(current, dx, dy, angle)
    const natural = distance(anchor, cableEnd(current, 0, 0, REST_TILT)) * 1.3
    const slack = Math.max(0, natural - distance(anchor, end))
    const c1 = { x: anchor.x - slack * 0.15, y: anchor.y - 40 - slack * 0.45 }
    const c2 = { x: end.x - 34 - slack * 0.35, y: end.y + slack * 0.25 }
    return `M${px(anchor.x)} ${px(anchor.y)}C${px(c1.x)} ${px(c1.y)} ${px(c2.x)} ${px(c2.y)} ${px(end.x)} ${px(end.y)}`
  })
  const guide = useTransform(() => {
    layout.get()
    const current = stage.current
    if (!current) return ""
    const from = prongTip(current, 0, 0, REST_TILT)
    const to = { x: current.socket.x - current.radius - 10, y: current.socket.y }
    const control = { x: (from.x + to.x) / 2 - 24, y: Math.min(from.y, to.y) - 30 }
    return `M${px(from.x + 12)} ${px(from.y - 4)}Q${px(control.x)} ${px(control.y)} ${px(to.x)} ${px(to.y)}`
  })
  const gap = useTransform(() => {
    const [dx, dy, angle] = [x.get(), y.get(), tilt.get(), layout.get()]
    const current = stage.current
    if (!current) return Infinity
    return Math.min(distance(prongTip(current, dx, dy, angle), current.socket), distance(plugPoint(current, dx, dy, angle, 0), current.socket))
  })
  useMotionValueEvent(gap, "change", value => setNear(Boolean(stage.current) && withinReach(stage.current!, value)))

  const stopIdle = useCallback(() => { idle.current?.stop(); idle.current = null }, [])
  const startIdle = useCallback(() => {
    stopIdle()
    if (reducedMotion || connectedRef.current || draggingRef.current) return
    idle.current = animate(tilt, [REST_TILT, REST_TILT + 5, REST_TILT], { duration: 3.6, repeat: Infinity, ease: "easeInOut", delay: 0.6 })
  }, [reducedMotion, tilt, stopIdle])
  useEffect(() => { startIdle(); return stopIdle }, [startIdle, stopIdle])

  const settleBack = useCallback((instant: boolean) => {
    if (instant) { x.jump(0); y.jump(0); tilt.jump(REST_TILT); return }
    animate(x, 0, settle)
    animate(y, 0, settle)
    animate(tilt, REST_TILT, { ...settle, damping: 9 }).then(startIdle)
  }, [x, y, tilt, startIdle])
  const plugIn = useCallback((instant: boolean) => {
    const current = stage.current
    if (!current) return
    stopIdle()
    connectedRef.current = true
    setConnected(true)
    setPulse(count => count + 1)
    setAnnouncement("插头已接入，连接恢复。")
    const target = seat(current)
    if (instant) { x.jump(target.x); y.jump(target.y); tilt.jump(0); return }
    animate(x, target.x, dock)
    animate(y, target.y, dock)
    animate(tilt, 0, dock)
  }, [x, y, tilt, stopIdle])
  const pullOut = useCallback((instant: boolean) => {
    connectedRef.current = false
    setConnected(false)
    setAnnouncement("插头已拔出。")
    settleBack(instant)
  }, [settleBack])

  function onDragStart() {
    draggingRef.current = true
    suppressClick.current = true
    stopIdle()
    setDragging(true)
    if (connectedRef.current) { connectedRef.current = false; setConnected(false); setAnnouncement("插头已拔出。") }
    animate(tilt, 0, { duration: 0.18 })
    animate(scale, 1.06, { duration: 0.18 })
  }
  function onDragEnd() {
    draggingRef.current = false
    setDragging(false)
    animate(scale, 1, { duration: 0.24 })
    const current = stage.current
    if (current && withinReach(current, gap.get())) plugIn(false)
    else settleBack(false)
    // The browser's click follows pointerup in the same task; let it pass before re-arming presses.
    window.setTimeout(() => { suppressClick.current = false }, 0)
  }
  function onPress() {
    if (suppressClick.current) return
    if (connectedRef.current) pullOut(reducedMotion)
    else plugIn(reducedMotion)
  }
  function goBack() {
    if (window.history.length > 1) window.history.back()
    else window.location.assign("/")
  }

  const text = connected ? copy.live : copy.lost
  const follow = { x, y, rotate: tilt, scale }
  const swap = { initial: { opacity: 0, y: 10 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0, y: -8 }, transition: reducedMotion ? { duration: 0 } : reveal }
  return (
    <Empty className="not-found-empty">
      <div ref={stageRef} className="not-found-stage" data-connected={connected} data-dragging={dragging}>
        <div className="not-found-glow" aria-hidden="true" />
        <svg className="not-found-cable" aria-hidden="true">
          <defs>
            <linearGradient id={`${maskId}-fade`} gradientUnits="userSpaceOnUse" x1="0" y1={size.height + 56} x2="0" y2={size.height - 24}>
              <stop offset="0" stopColor="#fff" stopOpacity="0" />
              <stop offset="1" stopColor="#fff" stopOpacity="1" />
            </linearGradient>
            <mask id={`${maskId}-mask`} maskUnits="userSpaceOnUse" x="-120" y="-120" width={size.width + 240} height={size.height + 240}>
              <rect x="-120" y="-120" width={size.width + 240} height={size.height + 240} fill={`url(#${maskId}-fade)`} />
            </mask>
          </defs>
          <motion.path className="not-found-guide" d={guide} />
          <g mask={`url(#${maskId}-mask)`}>
            <motion.path className="not-found-cable-line" d={cable} />
            <motion.path className="not-found-cable-current" d={cable} />
          </g>
        </svg>
        <div className="not-found-digits" aria-hidden="true">
          <span className="not-found-digit">4</span>
          <div ref={socketRef} className="not-found-socket" data-near={dragging && near}>
            <span className="not-found-slots"><i className="not-found-slot" /><i className="not-found-slot" /></span>
            {pulse > 0 && <span key={pulse} className="not-found-ripple" />}
          </div>
          <span className="not-found-digit">4</span>
        </div>
        <span className="not-found-spark not-found-spark-one" aria-hidden="true">+</span>
        <span className="not-found-spark not-found-spark-two" aria-hidden="true">+</span>
        <div ref={restRef} className="not-found-plug-rest">
          <motion.span className="not-found-plug-prongs" style={follow} aria-hidden="true"><i className="not-found-prong" /><i className="not-found-prong" /></motion.span>
          <motion.button type="button" className="not-found-plug" style={follow}
            drag={!reducedMotion} dragConstraints={stageRef} dragElastic={0.14} dragMomentum={false}
            onDragStart={onDragStart} onDragEnd={onDragEnd} onClick={onPress}
            data-dragging={dragging} aria-pressed={connected}
            aria-label={connected ? "插头已接入。按下可拔出插头" : "插头。按下或拖到插座上以重新连接"}
            title={connected ? "拔出插头" : "把插头插回去"}>
            <span className="not-found-plug-face" aria-hidden="true"><Zap strokeWidth={2.2} /></span>
          </motion.button>
        </div>
        <p className="not-found-status" aria-hidden="true"><span className="not-found-status-dot" />{connected ? "LINK UP" : "NO CARRIER"}</p>
      </div>

      <EmptyHeader className="not-found-heading">
        <AnimatePresence mode="popLayout" initial={false}>
          <motion.div key={connected ? "live" : "lost"} className="not-found-copy" {...swap}>
            <p className="not-found-eyebrow">{text.eyebrow}</p>
            <EmptyTitle><h1>{text.title}</h1></EmptyTitle>
            <EmptyDescription><p>{text.lines[0]}<br />{text.lines[1]}</p></EmptyDescription>
          </motion.div>
        </AnimatePresence>
      </EmptyHeader>
      <p className="sr-only" role="status">{announcement}</p>

      <EmptyContent className="not-found-content">
        <Button size="lg" nativeButton={false} role="link" render={<a href="/" />}>
          <ArrowLeft data-icon="inline-start" />返回首页
        </Button>
        <div className="not-found-slot-row">
          <AnimatePresence mode="popLayout" initial={false}>
            {connected
              ? <motion.nav key="shortcuts" aria-label="更多去处" className="not-found-shortcuts" {...swap}>
                  <a href="/account"><UserRound aria-hidden="true" />用户中心</a>
                  <span aria-hidden="true" />
                  <a href={wikiUrl()}><BookOpen aria-hidden="true" />Wiki</a>
                  <span aria-hidden="true" />
                  <button type="button" onClick={goBack}><Undo2 aria-hidden="true" />返回上一页</button>
                </motion.nav>
              : <motion.p key="hint" className="not-found-hint" {...swap}>
                  <Hand aria-hidden="true" />{reducedMotion ? "点按插头，重新连接" : "拖动插头到插座，或点按插头"}
                </motion.p>}
          </AnimatePresence>
        </div>
      </EmptyContent>
    </Empty>
  )
}
