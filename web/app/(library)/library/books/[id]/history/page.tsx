import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { NovelHistoryPage } from "@/components/novel-history"

export const metadata: Metadata = { title: "变更历史 · atnexus" }

export default async function HistoryPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  if (!/^[1-9]\d*$/.test(id) || Number(id) > 2147483647) notFound()
  return <NovelHistoryPage id={Number(id)} />
}
