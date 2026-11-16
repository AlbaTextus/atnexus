import type { Metadata } from "next"
import { NovelSubmit } from "@/components/novel-submit"

export const metadata: Metadata = { title: "提交小说" }

export default function UploadNovelPage() {
  return <NovelSubmit />
}
