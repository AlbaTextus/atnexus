"use client"

import Link from "next/link"
import { motion, useReducedMotion } from "framer-motion"

export function AuthWordmark({ verified }: { verified: boolean }) {
  const reducedMotion = useReducedMotion()
  const transition = { duration: reducedMotion ? 0 : 0.56, ease: [0.22, 1, 0.36, 1] as const }

  return <Link href="/" className="auth-wordmark" aria-label="ALBATEXTUS 首页" data-collapsed={verified}>
    <span className="auth-wordmark-text" aria-hidden="true">
      <span>A</span>
      <motion.span className="auth-wordmark-part" initial={false} animate={{ width: verified ? 0 : "auto", opacity: verified ? 0 : 1 }} transition={transition}>LBA</motion.span>
      <span>T</span>
      <motion.span className="auth-wordmark-part" initial={false} animate={{ width: verified ? 0 : "auto", opacity: verified ? 0 : 1 }} transition={transition}>EXTUS</motion.span>
    </span>
  </Link>
}
