"use client"

import { ServerCog } from "lucide-react"
import { formatBytes, formatDuration, type ImageRuntimeConfig } from "@/lib/admin/imgbed"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

export function ImgbedRuntimeCard({
  runtime,
  loading,
}: {
  runtime: ImageRuntimeConfig | null
  loading: boolean
}) {
  const items = runtime
    ? [
        { label: "公开访问根地址", value: runtime.publicURL, mono: true },
        { label: "单文件大小上限", value: formatBytes(runtime.maxUploadBytes) },
        { label: "单分块大小上限", value: formatBytes(runtime.maxChunkBytes) },
        { label: "单文件最大分块数", value: `${runtime.maxChunks.toLocaleString("zh-CN")} 块` },
        { label: "分块会话有效期", value: formatDuration(runtime.uploadTTLSeconds) },
        { label: "单账户并发分块会话上限", value: `${runtime.maxSessionsPerOwner.toLocaleString("zh-CN")} 个` },
      ]
    : []

  return (
    <Card aria-busy={loading}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ServerCog className="size-4 text-muted-foreground" aria-hidden="true" />
          <span>服务端运行参数</span>
        </CardTitle>
        <CardDescription>由环境变量配置并在后端启动时生效，仅作只读展示。</CardDescription>
      </CardHeader>
      <CardContent>
        {loading && !runtime ? (
          <Skeleton className="h-32 w-full" />
        ) : runtime ? (
          <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {items.map(item => (
              <div key={item.label} className="rounded-xl border border-border/80 bg-muted/20 px-3.5 py-2.5">
                <dt className="text-xs text-muted-foreground">{item.label}</dt>
                <dd className={`mt-1 text-sm font-medium ${item.mono ? "break-all font-mono text-xs" : "tabular-nums"}`}>
                  {item.value}
                </dd>
              </div>
            ))}
          </dl>
        ) : (
          <p className="py-4 text-sm text-muted-foreground">暂无运行参数信息</p>
        )}
      </CardContent>
    </Card>
  )
}
