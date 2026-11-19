import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { findNovelSection } from "@/lib/novel/novel-sections"
import { NovelResources } from "@/components/novel/novel-resources"

type Props = { params: Promise<{ section: string }> }

function resolveSection(id: string) {
  const section = findNovelSection(id)
  if (!section || section.id === "library") notFound()
  return section
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const section = resolveSection((await params).section)
  return { title: `${section.title} · atnexus` }
}

export default async function LibraryPage({ params }: Props) {
  const section = resolveSection((await params).section)
  if (section.id === "resources") return <NovelResources />
  notFound()
}
