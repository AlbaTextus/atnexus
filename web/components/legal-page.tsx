"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { AlertCircle, ArrowLeft } from "lucide-react"
import { api } from "@/lib/api"
import { legalNames, type LegalDocument, type LegalSlug } from "@/lib/legal"
import { authHref } from "@/components/auth-footer"
import { MarkdownContent } from "@/components/markdown-content"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

export type LegalPageParams = { searchParams: Promise<{ return_to?: string; from?: string }> }

export function LegalPage({ slug, returnTo, from }: { slug: LegalSlug; returnTo?: string; from?: string }) {
  const origin = from === "register" ? "register" : "login"
  const [document, setDocument] = useState<LegalDocument | null>(null)
  const [error, setError] = useState(false)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setDocument(null); setError(false)
    api<LegalDocument>(`/api/v1/legal/${slug}`, { signal: controller.signal, cache: "no-store" })
      .then(value => { if (!controller.signal.aborted) setDocument(value) })
      .catch(() => { if (!controller.signal.aborted) setError(true) })
    return () => controller.abort()
  }, [slug, retry])
  return <article className="flex min-w-0 flex-col gap-6" aria-labelledby="legal-title">
    {(returnTo || from) && <div><Button variant="ghost" size="sm" nativeButton={false} role="link" render={<Link href={authHref(origin === "register" ? "/register" : "/login", returnTo)} />}><ArrowLeft data-icon="inline-start" />{origin === "register" ? "返回注册" : "返回登录"}</Button></div>}
    <header><p className="mb-3 text-sm text-muted-foreground">政策与协议</p><h1 id="legal-title" className="text-3xl font-semibold">{document?.title || legalNames[slug]}</h1>{document && <><p className="mt-3 text-muted-foreground">{document.description}</p><p className="mt-3 text-xs text-muted-foreground">更新于 <time dateTime={document.updated_at}>{new Date(document.updated_at).toLocaleDateString("zh-CN")}</time></p></>}</header>
    {error ? <Alert variant="destructive"><AlertCircle /><AlertDescription>暂时无法加载文档，请稍后重试。<Button variant="link" onClick={() => setRetry(value => value + 1)}>重新加载</Button></AlertDescription></Alert> : document ? <MarkdownContent content={document.content} /> : <div className="flex flex-col gap-4" role="status" aria-label="正在加载文档"><Skeleton className="h-6 w-1/3" /><Skeleton className="h-64 w-full" /></div>}
  </article>
}
