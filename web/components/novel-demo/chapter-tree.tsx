import { ChevronRight, FileText, Folder } from "lucide-react"
import type { TableOfContents } from "@/lib/athub-client"

type Props = {
  volumes: TableOfContents["volumes"]
  chapters: TableOfContents["chapters"]
}

function ChapterTitles({ chapters }: { chapters: TableOfContents["chapters"] }) {
  return chapters.length ? <ul className="novel-chapter-branches">
    {chapters.map(chapter => <li className="novel-chapter-leaf" key={chapter.id}>
      <FileText aria-hidden="true" />
      <span>{chapter.title}</span>
    </li>)}
  </ul> : <p className="novel-chapter-empty">暂无章节</p>
}

export function ChapterTree({ volumes, chapters }: Props) {
  const orderedVolumes = [...volumes].sort((a, b) => a.order - b.order || a.id - b.id)
  const orderedChapters = [...chapters].sort((a, b) => a.order - b.order || a.id - b.id)
  const groups = new Map(orderedVolumes.map(volume => [volume.id, [] as TableOfContents["chapters"]]))
  const ungrouped: TableOfContents["chapters"] = []
  for (const chapter of orderedChapters) {
    const group = chapter.volumeId === null ? undefined : groups.get(chapter.volumeId)
    if (group) group.push(chapter)
    else ungrouped.push(chapter)
  }

  return <section className="panel novel-chapter-catalog" aria-labelledby="novel-catalog-heading">
    <h2 id="novel-catalog-heading" className="content-section-heading m-0 text-lg font-bold tracking-tight text-foreground sm:text-xl">章节目录</h2>
    <div className="novel-chapter-tree" role="region" aria-labelledby="novel-catalog-heading" tabIndex={0}>
        {orderedVolumes.length ? <ul className="novel-chapter-root">
          {orderedVolumes.map(volume => <li key={volume.id}>
            <details className="novel-chapter-volume">
              <summary><ChevronRight className="novel-tree-chevron" aria-hidden="true" /><Folder aria-hidden="true" /><span>{volume.title}</span></summary>
              <ChapterTitles chapters={groups.get(volume.id)!} />
            </details>
          </li>)}
          {ungrouped.length > 0 && <li>
            <details className="novel-chapter-volume">
              <summary><ChevronRight className="novel-tree-chevron" aria-hidden="true" /><Folder aria-hidden="true" /><span>未分卷</span></summary>
              <ChapterTitles chapters={ungrouped} />
            </details>
          </li>}
        </ul> : <ChapterTitles chapters={ungrouped} />}
    </div>
  </section>
}
