import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { WikiDocument } from "@/components/wiki-document"
import { authHref } from "@/components/auth-footer"
import { legalNames, type LegalDocument } from "@/lib/legal"

type Props = { params: Promise<{ slug: string }>; searchParams: Promise<{ return_to?: string; from?: string }> }
export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  return { title: `${slug === "privacy" || slug === "terms" ? legalNames[slug] : "文档"} · ALBATEXTUS Wiki` }
}
export default async function WikiDocumentPage({ params, searchParams }: Props) {
  const { slug } = await params
  if (slug !== "privacy" && slug !== "terms") notFound()
  const query = await searchParams
  const response = await fetch(`${process.env.OAUTH_BACKEND_URL || "http://localhost:8080"}/api/v1/legal/${slug}`, { cache: "no-store", signal: AbortSignal.timeout(10000) })
  if (!response.ok) throw new Error("暂时无法加载文档，请稍后重试。")
  const document: LegalDocument = await response.json()
  return <WikiDocument title={document.title} description={document.description} updatedAt={document.updated_at} content={document.content}>
    {(query.return_to || query.from) && <p className="wiki-return"><Link href={authHref(query.from === "register" ? "/register" : "/login", query.return_to)}>{query.from === "register" ? "返回注册" : "返回登录"}</Link></p>}
  </WikiDocument>
}
