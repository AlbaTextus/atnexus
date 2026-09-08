import type { Metadata } from "next"
import { WikiDocument } from "@/components/wiki-document"
import { communityIntroduction } from "@/lib/wiki-community"

export const metadata: Metadata = { title: `${communityIntroduction.title} · AlbaTextus Wiki`, description: communityIntroduction.description }

export default function CommunityPage() {
  return <WikiDocument {...communityIntroduction} />
}
