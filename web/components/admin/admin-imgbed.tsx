"use client"

import { useEffect, useState } from "react"
import { AlertCircle, RefreshCw } from "lucide-react"
import { api } from "@/lib/api"
import {
  imgbedError,
  type ImageApiKey,
  type QuotaResponse,
  type UploadConfig,
} from "@/lib/admin/imgbed"
import { ImgbedApiKeys } from "@/components/admin/imgbed-api-keys"
import { ImgbedChannelConfig } from "@/components/admin/imgbed-channel-config"
import { ImgbedIPBlacklist } from "@/components/admin/imgbed-ip-blacklist"
import { ImgbedRuntimeCard } from "@/components/admin/imgbed-runtime-card"
import { ImgbedSpaceOverview } from "@/components/admin/imgbed-space-overview"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

export function AdminImgbed() {
  const [healthy, setHealthy] = useState<boolean | null>(null)
  const [quota, setQuota] = useState<QuotaResponse | null>(null)
  const [config, setConfig] = useState<UploadConfig | null>(null)
  const [ips, setIPs] = useState<string[] | null>(null)
  const [keys, setKeys] = useState<ImageApiKey[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError("")

    Promise.allSettled([
      api<{ status: string }>("/api/v2/healthz", { signal: controller.signal, cache: "no-store" }),
      api<QuotaResponse>("/api/v2/manage/quota", { signal: controller.signal, cache: "no-store" }),
      api<UploadConfig>("/api/v2/manage/sysConfig/upload", { signal: controller.signal, cache: "no-store" }),
      api<{ ips: string[] }>("/api/v2/manage/cusConfig/blockipList", { signal: controller.signal, cache: "no-store" }),
      api<{ tokens: ImageApiKey[] }>("/api/v1/admin/image/keys", { signal: controller.signal, cache: "no-store" }),
    ]).then(([healthRes, quotaRes, configRes, ipRes, keysRes]) => {
      if (controller.signal.aborted) return

      const errors: string[] = []
      if (healthRes.status === "fulfilled") {
        setHealthy(healthRes.value.status === "ok")
      } else {
        setHealthy(false)
      }

      if (quotaRes.status === "fulfilled") setQuota(quotaRes.value)
      else errors.push(`空间占用：${imgbedError(quotaRes.reason)}`)

      if (configRes.status === "fulfilled") setConfig(configRes.value)
      else errors.push(`渠道配置：${imgbedError(configRes.reason)}`)

      if (ipRes.status === "fulfilled") setIPs(ipRes.value.ips || [])
      else errors.push(`IP 黑名单：${imgbedError(ipRes.reason)}`)

      if (keysRes.status === "fulfilled") setKeys(keysRes.value.tokens || [])
      else errors.push(`API Key：${imgbedError(keysRes.reason)}`)

      if (errors.length > 0) setError(errors.join("；"))
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })

    return () => controller.abort()
  }, [revision])

  const refresh = () => setRevision(value => value + 1)

  return (
    <section className="space-y-7 [&>[data-slot=card]]:border [&>[data-slot=card]]:border-border [&>[data-slot=card]]:shadow-none [&>[data-slot=card]]:ring-0" aria-labelledby="admin-imgbed-title">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <h1 id="admin-imgbed-title" className="text-2xl font-semibold tracking-tight">图床管理</h1>
          {healthy === true && <Badge variant="success">存储服务正常</Badge>}
          {healthy === false && <Badge variant="destructive">存储服务异常</Badge>}
        </div>
        <Button variant="outline" size="sm" disabled={loading} onClick={refresh}>
          <RefreshCw data-icon="inline-start" className={loading ? "animate-spin motion-reduce:animate-none" : undefined} />
          刷新数据
        </Button>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertCircle />
          <AlertDescription>
            {error}{" "}
            <Button variant="link" onClick={refresh}>重试</Button>
          </AlertDescription>
        </Alert>
      )}

      <ImgbedSpaceOverview
        quota={quota}
        config={config}
        blockedIPCount={ips?.length ?? 0}
        loading={loading}
      />

      <ImgbedChannelConfig
        config={config}
        quota={quota}
        loading={loading}
        onSaved={refresh}
      />

      <div className="grid gap-5 lg:grid-cols-2">
        <ImgbedIPBlacklist
          ips={ips}
          loading={loading}
          onSaved={next => setIPs(next)}
        />
        <ImgbedApiKeys
          keys={keys}
          loading={loading}
          onChanged={refresh}
        />
      </div>

      <ImgbedRuntimeCard
        runtime={quota?.runtime ?? null}
        loading={loading}
      />
    </section>
  )
}
