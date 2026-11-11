import type { Metadata } from "next"
import { NovelLibrary } from "@/components/novel-library"

export const metadata: Metadata = {
  title: "小说库 · atnexus",
  description: "atnexus 小说库。",
}

export default async function LibraryHome({ searchParams }: { searchParams: Promise<{ tagId?: string; authorId?: string }> }) {
  const { tagId, authorId } = await searchParams
  const validId = (value: string | undefined) => value && /^[1-9]\d*$/.test(value) && Number(value) <= 2147483647 ? Number(value) : undefined
  return <NovelLibrary tagId={validId(tagId)} authorId={validId(authorId)} />
}
