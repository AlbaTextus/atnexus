"use client"

import { useState, type FormEvent } from "react"
import { AlertCircle, Cloud, Eye, EyeOff, HardDrive, Pencil, Plus, Trash2 } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import {
  channelBytes,
  formatBytes,
  imgbedError,
  type QuotaResponse,
  type StorageChannel,
  type UploadConfig,
} from "@/lib/admin/imgbed"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"

type S3FormState = {
  mode: "create" | "edit"
  originalName?: string
  name: string
  enabled: boolean
  endpoint: string
  bucketName: string
  region: string
  accessKeyId: string
  secretAccessKey: string
  pathStyle: boolean
  cdnDomain: string
  savePath: string
  quotaEnabled: boolean
  limitGB: string
  threshold: string
}

type LocalFormState = {
  name: string
  enabled: boolean
  savePath?: string
  quotaEnabled: boolean
  limitGB: string
  threshold: string
}

function emptyS3Form(): S3FormState {
  return {
    mode: "create",
    name: "",
    enabled: true,
    endpoint: "",
    bucketName: "",
    region: "auto",
    accessKeyId: "",
    secretAccessKey: "",
    pathStyle: false,
    cdnDomain: "",
    savePath: "",
    quotaEnabled: false,
    limitGB: "10",
    threshold: "95",
  }
}

function s3FormFromChannel(channel: StorageChannel): S3FormState {
  return {
    mode: "edit",
    originalName: channel.name,
    name: channel.name,
    enabled: channel.enabled,
    endpoint: channel.endpoint || "",
    bucketName: channel.bucketName || "",
    region: channel.region || "",
    accessKeyId: "",
    secretAccessKey: "",
    pathStyle: Boolean(channel.pathStyle),
    cdnDomain: channel.cdnDomain || "",
    savePath: channel.savePath || "",
    quotaEnabled: Boolean(channel.quota?.enabled),
    limitGB: String(channel.quota?.limitGB ?? 0),
    threshold: String(channel.quota?.threshold ?? 95),
  }
}

function validateQuotaInputs(quotaEnabled: boolean, limitRaw: string, thresholdRaw: string) {
  const limitGB = limitRaw.trim() === "" ? 0 : Number(limitRaw)
  const threshold = thresholdRaw.trim() === "" ? 95 : Number(thresholdRaw)
  if (!Number.isFinite(limitGB) || limitGB < 0) {
    return { error: "配额上限（GB）必须为大于等于 0 的数字。" }
  }
  if (quotaEnabled && limitGB <= 0) {
    return { error: "开启配额限制时，容量上限（GB）须大于 0。" }
  }
  if (!Number.isFinite(threshold) || threshold < 0 || threshold > 100) {
    return { error: "告警阈值必须介于 0 到 100 之间。" }
  }
  return { quota: { enabled: quotaEnabled, limitGB, threshold } }
}

function validateEndpoint(raw: string): string | null {
  try {
    const u = new URL(raw.trim())
    if ((u.protocol !== "http:" && u.protocol !== "https:") || !u.host || u.username || u.password || u.search || u.hash) {
      return "Endpoint 须为有效的 HTTP/HTTPS 地址，且不能包含用户名、查询参数或片段。"
    }
    return null
  } catch {
    return "Endpoint 须为有效的 HTTP/HTTPS URL。"
  }
}

export function ImgbedChannelConfig({
  config,
  quota,
  loading,
  onSaved,
}: {
  config: UploadConfig | null
  quota: QuotaResponse | null
  loading: boolean
  onSaved: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [s3Form, setS3Form] = useState<S3FormState | null>(null)
  const [localForm, setLocalForm] = useState<LocalFormState | null>(null)
  const [deletingS3, setDeletingS3] = useState<StorageChannel | null>(null)
  const [dialogError, setDialogError] = useState("")
  const [showSecret, setShowSecret] = useState(false)

  const channelStats = quota?.channelStats || quota?.quotaStats || {}

  async function persistConfig(next: UploadConfig, message: string) {
    setBusy(true)
    setError("")
    setDialogError("")
    try {
      await api<{ success: boolean }>("/api/v2/manage/sysConfig/upload", {
        method: "POST",
        body: JSON.stringify(next),
      })
      feedback.success(message)
      onSaved()
      return true
    } catch (reason) {
      const msg = imgbedError(reason)
      setError(msg)
      setDialogError(msg)
      return false
    } finally {
      setBusy(false)
    }
  }

  async function toggleLoadBalance(group: "cfr2" | "s3", enabled: boolean) {
    if (!config || busy) return
    const next: UploadConfig = {
      ...config,
      [group]: { ...config[group], loadBalance: { enabled } },
    }
    await persistConfig(next, `${group === "cfr2" ? "本地" : "S3 "}渠道负载均衡已${enabled ? "开启" : "关闭"}`)
  }

  async function toggleChannelEnabled(group: "cfr2" | "s3", name: string, enabled: boolean) {
    if (!config || busy) return
    const next: UploadConfig = {
      ...config,
      [group]: {
        ...config[group],
        channels: config[group].channels.map(c => c.name === name ? { ...c, enabled } : c),
      },
    }
    await persistConfig(next, `渠道 ${name} 已${enabled ? "启用" : "停用"}`)
  }

  async function submitLocalForm(event: FormEvent) {
    event.preventDefault()
    if (!config || !localForm || busy) return
    const validated = validateQuotaInputs(localForm.quotaEnabled, localForm.limitGB, localForm.threshold)
    if (validated.error || !validated.quota) {
      setDialogError(validated.error || "参数无效")
      return
    }
    const next: UploadConfig = {
      ...config,
      cfr2: {
        ...config.cfr2,
        channels: config.cfr2.channels.map(c =>
          c.name === localForm.name
            ? {
                ...c,
                enabled: localForm.enabled,
                savePath: c.name === "R2_env" ? "environment variable" : localForm.savePath,
                quota: validated.quota!,
              }
            : c,
        ),
      },
    }
    const ok = await persistConfig(next, `本地渠道 ${localForm.name} 配置已保存`)
    if (ok) setLocalForm(null)
  }

  async function submitS3Form(event: FormEvent) {
    event.preventDefault()
    if (!config || !s3Form || busy) return
    const name = s3Form.name.trim()
    if (!name) {
      setDialogError("请填写渠道名称。")
      return
    }
    if (s3Form.mode === "create" && config.s3.channels.some(c => c.name === name)) {
      setDialogError(`S3 渠道「${name}」已存在，请使用其他名称。`)
      return
    }
    const endpointErr = validateEndpoint(s3Form.endpoint)
    if (endpointErr) {
      setDialogError(endpointErr)
      return
    }
    if (!s3Form.bucketName.trim()) {
      setDialogError("请填写 Bucket 名称。")
      return
    }
    if (s3Form.mode === "create" && (!s3Form.accessKeyId.trim() || !s3Form.secretAccessKey.trim())) {
      setDialogError("新增 S3 渠道时必须填写 Access Key ID 与 Secret Access Key。")
      return
    }
    const validated = validateQuotaInputs(s3Form.quotaEnabled, s3Form.limitGB, s3Form.threshold)
    if (validated.error || !validated.quota) {
      setDialogError(validated.error || "配额参数无效")
      return
    }

    const channelPayload: StorageChannel = {
      name,
      enabled: s3Form.enabled,
      endpoint: s3Form.endpoint.trim(),
      bucketName: s3Form.bucketName.trim(),
      region: s3Form.region.trim() || undefined,
      accessKeyId: s3Form.accessKeyId.trim() || undefined,
      secretAccessKey: s3Form.secretAccessKey.trim() || undefined,
      pathStyle: s3Form.pathStyle,
      cdnDomain: s3Form.cdnDomain.trim() || undefined,
      savePath: s3Form.savePath.trim() || undefined,
      quota: validated.quota,
    }

    const nextChannels = s3Form.mode === "create"
      ? [...config.s3.channels, channelPayload]
      : config.s3.channels.map(c => c.name === s3Form.originalName ? channelPayload : c)

    const next: UploadConfig = {
      ...config,
      s3: { ...config.s3, channels: nextChannels },
    }
    const ok = await persistConfig(next, s3Form.mode === "create" ? `已添加 S3 渠道 ${name}` : `S3 渠道 ${name} 已更新`)
    if (ok) {
      setS3Form(null)
      setShowSecret(false)
    }
  }

  async function confirmDeleteS3() {
    if (!config || !deletingS3 || busy) return
    const next: UploadConfig = {
      ...config,
      s3: {
        ...config.s3,
        channels: config.s3.channels.filter(c => c.name !== deletingS3.name),
      },
    }
    const ok = await persistConfig(next, `已删除 S3 渠道 ${deletingS3.name}`)
    if (ok) setDeletingS3(null)
  }

  return (
    <Card aria-busy={loading || busy}>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle>存储渠道与配额配置</CardTitle>
          <CardDescription>管理本地与 S3 对象存储渠道、负载均衡与单渠道容量配额（修改后最多 5 秒内全节点生效）。</CardDescription>
        </div>
        <Button
          size="sm"
          disabled={loading || busy || !config}
          onClick={() => {
            setDialogError("")
            setShowSecret(false)
            setS3Form(emptyS3Form())
          }}
        >
          <Plus data-icon="inline-start" />
          新增 S3 渠道
        </Button>
      </CardHeader>

      <CardContent className="space-y-6">
        {error && (
          <Alert variant="destructive">
            <AlertCircle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {loading && !config ? (
          <div className="space-y-4">
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-40 w-full" />
          </div>
        ) : config && (
          <>
            <div className="space-y-3 rounded-xl border border-border p-4">
              <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-3">
                <div className="flex items-center gap-2">
                  <HardDrive className="size-4 text-muted-foreground" aria-hidden="true" />
                  <div>
                    <h3 className="font-medium">本地存储渠道 (cfr2)</h3>
                    <p className="text-xs text-muted-foreground">基于环境变量 IMAGE_LOCAL_ROOT 挂载的本地对象目录。</p>
                  </div>
                </div>
                <div className="flex items-center gap-2.5">
                  <span className="text-xs text-muted-foreground">负载均衡</span>
                  <Switch
                    checked={config.cfr2.loadBalance.enabled}
                    disabled={busy}
                    aria-label="本地渠道负载均衡"
                    onCheckedChange={checked => void toggleLoadBalance("cfr2", checked)}
                  />
                </div>
              </div>

              <div className="space-y-2.5">
                {config.cfr2.channels.map(channel => {
                  const usage = channelStats[channel.name]
                  const usedBytes = channelBytes(usage)
                  return (
                    <div key={channel.name} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border/70 bg-muted/20 px-3.5 py-3">
                      <div className="min-w-0 space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{channel.name}</span>
                          {channel.savePath === "environment variable" && <Badge variant="secondary">默认环境目录</Badge>}
                          <Badge variant={channel.enabled ? "success" : "outline"}>
                            {channel.enabled ? "已启用" : "已停用"}
                          </Badge>
                        </div>
                        <p className="text-xs text-muted-foreground tabular-nums">
                          已用 {formatBytes(usedBytes)}（{usage?.fileCount ?? 0} 个文件） ·{" "}
                          {channel.quota?.enabled && channel.quota.limitGB > 0
                            ? `配额上限 ${channel.quota.limitGB} GB（阈值 ${channel.quota.threshold || 95}%）`
                            : "未启用配额上限"}
                        </p>
                      </div>
                      <div className="flex items-center gap-2.5">
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy}
                          onClick={() => {
                            setDialogError("")
                            setLocalForm({
                              name: channel.name,
                              enabled: channel.enabled,
                              savePath: channel.savePath,
                              quotaEnabled: Boolean(channel.quota?.enabled),
                              limitGB: String(channel.quota?.limitGB ?? 0),
                              threshold: String(channel.quota?.threshold ?? 95),
                            })
                          }}
                        >
                          <Pencil data-icon="inline-start" />
                          配额设置
                        </Button>
                        <Switch
                          checked={channel.enabled}
                          disabled={busy}
                          aria-label={`启用本地渠道 ${channel.name}`}
                          onCheckedChange={checked => void toggleChannelEnabled("cfr2", channel.name, checked)}
                        />
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>

            <div className="space-y-3 rounded-xl border border-border p-4">
              <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-3">
                <div className="flex items-center gap-2">
                  <Cloud className="size-4 text-muted-foreground" aria-hidden="true" />
                  <div>
                    <h3 className="font-medium">S3 对象存储渠道 (s3)</h3>
                    <p className="text-xs text-muted-foreground">兼容 Cloudflare R2、AWS S3、MinIO 等对象存储；凭据由后端安全保存。</p>
                  </div>
                </div>
                <div className="flex items-center gap-2.5">
                  <span className="text-xs text-muted-foreground">负载均衡</span>
                  <Switch
                    checked={config.s3.loadBalance.enabled}
                    disabled={busy}
                    aria-label="S3 渠道负载均衡"
                    onCheckedChange={checked => void toggleLoadBalance("s3", checked)}
                  />
                </div>
              </div>

              {config.s3.channels.length === 0 ? (
                <div className="rounded-lg border border-dashed border-border py-8 text-center text-sm text-muted-foreground">
                  尚未配置 S3 对象存储渠道，点击右上角「新增 S3 渠道」即可接入。
                </div>
              ) : (
                <div className="space-y-2.5">
                  {config.s3.channels.map(channel => {
                    const usage = channelStats[channel.name]
                    const usedBytes = channelBytes(usage)
                    return (
                      <div key={channel.name} className="flex flex-wrap items-start justify-between gap-3 rounded-lg border border-border/70 bg-muted/20 px-3.5 py-3">
                        <div className="min-w-0 flex-1 space-y-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-medium">{channel.name}</span>
                            <Badge variant={channel.enabled ? "success" : "outline"}>
                              {channel.enabled ? "已启用" : "已停用"}
                            </Badge>
                            {channel.pathStyle && <Badge variant="secondary">Path Style</Badge>}
                          </div>
                          <p className="break-all text-xs text-muted-foreground">
                            {channel.endpoint} · Bucket: <code>{channel.bucketName}</code> · Region: <code>{channel.region || "auto"}</code>
                            {channel.cdnDomain ? <> · CDN: <code>{channel.cdnDomain}</code></> : null}
                          </p>
                          <p className="text-xs text-muted-foreground tabular-nums">
                            已用 {formatBytes(usedBytes)}（{usage?.fileCount ?? 0} 个文件） ·{" "}
                            {channel.quota?.enabled && channel.quota.limitGB > 0
                              ? `配额上限 ${channel.quota.limitGB} GB（阈值 ${channel.quota.threshold || 95}%）`
                              : "未启用配额上限"}
                          </p>
                        </div>
                        <div className="flex items-center gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy}
                            onClick={() => {
                              setDialogError("")
                              setShowSecret(false)
                              setS3Form(s3FormFromChannel(channel))
                            }}
                          >
                            <Pencil data-icon="inline-start" />
                            编辑
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy}
                            aria-label={`删除渠道 ${channel.name}`}
                            onClick={() => {
                              setDialogError("")
                              setDeletingS3(channel)
                            }}
                          >
                            <Trash2 className="size-3.5 text-destructive" />
                          </Button>
                          <Switch
                            checked={channel.enabled}
                            disabled={busy}
                            aria-label={`启用 S3 渠道 ${channel.name}`}
                            onCheckedChange={checked => void toggleChannelEnabled("s3", channel.name, checked)}
                          />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </>
        )}
      </CardContent>

      <Dialog open={Boolean(localForm)} onOpenChange={open => { if (!open && !busy) setLocalForm(null) }}>
        <DialogContent className="center-dialog sm:max-w-md" showCloseButton={!busy}>
          <DialogHeader>
            <DialogTitle>本地渠道配额设置 · {localForm?.name}</DialogTitle>
            <DialogDescription>启用配额后，当估算用量达到上限的告警阈值百分比时将停止向此渠道写入新文件。</DialogDescription>
          </DialogHeader>
          {localForm && (
            <form onSubmit={event => void submitLocalForm(event)} className="space-y-4">
              {dialogError && (
                <Alert variant="destructive">
                  <AlertCircle />
                  <AlertDescription>{dialogError}</AlertDescription>
                </Alert>
              )}
              <FieldGroup className="gap-4">
                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldLabel htmlFor="local-enabled">启用该渠道</FieldLabel>
                    <FieldDescription>关闭后新上传将不再选择此渠道。</FieldDescription>
                  </FieldContent>
                  <Switch
                    id="local-enabled"
                    checked={localForm.enabled}
                    disabled={busy}
                    onCheckedChange={checked => setLocalForm(prev => prev ? { ...prev, enabled: checked } : prev)}
                  />
                </Field>
                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldLabel htmlFor="local-quota-enabled">开启容量配额</FieldLabel>
                    <FieldDescription>限制此本地渠道允许使用的最大空间。</FieldDescription>
                  </FieldContent>
                  <Switch
                    id="local-quota-enabled"
                    checked={localForm.quotaEnabled}
                    disabled={busy}
                    onCheckedChange={checked => setLocalForm(prev => prev ? { ...prev, quotaEnabled: checked } : prev)}
                  />
                </Field>
                <div className="grid grid-cols-2 gap-3">
                  <Field>
                    <FieldLabel htmlFor="local-limit-gb">容量上限 (GB)</FieldLabel>
                    <Input
                      id="local-limit-gb"
                      type="number"
                      step="0.1"
                      min="0"
                      disabled={busy || !localForm.quotaEnabled}
                      value={localForm.limitGB}
                      onChange={event => setLocalForm(prev => prev ? { ...prev, limitGB: event.target.value } : prev)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="local-threshold">写入阈值 (%)</FieldLabel>
                    <Input
                      id="local-threshold"
                      type="number"
                      step="1"
                      min="0"
                      max="100"
                      disabled={busy || !localForm.quotaEnabled}
                      value={localForm.threshold}
                      onChange={event => setLocalForm(prev => prev ? { ...prev, threshold: event.target.value } : prev)}
                    />
                  </Field>
                </div>
              </FieldGroup>
              <DialogFooter>
                <Button type="button" variant="outline" disabled={busy} onClick={() => setLocalForm(null)}>取消</Button>
                <Button type="submit" disabled={busy}>
                  {busy && <Spinner data-icon="inline-start" />}
                  保存设置
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(s3Form)} onOpenChange={open => { if (!open && !busy) setS3Form(null) }}>
        <DialogContent className="center-dialog max-h-[85vh] overflow-y-auto sm:max-w-lg" showCloseButton={!busy}>
          <DialogHeader>
            <DialogTitle>{s3Form?.mode === "create" ? "新增 S3 存储渠道" : `编辑 S3 渠道 · ${s3Form?.name}`}</DialogTitle>
            <DialogDescription>
              {s3Form?.mode === "edit"
                ? "出于安全策略，后端不回传已保存的 AccessKey 与 SecretKey；留空将保留现有凭据。"
                : "填写 S3 兼容存储端点与访问凭据。"}
            </DialogDescription>
          </DialogHeader>
          {s3Form && (
            <form onSubmit={event => void submitS3Form(event)} className="space-y-4">
              {dialogError && (
                <Alert variant="destructive">
                  <AlertCircle />
                  <AlertDescription>{dialogError}</AlertDescription>
                </Alert>
              )}
              <FieldGroup className="gap-3.5">
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="s3-name">渠道名称</FieldLabel>
                    <Input
                      id="s3-name"
                      required
                      disabled={busy || s3Form.mode === "edit"}
                      placeholder="例如：r2_primary"
                      value={s3Form.name}
                      onChange={event => setS3Form(prev => prev ? { ...prev, name: event.target.value } : prev)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="s3-region">Region</FieldLabel>
                    <Input
                      id="s3-region"
                      disabled={busy}
                      placeholder="auto"
                      value={s3Form.region}
                      onChange={event => setS3Form(prev => prev ? { ...prev, region: event.target.value } : prev)}
                    />
                  </Field>
                </div>

                <Field>
                  <FieldLabel htmlFor="s3-endpoint">Endpoint</FieldLabel>
                  <Input
                    id="s3-endpoint"
                    type="url"
                    required
                    disabled={busy}
                    placeholder="https://xxx.r2.cloudflarestorage.com"
                    value={s3Form.endpoint}
                    onChange={event => setS3Form(prev => prev ? { ...prev, endpoint: event.target.value } : prev)}
                  />
                </Field>

                <div className="grid gap-3 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="s3-bucket">Bucket 名称</FieldLabel>
                    <Input
                      id="s3-bucket"
                      required
                      disabled={busy}
                      placeholder="my-images-bucket"
                      value={s3Form.bucketName}
                      onChange={event => setS3Form(prev => prev ? { ...prev, bucketName: event.target.value } : prev)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="s3-cdn">CDN 域名（可选）</FieldLabel>
                    <Input
                      id="s3-cdn"
                      disabled={busy}
                      placeholder="https://cdn.example.com"
                      value={s3Form.cdnDomain}
                      onChange={event => setS3Form(prev => prev ? { ...prev, cdnDomain: event.target.value } : prev)}
                    />
                  </Field>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="s3-ak">Access Key ID</FieldLabel>
                    <Input
                      id="s3-ak"
                      required={s3Form.mode === "create"}
                      disabled={busy}
                      placeholder={s3Form.mode === "edit" ? "留空保留现有 AccessKey" : "必填"}
                      value={s3Form.accessKeyId}
                      onChange={event => setS3Form(prev => prev ? { ...prev, accessKeyId: event.target.value } : prev)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="s3-sk">Secret Access Key</FieldLabel>
                    <div className="relative flex items-center">
                      <Input
                        id="s3-sk"
                        type={showSecret ? "text" : "password"}
                        required={s3Form.mode === "create"}
                        disabled={busy}
                        className="pr-9"
                        placeholder={s3Form.mode === "edit" ? "留空保留现有 Secret" : "必填"}
                        value={s3Form.secretAccessKey}
                        onChange={event => setS3Form(prev => prev ? { ...prev, secretAccessKey: event.target.value } : prev)}
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        className="absolute right-1"
                        aria-label={showSecret ? "隐藏 Secret" : "显示 Secret"}
                        onClick={() => setShowSecret(v => !v)}
                      >
                        {showSecret ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                      </Button>
                    </div>
                  </Field>
                </div>

                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldLabel htmlFor="s3-path-style">使用 Path-Style 访问</FieldLabel>
                    <FieldDescription>MinIO 或自建对象存储通常需要开启。</FieldDescription>
                  </FieldContent>
                  <Switch
                    id="s3-path-style"
                    checked={s3Form.pathStyle}
                    disabled={busy}
                    onCheckedChange={checked => setS3Form(prev => prev ? { ...prev, pathStyle: checked } : prev)}
                  />
                </Field>

                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldLabel htmlFor="s3-enabled">启用该渠道</FieldLabel>
                  </FieldContent>
                  <Switch
                    id="s3-enabled"
                    checked={s3Form.enabled}
                    disabled={busy}
                    onCheckedChange={checked => setS3Form(prev => prev ? { ...prev, enabled: checked } : prev)}
                  />
                </Field>

                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldLabel htmlFor="s3-quota-enabled">开启容量配额</FieldLabel>
                  </FieldContent>
                  <Switch
                    id="s3-quota-enabled"
                    checked={s3Form.quotaEnabled}
                    disabled={busy}
                    onCheckedChange={checked => setS3Form(prev => prev ? { ...prev, quotaEnabled: checked } : prev)}
                  />
                </Field>

                {s3Form.quotaEnabled && (
                  <div className="grid grid-cols-2 gap-3">
                    <Field>
                      <FieldLabel htmlFor="s3-limit-gb">容量上限 (GB)</FieldLabel>
                      <Input
                        id="s3-limit-gb"
                        type="number"
                        step="0.1"
                        min="0"
                        disabled={busy}
                        value={s3Form.limitGB}
                        onChange={event => setS3Form(prev => prev ? { ...prev, limitGB: event.target.value } : prev)}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="s3-threshold">写入阈值 (%)</FieldLabel>
                      <Input
                        id="s3-threshold"
                        type="number"
                        step="1"
                        min="0"
                        max="100"
                        disabled={busy}
                        value={s3Form.threshold}
                        onChange={event => setS3Form(prev => prev ? { ...prev, threshold: event.target.value } : prev)}
                      />
                    </Field>
                  </div>
                )}
              </FieldGroup>
              <DialogFooter>
                <Button type="button" variant="outline" disabled={busy} onClick={() => setS3Form(null)}>取消</Button>
                <Button type="submit" disabled={busy}>
                  {busy && <Spinner data-icon="inline-start" />}
                  {s3Form.mode === "create" ? "添加渠道" : "保存修改"}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(deletingS3)} onOpenChange={open => { if (!open && !busy) setDeletingS3(null) }}>
        <DialogContent className="center-dialog" showCloseButton={!busy}>
          <DialogHeader>
            <DialogTitle>删除 S3 渠道 · {deletingS3?.name}</DialogTitle>
            <DialogDescription>删除后，新上传将不再使用此配置，且依赖此渠道名称的文件将无法读取。</DialogDescription>
          </DialogHeader>
          {deletingS3 && (channelStats[deletingS3.name]?.fileCount ?? 0) > 0 && (
            <Alert variant="destructive">
              <AlertCircle />
              <AlertDescription>
                当前仍有 {channelStats[deletingS3.name]?.fileCount} 个文件关联此渠道，建议改为「停用」而非删除。
              </AlertDescription>
            </Alert>
          )}
          {dialogError && (
            <Alert variant="destructive">
              <AlertCircle />
              <AlertDescription>{dialogError}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button variant="outline" disabled={busy} onClick={() => setDeletingS3(null)}>取消</Button>
            <Button variant="destructive" disabled={busy} onClick={() => void confirmDeleteS3()}>
              {busy && <Spinner data-icon="inline-start" />}
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
