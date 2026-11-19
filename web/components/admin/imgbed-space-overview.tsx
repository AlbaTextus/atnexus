"use client"

import { useMemo, useState } from "react"
import { Cloud, Files, FolderTree, HardDrive, Layers, Server } from "lucide-react"
import {
  channelBytes,
  formatBytes,
  formatDateTime,
  groupLabel,
  spaceLabel,
  type QuotaResponse,
  type StorageChannel,
  type UploadConfig,
} from "@/lib/admin/imgbed"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle, StatCard } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

type ChannelRow = {
  key: string
  name: string
  kind: "local" | "s3" | "unknown"
  configured: boolean
  channel?: StorageChannel
  usedBytes: number
  fileCount: number
}

const number = (value: number) => value.toLocaleString("zh-CN")

export function ImgbedSpaceOverview({
  quota,
  config,
  blockedIPCount,
  loading,
}: {
  quota: QuotaResponse | null
  config: UploadConfig | null
  blockedIPCount: number
  loading: boolean
}) {
  const [spaceView, setSpaceView] = useState<"detail" | "group">("detail")

  const totalBytes = useMemo(() => {
    if (!quota) return 0
    if (typeof quota.totalBytes === "number" && Number.isFinite(quota.totalBytes)) return quota.totalBytes
    return Math.round((quota.totalSizeMB || 0) * 1048576)
  }, [quota])

  const channelRows = useMemo<ChannelRow[]>(() => {
    const stats = quota?.channelStats || quota?.quotaStats || {}
    const rows: ChannelRow[] = []
    const seen = new Set<string>()

    for (const c of config?.cfr2.channels || []) {
      seen.add(c.name)
      const usage = stats[c.name]
      rows.push({
        key: `local:${c.name}`,
        name: c.name,
        kind: "local",
        configured: true,
        channel: c,
        usedBytes: channelBytes(usage),
        fileCount: usage?.fileCount ?? 0,
      })
    }
    for (const c of config?.s3.channels || []) {
      seen.add(c.name)
      const usage = stats[c.name]
      rows.push({
        key: `s3:${c.name}`,
        name: c.name,
        kind: "s3",
        configured: true,
        channel: c,
        usedBytes: channelBytes(usage),
        fileCount: usage?.fileCount ?? 0,
      })
    }
    for (const [name, usage] of Object.entries(stats)) {
      if (seen.has(name)) continue
      rows.push({
        key: `unknown:${name}`,
        name: name || "未命名渠道",
        kind: "unknown",
        configured: false,
        usedBytes: channelBytes(usage),
        fileCount: usage?.fileCount ?? 0,
      })
    }
    return rows
  }, [config, quota])

  const groupedSpaces = useMemo(() => {
    const spaces = quota?.spaceStats || []
    const map = new Map<string, { group: string; fileCount: number; usedBytes: number }>()
    for (const item of spaces) {
      const prev = map.get(item.group)
      if (prev) {
        prev.fileCount += item.fileCount
        prev.usedBytes += item.usedBytes
      } else {
        map.set(item.group, { group: item.group, fileCount: item.fileCount, usedBytes: item.usedBytes })
      }
    }
    return Array.from(map.values()).sort((a, b) => b.usedBytes - a.usedBytes || a.group.localeCompare(b.group))
  }, [quota])

  const allChannels = useMemo(
    () => [...(config?.cfr2.channels || []), ...(config?.s3.channels || [])],
    [config],
  )
  const enabledChannels = allChannels.filter(c => c.enabled).length
  const stateStats = quota?.stateStats
  const fileNote = stateStats
    ? `封禁 ${number(stateStats.blocked)} · 待提交 ${number(stateStats.pending)} · 清理中 ${number(stateStats.deleting)}`
    : "已就绪状态的文件总数"

  if (loading && !quota) {
    return (
      <div className="space-y-5" aria-label="正在加载空间占用">
        <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
          {Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-28" />)}
        </div>
        <div className="grid gap-5 lg:grid-cols-2">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-5" aria-busy={loading}>
      <div className="user-stats grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
        <StatCard
          label="总空间占用"
          value={quota ? formatBytes(totalBytes) : "—"}
          icon={<HardDrive size={18} />}
          tone="blue"
          note={quota ? `约 ${quota.totalSizeMB.toFixed(2)} MB` : "按已就绪文件统计"}
        />
        <StatCard
          label="托管文件"
          value={quota ? number(quota.totalCount) : "—"}
          icon={<Files size={18} />}
          tone="green"
          note={fileNote}
        />
        <StatCard
          label="存储渠道"
          value={config ? `${enabledChannels} / ${allChannels.length}` : "—"}
          icon={<Server size={18} />}
          tone="orange"
          note={config ? `本地 ${config.cfr2.channels.length} 个 · S3 ${config.s3.channels.length} 个` : "已启用 / 总配置"}
        />
        <StatCard
          label="活跃分块会话"
          value={stateStats ? number(stateStats.activeUploadSessions) : "—"}
          icon={<Layers size={18} />}
          tone="muted"
          note={`IP 黑名单 ${number(blockedIPCount)} 条`}
        />
      </div>

      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Cloud className="size-4 text-muted-foreground" aria-hidden="true" />
              <span>各存储渠道空间占用</span>
            </CardTitle>
            <CardDescription>
              按存储渠道统计已用容量与配额阈值{quota?.lastUpdated ? ` · 更新于 ${formatDateTime(quota.lastUpdated)}` : ""}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {channelRows.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">暂无存储渠道数据</p>
            ) : (
              channelRows.map(row => {
                const quotaCfg = row.channel?.quota
                const hasLimit = Boolean(quotaCfg?.enabled && quotaCfg.limitGB > 0)
                const limitBytes = hasLimit ? quotaCfg!.limitGB * 1024 * 1024 * 1024 : 0
                const threshold = quotaCfg && quotaCfg.threshold > 0 ? quotaCfg.threshold : 95
                const ratio = hasLimit && limitBytes > 0
                  ? (row.usedBytes / limitBytes) * 100
                  : totalBytes > 0 ? (row.usedBytes / totalBytes) * 100 : 0
                const barWidth = Math.min(100, Math.max(row.usedBytes > 0 ? 1.5 : 0, ratio))
                const barColor = !hasLimit
                  ? "bg-primary"
                  : ratio >= 100
                    ? "bg-destructive"
                    : ratio >= threshold
                      ? "bg-amber-500"
                      : "bg-emerald-600 dark:bg-emerald-500"

                return (
                  <div key={row.key} className="space-y-2 rounded-xl border border-border p-3.5">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">{row.name}</span>
                        <Badge variant="secondary">
                          {row.kind === "local" ? "本地存储" : row.kind === "s3" ? "S3 对象存储" : "历史渠道"}
                        </Badge>
                        {row.configured ? (
                          <Badge variant={row.channel?.enabled ? "success" : "outline"}>
                            {row.channel?.enabled ? "已启用" : "已停用"}
                          </Badge>
                        ) : (
                          <Badge variant="destructive">未配置</Badge>
                        )}
                        {hasLimit && ratio >= 100 && <Badge variant="destructive">已超限</Badge>}
                        {hasLimit && ratio >= threshold && ratio < 100 && (
                          <Badge variant="outline" className="border-amber-500/40 text-amber-600 dark:text-amber-400">
                            接近阈值
                          </Badge>
                        )}
                      </div>
                      <span className="text-xs font-medium tabular-nums text-muted-foreground">
                        {number(row.fileCount)} 个文件
                      </span>
                    </div>

                    <div className="relative h-2 w-full overflow-hidden rounded-full bg-muted" role="progressbar" aria-label={`${row.name} 空间占用`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(Math.min(100, ratio))}>
                      <div className={`h-full rounded-full transition-all ${barColor}`} style={{ width: `${barWidth}%` }} />
                      {hasLimit && (
                        <span
                          aria-hidden="true"
                          title={`告警阈值 ${threshold}%`}
                          className="absolute inset-y-0 w-0.5 bg-foreground/45"
                          style={{ left: `${Math.min(99.5, Math.max(0.5, threshold))}%` }}
                        />
                      )}
                    </div>

                    <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground tabular-nums">
                      <span>已用 {formatBytes(row.usedBytes)}</span>
                      {hasLimit ? (
                        <span>
                          上限 {quotaCfg!.limitGB} GB · {ratio.toFixed(1)}%（阈值 {threshold}%）
                        </span>
                      ) : (
                        <span>不限上限 · 占全站 {totalBytes > 0 ? ratio.toFixed(1) : "0.0"}%</span>
                      )}
                    </div>
                  </div>
                )
              })
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
            <div className="space-y-1">
              <CardTitle className="flex items-center gap-2">
                <FolderTree className="size-4 text-muted-foreground" aria-hidden="true" />
                <span>各业务空间占用</span>
              </CardTitle>
              <CardDescription>按目录归属统计用户、作品、论坛与自定义目录的空间分布</CardDescription>
            </div>
            <div className="flex gap-1 rounded-lg bg-muted p-1" role="group" aria-label="空间统计视图">
              <Button size="sm" variant={spaceView === "detail" ? "outline" : "ghost"} aria-pressed={spaceView === "detail"} onClick={() => setSpaceView("detail")}>
                细分空间
              </Button>
              <Button size="sm" variant={spaceView === "group" ? "outline" : "ghost"} aria-pressed={spaceView === "group"} onClick={() => setSpaceView("group")}>
                一级汇总
              </Button>
            </div>
          </CardHeader>
          <CardContent className="space-y-3.5">
            {spaceView === "detail" ? (
              !quota?.spaceStats?.length ? (
                <p className="py-8 text-center text-sm text-muted-foreground">暂无空间文件记录</p>
              ) : (
                quota.spaceStats.map(item => {
                  const share = totalBytes > 0 ? (item.usedBytes / totalBytes) * 100 : 0
                  const barWidth = Math.min(100, Math.max(item.usedBytes > 0 ? 1.5 : 0, share))
                  return (
                    <div key={item.key} className="space-y-1.5 rounded-xl border border-border/80 px-3.5 py-2.5">
                      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="font-medium">{spaceLabel(item.key)}</span>
                          <code className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{item.key}</code>
                        </div>
                        <span className="text-xs font-medium tabular-nums">
                          {formatBytes(item.usedBytes)} · {number(item.fileCount)} 个 ({share.toFixed(1)}%)
                        </span>
                      </div>
                      <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                        <div className="h-full rounded-full bg-primary/80 transition-all" style={{ width: `${barWidth}%` }} />
                      </div>
                    </div>
                  )
                })
              )
            ) : groupedSpaces.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">暂无空间文件记录</p>
            ) : (
              groupedSpaces.map(item => {
                const share = totalBytes > 0 ? (item.usedBytes / totalBytes) * 100 : 0
                const barWidth = Math.min(100, Math.max(item.usedBytes > 0 ? 1.5 : 0, share))
                return (
                  <div key={item.group} className="space-y-1.5 rounded-xl border border-border/80 px-3.5 py-2.5">
                    <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className="font-medium">{groupLabel(item.group)}</span>
                        <code className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{item.group}</code>
                      </div>
                      <span className="text-xs font-medium tabular-nums">
                        {formatBytes(item.usedBytes)} · {number(item.fileCount)} 个 ({share.toFixed(1)}%)
                      </span>
                    </div>
                    <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                      <div className="h-full rounded-full bg-primary/80 transition-all" style={{ width: `${barWidth}%` }} />
                    </div>
                  </div>
                )
              })
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
