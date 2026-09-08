"use client"

import { useEffect, useState } from "react"
import { AlertCircle, Check, Copy } from "lucide-react"
import { api } from "@/lib/api"
import { type OAuthMetadata, managementError } from "@/lib/applications"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

function EndpointRow({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState("")
  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), 2000)
    return () => clearTimeout(timer)
  }, [copied])
  async function copy() {
    try { await navigator.clipboard.writeText(value); setCopied(true); setError("") }
    catch { setError("复制失败，请选中地址手动复制。") }
  }
  return <div className="wiki-endpoint-row">
    <dt>{label}</dt>
    <dd>
      <code>{value}</code>
      <Button variant="ghost" size="icon" type="button" onClick={copy} aria-label={`复制 ${label}`} title={copied ? "已复制" : "复制地址"}>{copied ? <Check /> : <Copy />}</Button>
      <span className="sr-only" role="status">{copied ? `${label} 已复制` : ""}</span>
      {error && <span className="wiki-endpoint-error" role="alert">{error}</span>}
    </dd>
  </div>
}

export function IntegrationEndpoints() {
  const [metadata, setMetadata] = useState<OAuthMetadata | null>(null)
  const [error, setError] = useState("")
  useEffect(() => {
    const controller = new AbortController()
    api<OAuthMetadata>("/.well-known/oauth-authorization-server", { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) setMetadata(value) })
      .catch(reason => { if (!controller.signal.aborted) setError(managementError(reason)) })
    return () => controller.abort()
  }, [])
  if (error) return <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>
  if (!metadata) return <p role="status">正在加载服务端点…</p>
  return <dl className="wiki-endpoints" aria-label="OAuth 服务端点">
    <EndpointRow label="Issuer" value={metadata.issuer} />
    <EndpointRow label="Authorization Endpoint" value={metadata.authorization_endpoint} />
    <EndpointRow label="Token Endpoint" value={metadata.token_endpoint} />
    <EndpointRow label="User Endpoint" value={metadata.userinfo_endpoint} />
    <EndpointRow label="Revocation Endpoint" value={metadata.revocation_endpoint} />
  </dl>
}
