"use client"

import { useEffect } from "react"

const idleDelay = 1200
const scrollableOverflow = /^(auto|scroll|overlay)$/

export function ScrollbarBehavior() {
  useEffect(() => {
    const root = document.documentElement
    const timers = new Map<HTMLElement, number>()
    const preference = window.matchMedia("(prefers-reduced-motion: reduce)")
    let held: HTMLElement | null = null
    let horizontal: { element: HTMLElement; position: number; target: number; frame: number; time: number } | null = null

    function reveal(element: HTMLElement) {
      element.setAttribute("data-scroll-active", "")
      window.clearTimeout(timers.get(element))
      timers.set(element, window.setTimeout(() => {
        if (held === element) { reveal(element); return }
        element.removeAttribute("data-scroll-active")
        timers.delete(element)
      }, idleDelay))
    }
    function onScroll(event: Event) {
      const target = event.target
      if (target === document || target === root || target === document.body) reveal(root)
      else if (target instanceof HTMLElement) reveal(target)
    }
    function stopHorizontal() {
      if (horizontal) cancelAnimationFrame(horizontal.frame)
      horizontal = null
    }
    function animateHorizontal(time: number) {
      const active = horizontal
      if (!active || !active.element.isConnected) { stopHorizontal(); return }
      const distance = active.target - active.position
      const amount = preference.matches ? 1 : 1 - Math.exp(-Math.max(0, Math.min(time - active.time, 64)) / 45)
      // Keep a fractional position independently of browser pixel rounding.
      // Otherwise CSS zoom and high refresh rates can stall the last few pixels.
      active.position = Math.abs(distance) < .5 ? active.target : active.position + distance * amount
      active.element.scrollTo({ left: active.position, behavior: "instant" })
      active.time = time
      if (Math.abs(active.target - active.position) < .5) {
        active.element.scrollTo({ left: active.target, behavior: "instant" })
        horizontal = null
      } else active.frame = requestAnimationFrame(animateHorizontal)
    }
    function onWheel(event: WheelEvent) {
      // Preserve zoom gestures, Shift+wheel and native horizontal trackpad input.
      if (event.deltaX !== 0 || event.shiftKey) stopHorizontal()
      if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.shiftKey || event.deltaX !== 0 || !event.deltaY) return
      for (const target of event.composedPath()) {
        if (!(target instanceof HTMLElement)) continue
        const style = getComputedStyle(target)
        // A nested vertical list or editor keeps its normal wheel behavior.
        if (scrollableOverflow.test(style.overflowY) && target.scrollHeight > target.clientHeight + 1) return
        if (!scrollableOverflow.test(style.overflowX) || target.scrollWidth <= target.clientWidth + 1) continue
        const extent = target.scrollWidth - target.clientWidth
        const minimum = style.direction === "rtl" ? -extent : 0
        const maximum = style.direction === "rtl" ? 0 : extent
        const unit = event.deltaMode === WheelEvent.DOM_DELTA_LINE ? parseFloat(style.lineHeight) || 16
          : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? target.clientWidth : 1
        const from = horizontal?.element === target ? horizontal.target : target.scrollLeft
        const next = Math.min(maximum, Math.max(minimum, from + event.deltaY * unit))
        // At either end, let the wheel continue scrolling the surrounding page.
        if (Math.abs(next - from) < .5) return
        if (!event.cancelable) return
        event.preventDefault()
        reveal(target)
        if (horizontal?.element === target) horizontal.target = next
        else {
          stopHorizontal()
          horizontal = { element: target, position: target.scrollLeft, target: next, frame: 0, time: performance.now() }
          horizontal.frame = requestAnimationFrame(animateHorizontal)
        }
        return
      }
    }
    function onPointerDown(event: PointerEvent) {
      const element = event.target instanceof Element ? event.target.closest<HTMLElement>("[data-scroll-active]") : null
      if (element) { held = element; reveal(element) }
      // Grabbing a native thumb must immediately take over from wheel animation.
      stopHorizontal()
    }
    function onPointerUp() {
      const element = held
      held = null
      if (element) reveal(element)
    }
    function onBlur() { onPointerUp(); stopHorizontal() }
    root.setAttribute("data-scrollbars-ready", "")
    document.addEventListener("scroll", onScroll, { capture: true, passive: true })
    document.addEventListener("wheel", onWheel, { passive: false })
    document.addEventListener("pointerdown", onPointerDown, { passive: true })
    window.addEventListener("pointerup", onPointerUp, { passive: true })
    window.addEventListener("pointercancel", onPointerUp, { passive: true })
    document.addEventListener("keydown", stopHorizontal, { passive: true })
    window.addEventListener("blur", onBlur)
    return () => {
      stopHorizontal()
      document.removeEventListener("scroll", onScroll, true)
      document.removeEventListener("wheel", onWheel)
      document.removeEventListener("pointerdown", onPointerDown)
      window.removeEventListener("pointerup", onPointerUp)
      window.removeEventListener("pointercancel", onPointerUp)
      document.removeEventListener("keydown", stopHorizontal)
      window.removeEventListener("blur", onBlur)
      root.removeAttribute("data-scrollbars-ready")
      for (const [element, timer] of timers) {
        window.clearTimeout(timer)
        element.removeAttribute("data-scroll-active")
      }
    }
  }, [])
  return null
}
