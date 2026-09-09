"use client"

import { useState, useSyncExternalStore, type CSSProperties } from "react"
import { AnimatePresence, motion } from "framer-motion"
import { Sector } from "recharts"
import { Skeleton } from "@/components/ui/skeleton"

const numberFormat = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 })

function subscribeMotionPreference(onChange: () => void) {
  const query = window.matchMedia("(prefers-reduced-motion: reduce)")
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getMotionPreference = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches
const getServerMotionPreference = () => true

export function BalanceDistribution({ available, pending, loading }: {
  available: number
  pending: number
  loading: boolean
}) {
  const [active, setActive] = useState<string | null>(null)
  const reducedMotion = useSyncExternalStore(subscribeMotionPreference, getMotionPreference, getServerMotionPreference)
  const items = [
    { key: "available", label: "可用积分", value: available, color: "var(--balance-available)" },
    { key: "pending", label: "未来积分", value: pending, color: "var(--balance-pending)" },
  ].map(item => ({ ...item, value: Number.isFinite(item.value) ? Math.max(0, item.value) : 0 }))
  const total = items.reduce((sum, item) => sum + item.value, 0)
  let angle = 90

  return (
    <div className="flex h-[180px] min-w-0 items-center justify-center" data-slot="balance-distribution" style={{
      "--balance-available": "color-mix(in oklab, var(--primary) 80%, transparent)",
      "--balance-pending": "color-mix(in oklab, var(--color-zinc-400) 80%, transparent)",
    } as CSSProperties}>
      {loading ? <Skeleton className="size-28 rounded-full" /> : (
        <svg viewBox="0 0 400 260" className="h-full w-full max-w-[400px]" role="group" aria-label="可用和未来积分分布">
          {total === 0 ? (
            <>
              <circle cx={200} cy={130} r={71} fill="none" stroke="var(--muted)" strokeWidth={30} />
              <text x={200} y={134} textAnchor="middle" className="fill-muted-foreground text-xs">暂无积分</text>
            </>
          ) : items.map(item => {
            if (item.value === 0) return null
            const sweep = item.value / total * 360
            const startAngle = angle
            const endAngle = angle - sweep
            angle = endAngle
            const gap = sweep < 360 ? Math.min(1.2, sweep / 5) : 0
            const radians = -(startAngle + endAngle) / 2 * Math.PI / 180
            const right = Math.cos(radians) >= 0
            const leaderRadius = reducedMotion ? 86 : 92
            const sx = 200 + leaderRadius * Math.cos(radians)
            const sy = 130 + leaderRadius * Math.sin(radians)
            const mx = 200 + 104 * Math.cos(radians)
            const my = Math.max(28, Math.min(226, 130 + 104 * Math.sin(radians)))
            // Keep labels outside the ring, including a single full-circle segment.
            const ex = right ? 306 : 94
            const selected = active === item.key
            return (
              <g key={item.key} tabIndex={0} role="img"
                aria-label={`${item.label} ${numberFormat.format(item.value)}`}
                data-balance-segment={item.key}
                className="outline-none"
                onMouseEnter={() => setActive(item.key)} onMouseLeave={() => setActive(null)}
                onFocus={() => setActive(item.key)} onBlur={() => setActive(null)}
              >
                {/* Keep the hit area stationary while the visible segment moves. */}
                <Sector cx={200} cy={130} innerRadius={54} outerRadius={94}
                  startAngle={startAngle - gap} endAngle={endAngle + gap}
                  fill="transparent" stroke="none" pointerEvents="all" />
                <motion.g pointerEvents="none" aria-hidden="true" data-slot="balance-segment-motion"
                  style={{ transformOrigin: "200px 130px" }} initial={false}
                  animate={{
                    x: selected && !reducedMotion ? Math.cos(radians) * 4 : 0,
                    y: selected && !reducedMotion ? Math.sin(radians) * 4 : 0,
                    scale: selected && !reducedMotion ? 1.025 : 1,
                    opacity: active && !selected ? 0.8 : 1,
                  }}
                  transition={reducedMotion ? { duration: 0 } : { type: "spring", stiffness: 280, damping: 22 }}>
                  <Sector cx={200} cy={130} innerRadius={56} outerRadius={86}
                    startAngle={startAngle - gap} endAngle={endAngle + gap}
                    cornerRadius={5} fill={item.color} stroke="none" />
                </motion.g>
                <AnimatePresence>
                  {selected && (
                  <motion.g key="callout" pointerEvents="none" aria-hidden="true" data-slot="balance-callout"
                    initial={{ opacity: reducedMotion ? 1 : 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
                    transition={{ duration: reducedMotion ? 0 : 0.14 }}>
                    <motion.polyline points={`${sx},${sy} ${mx},${my} ${ex},${my}`} fill="none" stroke={item.color} strokeWidth={1.5}
                      initial={{ pathLength: reducedMotion ? 1 : 0 }} animate={{ pathLength: 1 }}
                      transition={{ duration: reducedMotion ? 0 : 0.22, ease: "easeOut" }} />
                    <motion.text x={ex + (right ? 6 : -6)} y={my - 5} textAnchor={right ? "start" : "end"}
                      fill={item.color} fontSize={13} initial={{ opacity: reducedMotion ? 1 : 0 }}
                      animate={{ opacity: 1 }} transition={{ duration: reducedMotion ? 0 : 0.18, delay: reducedMotion ? 0 : 0.07 }}>
                      <tspan>{item.label}</tspan>
                      <tspan x={ex + (right ? 6 : -6)} dy={19} fontSize={12}>{numberFormat.format(item.value)}</tspan>
                    </motion.text>
                  </motion.g>
                  )}
                </AnimatePresence>
              </g>
            )
          })}
        </svg>
      )}
    </div>
  )
}
