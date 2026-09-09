"use client"

import { WalletCards } from "lucide-react"
import { useId, useSyncExternalStore, type CSSProperties, type PointerEvent } from "react"
import { motion, useMotionTemplate, useSpring } from "framer-motion"
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { useUser } from "@/features/credit-reference/contexts/user-context"
import { CountingNumber } from "@/features/credit-reference/components/animate-ui/primitives/texts/counting-number"

const numberFormat = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 })

function subscribeMotionPreference(onChange: () => void) {
  const query = window.matchMedia("(prefers-reduced-motion: reduce)")
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getMotionPreference = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches
const getServerMotionPreference = () => true

export function BalanceSummary() {
  const { user, loading } = useUser()
  return <BalanceCard available={Number(user?.available_balance || "0")} pending={Number(user?.pending_balance || "0")} loading={loading} />
}

export function BalanceCard({ available, pending, loading, headingLevel = 1 }: {
  available: number; pending: number; loading: boolean; headingLevel?: 1 | 3
}) {
  const titleId = useId()
  const Heading = headingLevel === 1 ? "h1" : "h3"
  const reducedMotion = useSyncExternalStore(subscribeMotionPreference, getMotionPreference, getServerMotionPreference)
  const spring = { stiffness: 180, damping: 26, mass: 0.7 }
  const rotateX = useSpring(0, spring)
  const rotateY = useSpring(0, spring)
  const lightX = useSpring(50, spring)
  const lightY = useSpring(50, spring)
  const lightOpacity = useSpring(0, spring)
  const glow = useMotionTemplate`radial-gradient(circle at ${lightX}% ${lightY}%, rgb(255 255 255 / 45%), transparent 65%)`

  function followPointer(event: PointerEvent<HTMLDivElement>) {
    if (reducedMotion || event.pointerType !== "mouse") return
    const bounds = event.currentTarget.getBoundingClientRect()
    const x = Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width))
    const y = Math.max(0, Math.min(1, (event.clientY - bounds.top) / bounds.height))
    rotateX.set((0.5 - y) * 5)
    rotateY.set((x - 0.5) * 5)
    lightX.set(x * 100)
    lightY.set(y * 100)
    lightOpacity.set(1)
  }

  function resetCard() {
    rotateX.set(0)
    rotateY.set(0)
    lightOpacity.set(0)
  }

  function amount(value: number) {
    return reducedMotion ? numberFormat.format(value) : (
      <CountingNumber number={value} fromNumber={Math.round(value * 0.94)} decimalPlaces={0}
        thousandSeparator="," transition={{ stiffness: 180, damping: 28 }} />
    )
  }

  const facts = [
    { label: "可用积分", value: available, loading },
    { label: "未来积分", value: pending, loading },
  ]

  return (
    <div className="account-member-stage w-full min-w-0 max-w-[420px]" onPointerMove={followPointer}
      onPointerLeave={resetCard} onPointerCancel={resetCard} data-slot="wallet-card-stage">
    <motion.div
      style={{ rotateX: reducedMotion ? 0 : rotateX, rotateY: reducedMotion ? 0 : rotateY, transformPerspective: 1000 }}
      initial={false} whileHover={reducedMotion ? undefined : { y: -4 }}
      animate={reducedMotion ? { y: 0 } : undefined} transition={{ type: "spring", ...spring }}>
    <Card className="account-member-card w-full" style={{ "--card-spacing": "18px" } as CSSProperties} aria-labelledby={titleId} aria-busy={loading}>
      {!reducedMotion && <motion.div className="account-member-glow" aria-hidden="true" style={{ background: glow, opacity: lightOpacity }} />}
      <CardHeader className="account-member-heading">
        <CardTitle><span className="account-member-brand">ALBATEXTUS</span></CardTitle>
        <WalletCards className="size-7" aria-hidden="true" />
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <Heading id={titleId} className="text-sm text-muted-foreground">总积分</Heading>
          {loading ? <Skeleton className="h-11 w-36" /> : (
            <p className="break-all text-3xl font-semibold tracking-tight tabular-nums" data-slot="wallet-total">
              <span className="sr-only">{numberFormat.format(available + pending)}</span>
              <span aria-hidden="true">{amount(available + pending)}</span>
            </p>
          )}
        </div>
        <dl className="account-member-facts">
          {facts.map(fact => (
            <div key={fact.label}>
              <dt>{fact.label}</dt>
              <dd>{fact.loading ? <Skeleton className="h-5 w-16" /> : <>
                <span className="sr-only">{numberFormat.format(fact.value)}</span>
                <span aria-hidden="true">{amount(fact.value)}</span>
              </>}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
      <CardFooter className="account-member-footer py-3"><span className="ml-auto" aria-hidden="true">AT</span></CardFooter>
    </Card>
    </motion.div>
    </div>
  )
}
