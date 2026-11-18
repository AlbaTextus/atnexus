import type { Metadata } from "next"
import { NovelSubmissions } from "@/components/creatplat/novel-reviews"

export const metadata: Metadata = { title: "我的提交" }

export default function NovelSubmissionsPage() {
  return <NovelSubmissions />
}
