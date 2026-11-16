"use client"

import { type ElementType } from "react"

/** Shared wordmark; each app supplies its own routing link. */
export function BrandWordmark({ footer = false, collapsed = false, href = "/", LinkComponent = "a", variant = "site" }: {
  footer?: boolean; collapsed?: boolean; href?: string; LinkComponent?: ElementType; variant?: "site" | "auth"
}) {
  const compact = !footer && collapsed
  return <LinkComponent href={href} aria-label="ALBATEXTUS 首页" className={variant === "auth" ? "auth-wordmark" : "home-brand"} data-collapsed={compact}>
    <span className={variant === "auth" ? "auth-wordmark-text" : "home-wordmark"} aria-hidden="true">
      <span>A</span>
      <span className={variant === "auth" ? "auth-wordmark-part" : "home-wordmark-part"}><span>LBA</span></span>
      <span>T</span>
      <span className={variant === "auth" ? "auth-wordmark-part" : "home-wordmark-part"}><span>EXTUS</span></span>
    </span>
  </LinkComponent>
}
