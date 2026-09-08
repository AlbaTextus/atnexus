"use client"

import Link from "next/link"
import { Link as TransitionLink } from "next-view-transitions"
import { useSyncExternalStore } from "react"
import { motion } from "framer-motion"

function subscribeReducedMotion(onChange: () => void) {
  const query = window.matchMedia("(prefers-reduced-motion: reduce)")
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getReducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches
const getServerReducedMotion = () => true

export function HomeBrand({ footer = false, collapsed = false, href }: { footer?: boolean; collapsed?: boolean; href?: string }) {
  const reducedMotion = useSyncExternalStore(subscribeReducedMotion, getReducedMotion, getServerReducedMotion)
  const compact = !footer && collapsed
  const transition = { duration: reducedMotion ? 0 : 0.56, ease: [0.22, 1, 0.36, 1] as const }
  const BrandLink = href === "/" ? TransitionLink : Link
  return <BrandLink href={href ?? (footer ? "/" : "#top")} aria-label="AlbaTextus 首页" className="home-brand" data-collapsed={compact}>
    <span className="home-wordmark" aria-hidden="true">
      <span>A</span>
      <motion.span className="home-wordmark-part" initial={false} animate={{ width: compact ? 0 : "auto", opacity: compact ? 0 : 1 }} transition={transition}>{footer ? "LBA" : "lba"}</motion.span>
      <span>T</span>
      <motion.span className="home-wordmark-part" initial={false} animate={{ width: compact ? 0 : "auto", opacity: compact ? 0 : 1 }} transition={transition}>{footer ? "EXTUS" : "extus"}</motion.span>
    </span>
  </BrandLink>
}
