import type { Metadata } from "next"
import { WikiDocument } from "@/components/wiki-document"
import { communityLevels } from "@/lib/wiki-community"

export const metadata: Metadata = { title: `${communityLevels.title} · ALBATEXTUS Wiki`, description: communityLevels.description }

export default function CommunityLevelsPage() {
  return <WikiDocument {...communityLevels} />
}
