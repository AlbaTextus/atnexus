import type { Metadata } from "next"
import { NovelReviews } from "@/components/novel-reviews"

export const metadata: Metadata = { title: "审核中心" }

export default function NovelReviewsPage() {
  return <NovelReviews />
}
