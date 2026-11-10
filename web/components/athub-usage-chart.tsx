"use client"

import { useId } from "react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"

export type UsagePoint = { started_at: string; port: number; calls: number }

export function ATHubUsageChart({ trend, total, from, until, bucketSeconds }: {
  trend: UsagePoint[]
  total: number
  from: string
  until: string
  bucketSeconds: number
}) {
  const id = useId().replace(/:/g, "")
  const start = Date.parse(from)
  const end = Date.parse(until)
  const step = bucketSeconds * 1000
  const rows: Record<string, number>[] = []
  // Fill missing buckets with zero; never truncate trends to the paginated call log.
  if (Number.isFinite(start) && Number.isFinite(end) && step > 0) {
    for (let timestamp = start; timestamp < end && rows.length < 744; timestamp += step) {
      rows.push({ timestamp, calls: 0 })
    }
  }
  for (const point of trend) {
    const index = Math.floor((Date.parse(point.started_at) - start) / step)
    if (rows[index]) rows[index].calls += point.calls
  }
  const formatTime = (value: number, full = false) => new Date(value).toLocaleString("zh-CN", {
    month: "2-digit", day: "2-digit", ...(bucketSeconds === 3600 || full ? { hour: "2-digit", minute: "2-digit" } : {}),
  })

  return <div>
    <div className="h-72 w-full sm:h-80" role="img" aria-label={`调用趋势，共 ${total} 次调用`}>
      <ResponsiveContainer width="100%" height="100%" initialDimension={{ width: 800, height: 320 }}>
        <AreaChart data={rows} margin={{ top: 10, right: 12, bottom: 8, left: -20 }} accessibilityLayer>
          <defs><linearGradient id={`${id}-calls`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#ff8a00" stopOpacity={0.12} />
            <stop offset="100%" stopColor="#ff8a00" stopOpacity={0.01} />
          </linearGradient></defs>
          <CartesianGrid vertical={false} stroke="var(--border)" strokeOpacity={0.65} />
          <XAxis dataKey="timestamp" tickFormatter={value => formatTime(Number(value))} axisLine={false} tickLine={false} minTickGap={48} tickMargin={14} tick={{ fill: "var(--muted-foreground)", fontSize: 12 }} />
          <YAxis allowDecimals={false} axisLine={false} tickLine={false} tickMargin={10} tick={{ fill: "var(--muted-foreground)", fontSize: 12 }} domain={[0, "auto"]} />
          <Tooltip labelFormatter={value => formatTime(Number(value), true)} formatter={value => [Number(value).toLocaleString() + " 次", "调用次数"]}
            contentStyle={{ background: "var(--popover)", color: "var(--popover-foreground)", border: "1px solid var(--border)", borderRadius: 10, boxShadow: "0 8px 30px #0000000d", fontSize: 13 }}
            labelStyle={{ marginBottom: 8, fontWeight: 600 }} cursor={{ stroke: "var(--muted-foreground)", strokeOpacity: 0.25, strokeDasharray: "4 4" }} />
          <Area type="monotone" dataKey="calls" name="调用次数"
            stroke="#ff8a00" strokeWidth={2.2} fill={`url(#${id}-calls)`} dot={false} activeDot={{ r: 4, strokeWidth: 2, stroke: "var(--background)" }} isAnimationActive={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  </div>
}
