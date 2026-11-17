"use client"

import { useEffect, useLayoutEffect, useRef, type ReactNode, type RefObject } from "react"

const motionQuery = "(prefers-reduced-motion: reduce)"
const ease = "cubic-bezier(0.22, 1, 0.36, 1)"

// Animate real height so the following content moves with the expanding panel.
// Measurements stay in CSS pixels, including inside the site's zoomed layout.
export function NovelAutoHeight({ children, open = true, inline = false, className = "", disabled = false, clampToParent = false }: {
  children: ReactNode
  open?: boolean
  inline?: boolean
  className?: string
  disabled?: boolean
  clampToParent?: boolean
}) {
  const outer = useRef<HTMLDivElement>(null)
  const inner = useRef<HTMLDivElement>(null)
  const initiallyOpen = useRef(open)
  const Element = inline ? "span" : "div"
  useLayoutEffect(() => {
    const host = outer.current, content = inner.current
    if (!host || !content) return
    const preference = window.matchMedia(motionQuery)
    let animation: Animation | null = null
    let initialized = host.style.height !== ""
    const measure = () => {
      // offsetHeight is not affected by inherited CSS zoom.
      const limit = clampToParent && host.parentElement ? parseFloat(getComputedStyle(host.parentElement).maxHeight) : Infinity
      const target = open ? Math.min(content.offsetHeight, Number.isFinite(limit) ? limit : Infinity) : 0
      const previous = parseFloat(getComputedStyle(host).height) || 0
      animation?.cancel()
      host.style.height = `${target}px`
      host.style.overflow = open ? "visible" : "clip"
      if (initialized && !disabled && !preference.matches && Math.abs(previous - target) > 1) {
        host.style.overflow = "clip"
        animation = host.animate([{ height: `${previous}px` }, { height: `${target}px` }], { duration: 320, easing: ease })
        animation.onfinish = () => { host.style.overflow = open ? "visible" : "clip" }
      }
      initialized = true
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(content)
    if (clampToParent) window.addEventListener("resize", measure)
    preference.addEventListener("change", measure)
    return () => {
      observer.disconnect()
      window.removeEventListener("resize", measure)
      preference.removeEventListener("change", measure)
      // Preserve the current visual height when a transition is interrupted.
      const current = getComputedStyle(host).height
      animation?.cancel()
      host.style.height = current
    }
  }, [open, disabled, clampToParent])
  return <Element ref={outer} className={`novel-motion-size ${className}`} aria-hidden={!open || undefined}
    style={initiallyOpen.current ? undefined : { height: 0, overflow: "clip" }}>
    <Element ref={inner} className="novel-motion-size-content" inert={!open || undefined}>{children}</Element>
  </Element>
}

// Observe once per route. No scroll handlers, wheel interception or per-frame
// React updates; panels remain visible when JS or motion support is unavailable.
export function useNovelScrollReveal(root: RefObject<HTMLDivElement | null>, route: string) {
  useEffect(() => {
    const host = root.current
    if (!host) return
    const preference = window.matchMedia(motionQuery)
    let cleanup = () => {}
    const start = () => {
      cleanup()
      if (preference.matches) return
      const seen = new WeakSet<Element>()
      const pending = new Set<HTMLElement>()
      const animations = new Map<HTMLElement, Animation>()
      const finish = (element: HTMLElement) => {
        pending.delete(element)
        element.style.removeProperty("opacity")
        animations.get(element)?.finish()
      }
      const observer = new IntersectionObserver(entries => {
        let order = 0
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          const element = entry.target as HTMLElement
          observer.unobserve(element)
          pending.delete(element)
          element.style.removeProperty("opacity")
          const animation = element.animate([
            { opacity: 0, transform: "translateY(18px)" },
            { opacity: 1, transform: "translateY(0)" },
          ], { duration: 460, delay: Math.min(order++ * 35, 70), easing: ease, fill: "backwards" })
          animations.set(element, animation)
          animation.onfinish = () => animations.delete(element)
        }
      }, { rootMargin: "0px 0px -24px 0px", threshold: 0 })
      const selector = ".novel-detail-page .panel, .library-books > .library-book, .novel-history-list > li, .novel-upload-preview-layout > aside > .panel"
      const scan = () => host.querySelectorAll<HTMLElement>(selector).forEach(element => {
        if (seen.has(element)) return
        seen.add(element)
        // Already visible content stays put; new sections below the fold reveal
        // as they arrive. Nested cards inherit their panel's reveal.
        if (element.getBoundingClientRect().top < window.innerHeight || element.parentElement?.closest(".panel")) return
        pending.add(element)
        element.style.opacity = "0"
        observer.observe(element)
      })
      const focus = (event: FocusEvent) => {
        for (const element of pending) if (element.contains(event.target as Node)) { observer.unobserve(element); finish(element) }
        for (const [element] of animations) if (element.contains(event.target as Node)) finish(element)
      }
      scan()
      const mutation = new MutationObserver(records => { if (records.some(record => record.addedNodes.length)) scan() })
      mutation.observe(host, { childList: true, subtree: true })
      host.addEventListener("focusin", focus)
      cleanup = () => {
        observer.disconnect()
        mutation.disconnect()
        host.removeEventListener("focusin", focus)
        for (const element of pending) element.style.removeProperty("opacity")
        for (const animation of animations.values()) animation.cancel()
      }
    }
    start()
    preference.addEventListener("change", start)
    return () => { cleanup(); preference.removeEventListener("change", start) }
  }, [root, route])
}
