"use client"

import { Fingerprint, ShieldCheck } from "lucide-react"
import { useSyncExternalStore, type PointerEvent } from "react"
import { motion, useMotionTemplate, useSpring } from "framer-motion"
import { useAccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { roleNames } from "@/lib/applications"
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from "@/components/ui/card"

function subscribeMotionPreference(onChange: () => void) {
  const query = window.matchMedia("(prefers-reduced-motion: reduce)")
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getMotionPreference = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches
const getServerMotionPreference = () => true

export function AccountIdentityCard() {
  const user = useAccountUser()
  const reducedMotion = useSyncExternalStore(subscribeMotionPreference, getMotionPreference, getServerMotionPreference)
  const spring = { stiffness: 180, damping: 24, mass: 0.7 }
  const rotateX = useSpring(0, spring)
  const rotateY = useSpring(0, spring)
  const lightX = useSpring(50, spring)
  const lightY = useSpring(50, spring)
  const lightOpacity = useSpring(0, spring)
  const glow = useMotionTemplate`radial-gradient(circle at ${lightX}% ${lightY}%, rgb(255 255 255 / 65%), transparent 65%)`
  function followPointer(event: PointerEvent<HTMLDivElement>) {
    if (reducedMotion || event.pointerType !== "mouse") return
    const bounds = event.currentTarget.getBoundingClientRect()
    const x = Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width))
    const y = Math.max(0, Math.min(1, (event.clientY - bounds.top) / bounds.height))
    rotateX.set((0.5 - y) * 7); rotateY.set((x - 0.5) * 7)
    lightX.set(x * 100); lightY.set(y * 100); lightOpacity.set(1)
  }
  function resetCard() { rotateX.set(0); rotateY.set(0); lightOpacity.set(0) }
  const name = user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])
  const joinedAt = user.created_at ? new Date(user.created_at) : null
  const validDate = joinedAt && !Number.isNaN(joinedAt.getTime())
  return <section className="account-identity-section" aria-labelledby="account-identity-title">
    <h2 id="account-identity-title">身份卡</h2>
    <div className="account-member-stage" onPointerMove={followPointer} onPointerLeave={resetCard} onPointerCancel={resetCard}>
    <motion.div style={{ rotateX: reducedMotion ? 0 : rotateX, rotateY: reducedMotion ? 0 : rotateY, transformPerspective: 1000 }} whileHover={reducedMotion ? undefined : { y: -4 }} whileTap={reducedMotion ? undefined : { scale: 0.985 }} transition={{ type: "spring", stiffness: 180, damping: 24 }}>
    <Card className="account-member-card">
      {!reducedMotion && <motion.div className="account-member-glow" aria-hidden="true" style={{ background: glow, opacity: lightOpacity }} />}
      <CardHeader className="account-member-heading">
        <CardTitle><span className="account-member-brand">ALBATEXTUS</span></CardTitle>
        <Fingerprint className="size-7" aria-hidden="true" />
      </CardHeader>
      <CardContent className="flex flex-col gap-6">
        <div className="flex min-w-0 items-center gap-4">
          <UserAvatar src={user.avatar_url} name={name} className="size-16" />
          <div className="min-w-0"><p className="account-member-name">{name}</p><p className="account-member-role"><ShieldCheck className="size-4 shrink-0" aria-hidden="true" />{roleNames[user.role] || `${user.level} 级`}</p></div>
        </div>
        <dl className="account-member-facts">
          <div><dt>用户 ID</dt><dd className="account-member-number">{user.numeric_id}</dd></div>
          <div><dt>加入日期</dt><dd>{validDate ? <time dateTime={user.created_at}>{joinedAt.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" })}</time> : "暂无记录"}</dd></div>
        </dl>
      </CardContent>
      <CardFooter className="account-member-footer"><span className="min-w-0 break-all">{user.id}</span><span className="shrink-0" aria-hidden="true">AT</span></CardFooter>
    </Card>
    </motion.div>
    </div>
  </section>
}
