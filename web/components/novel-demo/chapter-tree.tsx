"use client"

import { useId, useRef, useState } from "react"
import { ChevronRight, FileText, Folder } from "lucide-react"
import type { TableOfContents } from "@/lib/athub-client"
import { ContentSectionHeading } from "./content-section-heading"
import { NovelAutoHeight } from "@/components/novel-motion"

type Props = {
  volumes: TableOfContents["volumes"]
  chapters: TableOfContents["chapters"]
}

function ChapterTitles({ chapters }: { chapters: TableOfContents["chapters"] }) {
  return chapters.length ? <ul className="novel-chapter-branches">
    {chapters.map(chapter => <li className="novel-chapter-leaf" key={chapter.id}>
      <FileText aria-hidden="true" />
      <span title={chapter.title}>{chapter.title}</span>
    </li>)}
  </ul> : <p className="novel-chapter-empty">暂无章节</p>
}

export function ChapterTree({ volumes, chapters }: Props) {
  const [selectedVolume, setSelectedVolume] = useState<number | "ungrouped" | null>(null)
  const catalogId = useId()
  const orderedVolumes = [...volumes].sort((a, b) => a.order - b.order || a.id - b.id)
  const orderedChapters = [...chapters].sort((a, b) => a.order - b.order || a.id - b.id)
  const groups = new Map(orderedVolumes.map(volume => [volume.id, [] as TableOfContents["chapters"]]))
  const ungrouped: TableOfContents["chapters"] = []
  for (const chapter of orderedChapters) {
    const group = chapter.volumeId === null ? undefined : groups.get(chapter.volumeId)
    if (group) group.push(chapter)
    else ungrouped.push(chapter)
  }
  const volumeGroups: { key: number | "ungrouped"; title: string; chapters: Props["chapters"] }[] =
    orderedVolumes.map(volume => ({ key: volume.id, title: volume.title, chapters: groups.get(volume.id)! }))
  if (orderedVolumes.length && ungrouped.length) {
    volumeGroups.push({ key: "ungrouped", title: "未分卷", chapters: ungrouped })
  }
  const active = volumeGroups.find(volume => volume.key === selectedVolume)
  const previous = useRef<typeof active>(undefined)
  if (active) previous.current = active
  const shown = active || previous.current
  if (!volumes.length && !chapters.length) return null

  return <section className="panel novel-chapter-catalog" aria-labelledby="novel-catalog-heading">
    <ContentSectionHeading id="novel-catalog-heading">目录</ContentSectionHeading>
    <div className={`novel-chapter-tree${volumeGroups.length ? " has-volumes" : ""}`}>
      {volumeGroups.length ? <>
        <div className="novel-chapter-volumes" role="group" aria-label="作品分卷" tabIndex={0}>
          <ul className="novel-chapter-root">
            {volumeGroups.map(volume => <li key={volume.key}>
              <button type="button" className="novel-chapter-volume" id={`${catalogId}-volume-${volume.key}`}
                aria-expanded={active?.key === volume.key} aria-controls={`${catalogId}-chapters`}
                onClick={() => setSelectedVolume(active?.key === volume.key ? null : volume.key)}>
                <ChevronRight className="novel-tree-chevron" aria-hidden="true" /><Folder aria-hidden="true" /><span title={volume.title}>{volume.title}</span>
              </button>
            </li>)}
          </ul>
        </div>
        <div id={`${catalogId}-chapters`} className="novel-chapter-content" role="region" data-open={Boolean(active)}
          aria-labelledby={active ? `${catalogId}-volume-${active.key}` : undefined} aria-hidden={!active} inert={!active || undefined} tabIndex={active ? 0 : -1}>
          <NovelAutoHeight open={Boolean(active)} clampToParent>
            {shown && <div key={shown.key} className="novel-motion-enter"><ChapterTitles chapters={shown.chapters} /></div>}
          </NovelAutoHeight>
        </div>
      </> : <div className="novel-chapter-content" role="region" aria-label="章节" tabIndex={0}><ChapterTitles chapters={ungrouped} /></div>}
    </div>
  </section>
}
