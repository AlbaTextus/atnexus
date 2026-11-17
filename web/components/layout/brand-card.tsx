"use client"

import { useId } from "react"

/** Athub's blue-and-blush brand surface, with motion controlled by user preference. */
export function BrandCard() {
  const titleId = useId()

  return (
    <section className="brand-card" aria-labelledby={titleId}>
      <div className="brand-card-flow" aria-hidden="true">
        <span className="brand-card-current brand-card-current-blue" />
        <span className="brand-card-current brand-card-current-pink" />
        <span className="brand-card-current brand-card-current-pearl" />
      </div>
      <div className="brand-card-grain" aria-hidden="true" />
      <h2 id={titleId} className="brand-card-title">ALBATEXTUS</h2>
    </section>
  )
}
