"use client"

import { useEffect, useState, type ReactNode } from "react"
import Image from "next/image"
import { AlertCircle, Globe, UserRoundPlus } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { feedback } from "@/lib/feedback"
import { ProviderIcon } from "@/components/provider-icon"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"

type FeatureKey = "external_login_enabled" | "registration_enabled" | "turnstile_enabled"
type SettingTarget = { feature: FeatureKey } | { provider: string }

type LoginProviderSetting = {
  id: string
  name: string
  enabled: boolean
  configured: boolean
}

type SiteFeatureSettings = Record<FeatureKey, boolean> & {
  provider_configured: boolean
  turnstile_configured: boolean
  providers: LoginProviderSetting[]
  updated_at: string
}

const featureLabels: Record<FeatureKey, string> = {
  external_login_enabled: "第三方登录",
  registration_enabled: "全站注册",
  turnstile_enabled: "Cloudflare 验证",
}

export function SiteFeatureControls() {
  const [settings, setSettings] = useState<SiteFeatureSettings | null>(null)
  const [pending, setPending] = useState<string | null>(null)
  const [error, setError] = useState("")
  const [retry, setRetry] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setError("")
    api<SiteFeatureSettings>("/api/v1/admin/features", { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) setSettings(value) })
      .catch(reason => { if (!controller.signal.aborted) setError(managementError(reason)) })
    return () => controller.abort()
  }, [retry])

  async function update(target: SettingTarget, enabled: boolean, label: string) {
    if (!settings || pending) return
    setPending("feature" in target ? target.feature : `provider:${target.provider}`)
    setError("")
    try {
      const updated = await api<SiteFeatureSettings>("/api/v1/admin/features", {
        method: "PATCH",
        body: JSON.stringify({ ...target, enabled }),
      })
      setSettings(updated)
      feedback.success(`${label}已${enabled ? "开启" : "关闭"}`)
    } catch (reason) {
      setError(managementError(reason))
    } finally {
      setPending(null)
    }
  }

  const rows: Array<{ key: FeatureKey; description: string; icon: ReactNode }> = [
    {
      key: "external_login_enabled",
      description: "统一控制第三方账号登录与绑定，下方可分别设置各平台。",
      icon: <Globe aria-hidden="true" />,
    },
    {
      key: "registration_enabled",
      description: "控制邮箱注册和第三方首次建号；关闭后已有账户仍可登录。",
      icon: <UserRoundPlus aria-hidden="true" />,
    },
    {
      key: "turnstile_enabled",
      description: settings && !settings.turnstile_configured
        ? "当前未配置 Cloudflare 密钥；开启后仍需完成环境配置。"
        : "控制登录、注册和通行密钥操作的人机验证。",
      icon: <Image src="/brands/cloudflare.svg" alt="" width={20} height={20} className="size-4 shrink-0 object-contain" aria-hidden="true" />,
    },
  ]

  return <Card>
    <CardHeader>
      <CardTitle>功能开关</CardTitle>
      <CardDescription>设置会立即作用于整个账户中心。</CardDescription>
    </CardHeader>
    <CardContent className="flex flex-col gap-4">
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error} <Button variant="link" onClick={() => setRetry(value => value + 1)}>重试</Button></AlertDescription></Alert>}
      {!settings ? <div className="flex flex-col gap-3" aria-label="正在加载功能开关"><Skeleton className="h-14 w-full" /><Skeleton className="h-14 w-full" /><Skeleton className="h-14 w-full" /></div> : <FieldGroup className="gap-4">
        {rows.map(row => <div key={row.key} className="flex flex-col gap-4 rounded-xl border border-border p-4 sm:p-5">
          <Field orientation="horizontal" data-disabled={Boolean(pending)} className="gap-4">
            <FieldContent className="min-w-0">
              <FieldLabel htmlFor={`site-feature-${row.key}`}>{row.icon}{featureLabels[row.key]}</FieldLabel>
              <FieldDescription>{row.description}</FieldDescription>
            </FieldContent>
            <Switch
              id={`site-feature-${row.key}`}
              checked={settings[row.key]}
              disabled={Boolean(pending)}
              aria-label={`${settings[row.key] ? "关闭" : "开启"}${featureLabels[row.key]}`}
              aria-busy={pending === row.key}
              onCheckedChange={checked => void update({ feature: row.key }, checked, featureLabels[row.key])}
            />
          </Field>
          {row.key === "external_login_enabled" && <FieldGroup className="gap-4 border-t border-border pt-4 pl-4 sm:pl-6">
            {settings.providers?.map(provider => <Field key={provider.id} orientation="horizontal" className="gap-4" data-disabled={Boolean(pending) || !settings.external_login_enabled}>
              <FieldContent className="min-w-0">
                <FieldLabel htmlFor={`site-provider-${provider.id}`}><ProviderIcon provider={provider.id} />{provider.name}</FieldLabel>
                <FieldDescription id={`site-provider-${provider.id}-description`}>
                  {!settings.external_login_enabled
                    ? "总开关已关闭，开启后按此平台的设置生效。"
                    : !provider.configured
                      ? "尚未配置，此登录方式暂不可用。"
                      : `允许使用 ${provider.name} 登录与绑定账号。`}
                </FieldDescription>
              </FieldContent>
              <Switch
                id={`site-provider-${provider.id}`}
                checked={provider.enabled}
                disabled={Boolean(pending) || !settings.external_login_enabled}
                aria-label={`${provider.enabled ? "关闭" : "开启"}${provider.name}登录`}
                aria-describedby={`site-provider-${provider.id}-description`}
                aria-busy={pending === `provider:${provider.id}`}
                onCheckedChange={checked => void update({ provider: provider.id }, checked, `${provider.name}登录`)}
              />
            </Field>)}
          </FieldGroup>}
        </div>)}
      </FieldGroup>}
    </CardContent>
  </Card>
}
