"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { AlertCircle, ArrowLeft } from "lucide-react"
import { api } from "@/lib/api"
import { legalNames, type LegalDocument, type LegalSlug } from "@/lib/legal"
import { AuthLayout } from "@/components/auth-layout"
import { AuthFooter, authHref } from "@/components/auth-footer"
import { MarkdownContent } from "@/components/markdown-content"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
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
  return <AuthLayout>
    <article className="flex w-full max-w-3xl flex-col gap-4" aria-labelledby="legal-title">
      <div><Button variant="ghost" size="sm" nativeButton={false} role="link" render={<Link href={authHref(origin === "register" ? "/register" : "/login", returnTo)} />}><ArrowLeft data-icon="inline-start" />{origin === "register" ? "返回注册" : "返回登录"}</Button></div>
      <Card>
        <CardHeader><p className="mb-2 text-xs text-muted-foreground">AlbaTextus 账户中心</p><CardTitle><h1 id="legal-title">{document?.title || legalNames[slug]}</h1></CardTitle>{document && <><CardDescription>{document.description}</CardDescription><p className="mt-2 text-xs text-muted-foreground">更新于 <time dateTime={document.updated_at}>{new Date(document.updated_at).toLocaleDateString("zh-CN")}</time></p></>}</CardHeader>
        <CardContent>
          {error ? <Alert variant="destructive"><AlertCircle /><AlertDescription>暂时无法加载协议，请稍后重试。<Button variant="link" onClick={() => setRetry(value => value + 1)}>重新加载</Button></AlertDescription></Alert> : document ? <MarkdownContent content={document.content} /> : <div className="flex flex-col gap-4" role="status" aria-label="正在加载协议"><Skeleton className="h-6 w-1/3" /><Skeleton className="h-32 w-full" /><Skeleton className="h-32 w-full" /></div>}
        </CardContent>
        <CardFooter><AuthFooter returnTo={returnTo} from={origin} /></CardFooter>
      </Card>
    </article>
  </AuthLayout>
}
