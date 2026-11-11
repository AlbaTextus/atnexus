import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { findNovelSection } from "@/lib/novel-sections"
import { NovelConnection } from "@/components/novel-access"
import { NovelDirectory } from "@/components/novel-directory"

type Props = { params: Promise<{ section: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const section = findNovelSection((await params).section)
  if (!section || section.id === "library") notFound()
  return { title: `${section.title} · atnexus` }
}

export default async function LibraryPage({ params }: Props) {
  const section = findNovelSection((await params).section)
  if (!section || section.id === "library") notFound()
  if (section.id === "settings") return <div className="novel-demo"><div className="page-shell"><h1>设置</h1><NovelConnection /></div></div>
  if (section.id === "categories") return <NovelDirectory kind="tags" />
  if (section.id === "sources") return <NovelDirectory kind="platforms" />
  notFound()
}
