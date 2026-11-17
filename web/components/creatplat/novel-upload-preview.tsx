"use client"

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { BookOpen, ChevronLeft, ChevronRight, FileText, Folder, Library, Pencil, Search, Trash2, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/novel/ui/tabs"
import type { EpubChapter, EpubVolume } from "@/lib/creatplat/epub-import"
import { NovelAutoHeight } from "@/components/novel/novel-motion"

export type UploadChapterDraft = EpubChapter & { key: number }

type Props = {
  title: string
  summary: string
  chapters: UploadChapterDraft[]
  volumes: EpubVolume[]
  disabled: boolean
  editingDetails: boolean
  onEditingDetails: (value: boolean) => void
  details: ReactNode
  onChapterChange: (key: number, field: "title" | "content", value: string) => void
  onChapterRemove: (key: number) => void
}

function readingText(chapter: UploadChapterDraft) {
  const lines = chapter.content.split("\n")
  const normalize = (text: string) => text.replace(/\s/g, "")
  // EPUB headings may span a chapter number and a subtitle on separate lines.
  for (let count = 1; count <= Math.min(4, lines.length); count++) {
    if (normalize(lines.slice(0, count).join("")) === normalize(chapter.title)) {
      return lines.slice(count).join("\n").trim() || chapter.content
    }
  }
  return chapter.content
}

export function NovelUploadPreview({ title, summary, chapters, volumes, disabled, editingDetails, onEditingDetails, details, onChapterChange, onChapterRemove }: Props) {
  const [tab, setTab] = useState("intro")
  const [selectedKey, setSelectedKey] = useState<number | null>(null)
  const [query, setQuery] = useState("")
  const [editingChapter, setEditingChapter] = useState(false)
  const reader = useRef<HTMLDivElement>(null)
  const index = chapters.findIndex(chapter => chapter.key === selectedKey)
  const selected = index < 0 ? null : chapters[index]
  const filtered = useMemo(() => {
    const search = query.trim().toLocaleLowerCase()
    const matchingVolumes = new Set(volumes.filter(volume => volume.title.toLocaleLowerCase().includes(search)).map(volume => volume.order))
    return chapters.map((chapter, index) => ({ chapter, index })).filter(({ chapter, index }) =>
      !search || chapter.title.toLocaleLowerCase().includes(search) || String(index + 1) === search || chapter.volumeOrder !== null && matchingVolumes.has(chapter.volumeOrder))
  }, [chapters, volumes, query])
  const content = useMemo(() => selected ? readingText(selected) : "", [selected])

  useEffect(() => { reader.current?.scrollTo({ top: 0 }) }, [selectedKey])

  function select(key: number | null) {
    setSelectedKey(key)
    setEditingChapter(false)
  }
  const chapterList = (items: typeof filtered) => <ol>{items.map(({ chapter, index }) => <li key={chapter.key}>
    <button type="button" disabled={disabled} aria-pressed={chapter.key === selectedKey} aria-controls="upload-chapter-reader" onClick={() => select(chapter.key === selectedKey ? null : chapter.key)}>
      <span className="novel-upload-chapter-number">{String(index + 1).padStart(2, "0")}</span><span>{chapter.title || "未命名章节"}</span><ChevronRight aria-hidden="true" />
    </button>
  </li>)}</ol>
  const ungrouped = filtered.filter(({ chapter }) => chapter.volumeOrder === null || !volumes.some(volume => volume.order === chapter.volumeOrder))

  return <div className="novel-upload-main">
    <div className="novel-upload-preview-heading">
      <h2>作品预览</h2>
      <Button type="button" variant="outline" size="sm" disabled={disabled} aria-expanded={editingDetails} onClick={() => onEditingDetails(!editingDetails)}>
        {editingDetails ? "完成修改" : <><Pencil aria-hidden="true" />修改资料</>}
      </Button>
    </div>
    <NovelAutoHeight open={editingDetails}>
      <section className="panel novel-upload-edit-details novel-motion-enter" aria-label="修改小说资料">{details}</section>
    </NovelAutoHeight>
    <Tabs value={tab} onValueChange={setTab}>
      <TabsList className="novel-upload-tabs" aria-label="上传作品预览">
        <TabsTrigger value="intro"><FileText aria-hidden="true" />作品简介</TabsTrigger>
        <TabsTrigger value="catalog"><Library aria-hidden="true" />章节目录<span>{chapters.length}</span></TabsTrigger>
      </TabsList>
      <NovelAutoHeight>
      <TabsContent value="intro" className="novel-motion-enter">
        <section className="panel novel-upload-intro" aria-label="作品简介">
          <h2>简介</h2>
          <div className="synopsis">{summary.trim() ? summary.split(/\r?\n/).filter(line => line.trim()).map((line, index) => <p key={index}>{line}</p>) : <p>暂无简介</p>}</div>
          <Button type="button" variant="outline" disabled={disabled || !chapters.length} onClick={() => { setTab("catalog"); select(chapters[0]?.key ?? null) }}><BookOpen aria-hidden="true" />预览首章</Button>
        </section>
      </TabsContent>
      <TabsContent value="catalog" className="novel-motion-enter">
        <section className="panel novel-upload-catalog" aria-label="目录预览">
          <div className="novel-upload-catalog-heading">
            <h2>目录预览</h2>
            <div className="novel-upload-search"><Search aria-hidden="true" /><Input type="search" aria-label="搜索章节" placeholder="搜索章节" value={query} onChange={event => setQuery(event.target.value)} /></div>
          </div>
          <div className={`novel-upload-catalog-body${selected ? " has-selection" : ""}`}>
            <nav className="novel-upload-chapter-list" aria-label="预览章节目录">
              {filtered.length || volumes.length && !query.trim() ? <>
                {volumes.map(volume => {
                  const items = filtered.filter(({ chapter }) => chapter.volumeOrder === volume.order)
                  if (!items.length && query.trim()) return null
                  return <details key={volume.order} className="novel-upload-volume" open={query.trim() || selected?.volumeOrder === volume.order ? true : undefined}>
                    <summary><ChevronRight aria-hidden="true" /><Folder aria-hidden="true" /><span>{volume.title}</span><span>{items.length}</span></summary>
                    {items.length ? chapterList(items) : <p className="novel-upload-empty">暂无章节</p>}
                  </details>
                })}
                {ungrouped.length > 0 && (volumes.length ? <details className="novel-upload-volume" open={query.trim() || selected?.volumeOrder === null ? true : undefined}><summary><ChevronRight aria-hidden="true" /><Folder aria-hidden="true" /><span>未分卷</span><span>{ungrouped.length}</span></summary>{chapterList(ungrouped)}</details> : chapterList(ungrouped))}
              </> : <p className="novel-upload-empty">没有匹配的章节</p>}
            </nav>
            {selected && <section id="upload-chapter-reader" className="novel-upload-reader novel-motion-enter" aria-label="章节正文预览">
              <div className="novel-upload-reader-toolbar">
                <span>{volumes.find(volume => volume.order === selected.volumeOrder)?.title && `${volumes.find(volume => volume.order === selected.volumeOrder)!.title} · `}第 {index + 1} / {chapters.length} 章</span>
                <div>
                  <Button type="button" variant="ghost" size="icon" aria-label={editingChapter ? "结束编辑章节" : "编辑当前章节"} disabled={disabled} onClick={() => setEditingChapter(value => !value)}><Pencil aria-hidden="true" /></Button>
                  <Button type="button" variant="ghost" size="icon" aria-label="收起章节正文" onClick={() => select(null)}><X aria-hidden="true" /></Button>
                </div>
              </div>
              <div ref={reader} className="novel-upload-reading-scroll">
                <div key={`${selected.key}:${editingChapter}`} className="novel-motion-enter">
                {editingChapter ? <fieldset disabled={disabled} className="novel-upload-chapter-editor">
                  <label className="novel-submit-field" htmlFor={`chapter-title-${selected.key}`}>章节标题<Input id={`chapter-title-${selected.key}`} value={selected.title} onChange={event => onChapterChange(selected.key, "title", event.target.value)} /></label>
                  <label className="novel-submit-field" htmlFor={`chapter-content-${selected.key}`}>正文<Textarea id={`chapter-content-${selected.key}`} value={selected.content} onChange={event => onChapterChange(selected.key, "content", event.target.value)} /></label>
                  <Button type="button" variant="outline" onClick={() => { onChapterRemove(selected.key); select(null) }}><Trash2 aria-hidden="true" />移除当前章节</Button>
                </fieldset> : <article className="novel-upload-reading-text" aria-label={selected.title || title}>
                  <h3>{selected.title || "未命名章节"}</h3>
                  <div>{content}</div>
                </article>}
                </div>
              </div>
              <div className="novel-upload-reader-pagination">
                <Button type="button" variant="ghost" size="sm" disabled={disabled || index <= 0} onClick={() => select(chapters[index - 1].key)}><ChevronLeft aria-hidden="true" />上一章</Button>
                <Button type="button" variant="ghost" size="sm" disabled={disabled || index >= chapters.length - 1} onClick={() => select(chapters[index + 1].key)}>下一章<ChevronRight aria-hidden="true" /></Button>
              </div>
            </section>}
          </div>
        </section>
      </TabsContent>
      </NovelAutoHeight>
    </Tabs>
  </div>
}
