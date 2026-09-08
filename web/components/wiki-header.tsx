"use client"

import { createContext, useContext, useEffect, useState, type ReactNode } from "react"
import { HomeBrand } from "@/components/home-brand"

const BrandCollapsed = createContext(false)

export function WikiHeader({ children }: { children: ReactNode }) {
  const [brandCollapsed, setBrandCollapsed] = useState(false)
  const [headerCollapsed, setHeaderCollapsed] = useState(false)
  useEffect(() => {
    let frame = 0
    function update() {
      frame = 0
      setBrandCollapsed(current => window.scrollY > 80 ? true : window.scrollY < 20 ? false : current)
      setHeaderCollapsed(current => window.scrollY > 320 ? true : window.scrollY < 260 ? false : current)
    }
    function schedule() { if (!frame) frame = requestAnimationFrame(update) }
    update()
    window.addEventListener("scroll", schedule, { passive: true })
    window.addEventListener("resize", schedule)
    return () => {
      window.removeEventListener("scroll", schedule)
      window.removeEventListener("resize", schedule)
      if (frame) cancelAnimationFrame(frame)
    }
  }, [])
  return <BrandCollapsed.Provider value={brandCollapsed}><div className="home-header wiki-home-header" data-collapsed={headerCollapsed}>{children}</div></BrandCollapsed.Provider>
}

export function WikiBrand() {
  return <HomeBrand href="/" collapsed={useContext(BrandCollapsed)} />
}
