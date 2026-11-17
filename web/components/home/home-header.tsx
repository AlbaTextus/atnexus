"use client"

import { type ReactNode, type ElementType } from "react"
import { BrandWordmark } from "@/components/layout/brand-wordmark"
import { useHeaderCollapse } from "@/hooks/use-header-collapse"

/** Connect landing header, including its separate wordmark and glass collapse thresholds. */
export function HomeHeader({ actions, brandHref = "/", LinkComponent = "a" }: {
  actions: ReactNode; brandHref?: string; LinkComponent?: ElementType
}) {
  const { brand: brandCollapsed, header: headerCollapsed } = useHeaderCollapse(undefined, true)
  return <header className="home-header" data-collapsed={headerCollapsed}>
    <div className="home-container home-header-inner">
      <BrandWordmark href={brandHref} collapsed={brandCollapsed} LinkComponent={LinkComponent} />
      <div className="home-header-actions">{actions}</div>
    </div>
  </header>
}
