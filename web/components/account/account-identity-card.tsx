"use client"

import { Fingerprint, Rotate3D, ShieldCheck } from "lucide-react"
import { useId, useState, useSyncExternalStore, type PointerEvent } from "react"
import { motion, useMotionTemplate, useSpring } from "framer-motion"
import { useAccountUser, type AccountUser } from "@/components/account/account-shell"
import { UserAvatar } from "@/components/shared/user-avatar"
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from "@/components/ui/card"
import { Button } from "@/components/ui/button"

function subscribeMotionPreference(onChange: () => void) {
  const query = window.matchMedia("(prefers-reduced-motion: reduce)")
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getMotionPreference = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches
const getServerMotionPreference = () => true

export function AccountIdentityCard() {
  const user = useAccountUser()
  return <UserIdentityCard user={user} />
}

export function UserIdentityCard({ user }: { user: AccountUser }) {
  const cardId = useId()
  const [showBack, setShowBack] = useState(false)
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
  function flipCard() { resetCard(); setShowBack(value => !value) }
  const name = user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])
  const joinedAt = user.created_at ? new Date(user.created_at) : null
  const validDate = joinedAt && !Number.isNaN(joinedAt.getTime())
  const joinedDate = validDate ? joinedAt.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }) : "暂无记录"
  return <section className="account-identity-section" aria-labelledby={`${cardId}-title`}>
    <h2 id={`${cardId}-title`}>身份卡</h2>
    <div id={cardId} className="account-member-stage" onPointerMove={followPointer} onPointerLeave={resetCard} onPointerCancel={resetCard}>
    <motion.div style={{ rotateX: reducedMotion ? 0 : rotateX, rotateY: reducedMotion ? 0 : rotateY, transformPerspective: 1000 }} whileHover={reducedMotion ? undefined : { y: -4 }} whileTap={reducedMotion ? undefined : { scale: 0.985 }} transition={{ type: "spring", stiffness: 180, damping: 24 }}>
    <div className="account-member-flipper" data-flipped={showBack}>
    <div className="account-member-face" aria-hidden={showBack} inert={showBack}>
    <Card className="account-member-card">
      {!reducedMotion && <motion.div className="account-member-glow" aria-hidden="true" style={{ background: glow, opacity: lightOpacity }} />}
      <CardHeader className="account-member-heading">
        <CardTitle><span className="account-member-brand">ALBATEXTUS</span></CardTitle>
        <Fingerprint className="size-7" aria-hidden="true" />
      </CardHeader>
      <CardContent className="flex flex-col gap-6">
        <div className="flex min-w-0 items-center gap-4">
          <UserAvatar src={user.avatar_url} name={name} className="size-16" />
          <div className="min-w-0"><p className="account-member-name">{name}</p><p className="account-member-role"><ShieldCheck className="size-4 shrink-0" aria-hidden="true" /><span aria-hidden="true">·</span><span>LT{user.level}</span></p></div>
        </div>
        <dl className="account-member-facts">
          <div><dt>用户 ID</dt><dd className="account-member-number">{user.numeric_id}</dd></div>
          <div><dt>加入日期</dt><dd>{validDate ? <time dateTime={user.created_at}>{joinedDate}</time> : "暂无记录"}</dd></div>
        </dl>
      </CardContent>
      <CardFooter className="account-member-footer"><span className="min-w-0 break-all">{user.id}</span><span className="shrink-0" aria-hidden="true">AT</span></CardFooter>
    </Card>
    </div>
    <div className="account-member-face account-member-face-back" aria-hidden={!showBack} inert={!showBack}>
      <Card className="account-member-card account-member-back">
        <CardHeader className="account-member-heading">
          <CardTitle><span className="account-member-back-shield"><ShieldCheck aria-hidden="true" /></span></CardTitle>
        </CardHeader>
        <CardContent className="account-member-back-content">
          <svg className="account-member-chip" viewBox="0 0 96 76" fill="none" stroke="currentColor" strokeWidth="3.5" strokeLinecap="round" strokeLinejoin="round" role="img" aria-label="身份卡芯片装饰">
            <rect x="4" y="4" width="88" height="68" rx="13" />
            <rect x="33" y="24" width="30" height="28" rx="7" />
            <path d="M33 31H4m29 14H4m59-14h29M63 45h29M38 24V4m20 20V4M38 52v20m20-20v20M18 4v15l15 5M78 4v15l-15 5M18 72V57l15-5m45 20V57l-15-5" />
          </svg>
        </CardContent>
        <CardFooter className="account-member-back-footer">
          <span className="account-member-brand">ALBATEXTUS</span>
        </CardFooter>
      </Card>
    </div>
    </div>
    </motion.div>
    </div>
    <div className="account-member-controls"><Button type="button" variant="ghost" size="sm" onClick={flipCard} aria-controls={cardId} aria-label={showBack ? "查看身份卡正面" : "查看身份卡背面"}><Rotate3D data-icon="inline-start" />{showBack ? "翻回正面" : "查看背面"}</Button><span className="sr-only" role="status" aria-live="polite">{showBack ? "身份卡背面" : "身份卡正面"}</span></div>
  </section>
}
