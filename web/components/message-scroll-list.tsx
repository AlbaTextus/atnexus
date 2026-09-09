"use client"

import { useRef, type ReactNode } from "react"

/** Native wheel/touch scrolling, a draggable scrollbar, and mouse grab-to-scroll. */
export function MessageScrollList({ children, label }: { children: ReactNode; label: string }) {
  const drag = useRef<{ y: number; top: number; active: boolean } | null>(null)
  const suppressClick = useRef(false)
  return <div className="message-scroll-list" role="region" aria-label={label} tabIndex={0}
    onPointerDown={event => {
      suppressClick.current = false
      // Leave native scrollbar and resize-handle interactions to the browser.
      const box = event.currentTarget.getBoundingClientRect()
      if (event.pointerType !== "mouse" || event.button !== 0 || event.clientX >= box.left + event.currentTarget.clientWidth || event.clientY > box.bottom - 16) return
      drag.current = { y: event.clientY, top: event.currentTarget.scrollTop, active: false }
    }}
    onPointerMove={event => {
      const state = drag.current
      if (!state || !(event.buttons & 1)) return
      const delta = event.clientY - state.y
      if (!state.active && Math.abs(delta) > 6) {
        state.active = true
        event.currentTarget.setPointerCapture(event.pointerId)
        event.currentTarget.dataset.dragging = "true"
      }
      if (state.active) {
        event.preventDefault()
        event.currentTarget.scrollTop = state.top - delta
        suppressClick.current = true
      }
    }}
    onPointerUp={event => { drag.current = null; delete event.currentTarget.dataset.dragging; if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId) }}
    onPointerCancel={event => { drag.current = null; delete event.currentTarget.dataset.dragging }}
    onLostPointerCapture={event => { drag.current = null; delete event.currentTarget.dataset.dragging }}
    onClickCapture={event => { if (suppressClick.current && event.detail !== 0) { event.preventDefault(); event.stopPropagation() } suppressClick.current = false }}>
    {children}
  </div>
}
