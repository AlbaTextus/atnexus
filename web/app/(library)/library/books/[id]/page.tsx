import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { NovelDetail } from "@/components/novel/novel-detail"

export const metadata: Metadata = { title: "作品详情 · atnexus" }

export default async function NovelPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  if (!/^[1-9]\d*$/.test(id) || Number(id) > 2147483647) notFound()
  return <NovelDetail id={Number(id)} />
}
