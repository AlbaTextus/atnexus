"use client"

import { useEffect, useMemo, useRef, useState, type ChangeEvent, type DragEvent, type FormEvent } from "react"
import { Link } from "next-view-transitions"
import { BookOpen, Check, CircleCheck, Clock3, FileText, Loader2, Send, Upload, UserRound } from "lucide-react"
import { ContentFailure, NovelAccessGate, useNovelAccess } from "@/components/novel-access"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { api, ApiError } from "@/lib/api"
import { allowsEndpoint, type RolePolicy } from "@/lib/athub-permissions"
import { novelStatus, safeContentURL, type ContentAuthor, type ContentPlatform, type ContentTag, type Novel } from "@/lib/athub-client"
import { readAllContent } from "@/lib/novel-demo-data"
import { MAX_CHAPTERS, parseEpub, type EpubVolume, type EpubCover } from "@/lib/epub-import"
import { NovelUploadPreview, type UploadChapterDraft } from "@/components/novel-upload-preview"
import { DemoCover } from "@/components/novel-demo/novel-cover"
import "@/styles/novel-submit.css"

type Catalog = { platforms: ContentPlatform[]; authors: ContentAuthor[]; tags: ContentTag[]; policy: RolePolicy }
type SubmissionResult = { status: "pending"; review_id: string } | { novel: { id: number }; chapterCount: number }

export function NovelSubmit() {
  return <div className="novel-demo novel-submit-view"><NovelAccessGate><SubmissionForm /></NovelAccessGate></div>
}

function SubmissionForm() {
  const { request } = useNovelAccess()
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [loadError, setLoadError] = useState("")
  const [revision, setRevision] = useState(0)
  const [title, setTitle] = useState("")
  const [platformId, setPlatformId] = useState("")
  const [authorId, setAuthorId] = useState("")
  const [status, setStatus] = useState<Novel["status"]>("ONGOING")
  const [summary, setSummary] = useState("")
  const [coverUrl, setCoverUrl] = useState("")
  const [tagIds, setTagIds] = useState<number[]>([])
  const [chapters, setChapters] = useState<UploadChapterDraft[]>([])
  const [volumes, setVolumes] = useState<EpubVolume[]>([])
  const [epubCover, setEpubCover] = useState<EpubCover | null>(null)
  const [coverPreview, setCoverPreview] = useState("")
  const nextKey = useRef(1)
  const submitting = useRef(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<SubmissionResult | null>(null)
  const epubInput = useRef<HTMLInputElement>(null)
  const importing = useRef(false)
  const [parsing, setParsing] = useState(false)
  const [importError, setImportError] = useState("")
  const [importedAuthor, setImportedAuthor] = useState("")
  const [fileName, setFileName] = useState("")
  const [editingDetails, setEditingDetails] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [previewRevision, setPreviewRevision] = useState(0)

  useEffect(() => {
    if (!epubCover?.file) { setCoverPreview(""); return }
    const url = URL.createObjectURL(epubCover.file)
    setCoverPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [epubCover])

  useEffect(() => {
    const controller = new AbortController()
    setLoadError("")
    Promise.all([
      readAllContent<ContentPlatform>(request, "/api/v5/platforms", controller.signal),
      readAllContent<ContentAuthor>(request, "/api/v5/authors", controller.signal),
      readAllContent<ContentTag>(request, "/api/v5/tags", controller.signal),
      api<RolePolicy>("/api/v1/account/athub/permissions", { signal: controller.signal, cache: "no-store" }),
    ]).then(([platforms, authors, tags, policy]) => {
      if (!controller.signal.aborted) {
        setCatalog({ platforms, authors, tags, policy })
        if (platforms.length === 1) setPlatformId(current => current || String(platforms[0].id))
      }
    }).catch(reason => {
      if (!controller.signal.aborted) setLoadError(reason instanceof Error ? reason.message : "加载失败。")
    })
    return () => controller.abort()
  }, [request, revision])

  const authorOptions = catalog?.authors.filter(author => author.platformId === Number(platformId)) || []
  const canUpload = Boolean(catalog && allowsEndpoint(catalog.policy, "upload", "POST", "/api/v2/upload"))
  const wordCount = useMemo(() => chapters.reduce((total, chapter) => total + chapter.content.replace(/\s/g, "").length, 0), [chapters])
  const authorName = authorOptions.find(author => author.id === Number(authorId))?.name || (authorId === "none" ? "" : importedAuthor) || "作者待补充"

  function updateChapter(key: number, field: "title" | "content", value: string) {
    setChapters(current => current.map(chapter => chapter.key === key ? { ...chapter, [field]: value } : chapter))
  }

  async function importEpub(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (file) await importFile(file)
  }

  async function importFile(file: File) {
    if (importing.current || submitting.current) return
    importing.current = true
    setParsing(true)
    setImportError("")
    try {
      const book = await parseEpub(file)
      if (book.chapters.length > MAX_CHAPTERS) throw new Error("EPUB 超过 1000 章，请拆分文件后导入。")
      const imported = book.chapters.map(chapter => ({ ...chapter, key: nextKey.current++ }))
      const matchingAuthors = book.author.trim() ? catalog?.authors.filter(author => author.name?.trim() === book.author.trim()) || [] : []
      const inferredPlatform = catalog?.platforms.find(platform =>
        platform.name === book.publisher || platform.name === book.source || platform.url && book.source.startsWith(platform.url))
      const preferredPlatform = inferredPlatform ? String(inferredPlatform.id)
        : matchingAuthors.length === 1 ? String(matchingAuthors[0].platformId)
          : platformId || (catalog?.platforms.length === 1 ? String(catalog.platforms[0].id) : "")
      const author = matchingAuthors.find(author => author.platformId === Number(preferredPlatform))
      setChapters(imported)
      setVolumes(book.volumes)
      setEpubCover(book.cover)
      setTitle(book.title)
      setSummary(book.summary)
      setImportedAuthor(book.author)
      setPlatformId(preferredPlatform)
      setAuthorId(author ? String(author.id) : "")
      setCoverUrl(book.cover?.url || "")
      setStatus("ONGOING")
      setTagIds([])
      setFileName(file.name)
      setEditingDetails(false)
      setPreviewRevision(value => value + 1)
      setError("")
    } catch (reason) {
      setImportError(reason instanceof Error ? reason.message : "EPUB 导入失败，请检查文件后重试。")
    } finally {
      importing.current = false
      setParsing(false)
    }
  }

  function dropFile(event: DragEvent<HTMLElement>) {
    event.preventDefault()
    setDragging(false)
    if (event.dataTransfer.files.length !== 1) { setImportError("请一次选择一本 EPUB。"); return }
    void importFile(event.dataTransfer.files[0])
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitting.current || importing.current || !catalog || !canUpload || result) return
    setError("")
    if (!fileName || !chapters.length) { setError("请先导入 EPUB。"); return }
    if (!title.trim()) { setError("请补全小说名称。"); setEditingDetails(true); return }
    if (!catalog.platforms.some(platform => platform.id === Number(platformId))) { setError("请选择来源平台。"); return }
    if (authorId && authorId !== "none" && !authorOptions.some(author => author.id === Number(authorId))) { setError("请选择该平台下的作者。"); return }
    if (!authorId && Array.from(importedAuthor.trim()).length > 200) { setError("作者名称不能超过 200 个字符。"); setEditingDetails(true); return }
    if (coverUrl.trim() && !safeContentURL(coverUrl.trim())) { setError("封面地址需要使用有效的 HTTP 或 HTTPS 链接。"); return }
    const invalidChapter = chapters.findIndex(chapter => !chapter.title.trim() || !chapter.content.trim())
    if (invalidChapter >= 0) { setError(`第 ${invalidChapter + 1} 章缺少标题或正文，请在目录预览中编辑或移除。`); return }
    if ([title, summary, coverUrl, !authorId ? importedAuthor : "", ...chapters.flatMap(chapter => [chapter.title, chapter.content])].some(value => value.includes("\0"))) {
      setError("提交的内容中含有无效字符，请移除后重试。"); return
    }
    const payload = {
      authorName: !authorId ? importedAuthor.trim() || null : null,
      novel: { title: title.trim(), platformId: Number(platformId), authorId: authorId && authorId !== "none" ? Number(authorId) : null,
        status, summary: summary.trim() || null, coverUrl: coverUrl.trim() || null, tagIds },
      volumes,
      chapters: chapters.map((chapter, index) => ({ order: index + 1, title: chapter.title.trim(), content: chapter.content, volumeOrder: chapter.volumeOrder })),
    }
    let body = JSON.stringify(payload)
    if (new TextEncoder().encode(body).length > 8 * 1024 * 1024) { setError("本次提交超过 8 MiB，请减少章节后重试。"); return }
    submitting.current = true
    setBusy(true)
    let uploadingCover = Boolean(epubCover?.file && !coverUrl.trim())
    try {
      if (epubCover?.file && !coverUrl.trim()) {
        const form = new FormData()
        form.append("file", epubCover.file)
        const uploaded = await api<{ url: string }>("/api/v4/account/athub/cover", { method: "POST", body: form })
        const url = safeContentURL(uploaded?.url)
        if (!url) throw new ApiError(502, "cover_upload_failed", "封面上传未返回有效链接，请重试。")
        setCoverUrl(url)
        payload.novel.coverUrl = url
        body = JSON.stringify(payload)
        if (new TextEncoder().encode(body).length > 8 * 1024 * 1024) throw new ApiError(413, "payload_too_large", "本次提交超过 8 MiB。")
      }
      uploadingCover = false
      const response = await api<SubmissionResult>("/api/v4/account/athub/submissions", { method: "POST", body })
      if (!response || !("novel" in response && response.novel?.id > 0) &&
        !("status" in response && response.status === "pending" && response.review_id)) throw new Error("提交服务返回的数据格式不正确。")
      setResult(response)
    } catch (reason) {
      setError(reason instanceof ApiError
        ? reason.code.startsWith("cover_") ? reason.message
          : reason.status === 403 ? "当前账户没有提交权限，或权限已发生变化。"
          : reason.status === 401 ? "登录已失效，请重新登录后再提交。"
            : reason.status === 400 ? "提交资料未通过校验，请检查小说资料和章节内容。"
              : reason.status === 409 ? "作品或章节已存在，请检查后再提交。"
                : reason.status === 413 ? "提交内容过大，请减少章节后重试。"
                  : reason.status >= 500 ? "提交服务暂时不可用，请稍后重试。" : reason.message
        : uploadingCover ? "无法连接封面上传服务。书籍预览已保留，请重试。"
          : "未能确认提交结果，请先在书库或审核记录中确认是否已提交，避免重复提交。")
    } finally {
      submitting.current = false
      setBusy(false)
    }
  }

  if (loadError) return <div className="page-shell novel-submit-page"><ContentFailure message={loadError} retry={() => setRevision(value => value + 1)} /></div>
  if (!catalog) return <div className="page-shell novel-page-loading" role="status">正在加载提交表单…</div>

  if (result) {
    const pending = "status" in result
    return <div className="page-shell novel-submit-page novel-content-ready">
      <Card className="novel-submit-success"><CardContent>
        {pending ? <Clock3 aria-hidden="true" /> : <CircleCheck aria-hidden="true" />}
        <h1>{pending ? "已提交审核" : "小说已入库"}</h1>
        <p>{pending ? `《${title.trim()}》审核通过后将在书库中展示。` : `《${title.trim()}》已成功提交${chapters.length ? `，共 ${chapters.length} 章` : ""}。`}</p>
        <div>
          {!pending && "novel" in result && <Button nativeButton={false} render={<Link href={`/library/books/${result.novel.id}`} />}>查看作品</Button>}
          <Button nativeButton={false} variant="outline" render={<Link href="/library" />}>返回书库</Button>
          <Button variant="ghost" onClick={() => {
            setResult(null); setTitle(""); setAuthorId(""); setSummary(""); setCoverUrl(""); setTagIds([]); setChapters([]); setVolumes([]); setEpubCover(null); setStatus("ONGOING"); setError(""); setImportError(""); setImportedAuthor(""); setFileName(""); setEditingDetails(false)
          }}>继续提交</Button>
        </div>
      </CardContent></Card>
    </div>
  }

  return <div className="page-shell novel-submit-page novel-content-ready">
    <header className="library-heading"><h1>提交小说</h1><Link href="/library">返回书库</Link></header>
    {!canUpload ? <Card className="novel-submit-notice"><CardContent><p role="status">当前账户没有提交小说的权限。</p></CardContent></Card>
      : !catalog.platforms.length ? <Card className="novel-submit-notice"><CardContent><p role="status">暂无可选的来源平台，请联系管理员添加后再提交。</p></CardContent></Card>
        : <form onSubmit={submit} className="novel-upload-form" aria-busy={parsing || busy}>
          <input ref={epubInput} type="file" accept=".epub,application/epub+zip" aria-label="选择 EPUB 文件" hidden disabled={busy || parsing} onChange={importEpub} />
          {!fileName ? <section className={`panel novel-upload-dropzone${dragging ? " is-dragging" : ""}`} aria-label="导入 EPUB"
            onDragOver={event => { event.preventDefault(); if (!busy && !parsing) setDragging(true) }}
            onDragLeave={event => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false) }} onDrop={dropFile}>
            <div className="novel-upload-drop-icon">{parsing ? <Loader2 className="animate-spin" aria-hidden="true" /> : <BookOpen aria-hidden="true" />}</div>
            <h2>{parsing ? "正在解析 EPUB…" : "上传 EPUB"}</h2>
            <p>拖入书籍文件，预览作品与章节</p>
            <Button type="button" disabled={busy || parsing} onClick={() => epubInput.current?.click()}><Upload aria-hidden="true" />选择 EPUB 文件</Button>
            <span className="novel-upload-file-limit">EPUB · 最大 32 MiB</span>
            {importError && <p className="novel-submit-error" role="alert">{importError}</p>}
          </section> : <>
            <div className="novel-upload-file-bar">
              <div role="status"><FileText aria-hidden="true" /><span>{parsing ? "正在解析 EPUB…" : fileName}</span><span className="novel-upload-file-count">{chapters.length} 章</span>{volumes.length > 0 && <span className="novel-upload-file-count">{volumes.length} 卷</span>}</div>
              <Button type="button" size="sm" variant="outline" disabled={busy || parsing} onClick={() => epubInput.current?.click()}>
                {parsing ? <Loader2 className="animate-spin" aria-hidden="true" /> : <Upload aria-hidden="true" />}{parsing ? "正在拆章…" : "更换 EPUB"}
              </Button>
            </div>
            {importError && <p className="novel-submit-error novel-upload-import-error" role="alert">{importError}</p>}
            <div className="novel-upload-preview-layout">
              <aside className="novel-upload-sidebar" aria-label="书籍信息">
                <section className="panel novel-upload-book-card">
                  <div className="novel-upload-cover"><DemoCover src={safeContentURL(coverUrl) || coverPreview || null} title={title} /></div>
                  <h2>{title || "未命名作品"}</h2>
                  <p className="novel-upload-author"><UserRound aria-hidden="true" />{authorName}</p>
                  {tagIds.length > 0 && <div className="novel-upload-selected-tags">{catalog.tags.filter(tag => tagIds.includes(tag.id)).map(tag => <span key={tag.id}>{tag.name}</span>)}</div>}
                  <dl className="novel-upload-facts">{volumes.length > 0 && <div><dt>分卷</dt><dd>{volumes.length} 卷</dd></div>}<div><dt>章节</dt><dd>{chapters.length} 章</dd></div><div><dt>字数</dt><dd>{wordCount.toLocaleString("zh-CN")} 字</dd></div><div><dt>状态</dt><dd>{novelStatus(status)}</dd></div></dl>
                </section>
                <section className="panel novel-upload-submit-card" aria-label="提交设置">
                  <label className="novel-submit-field" htmlFor="novel-platform">来源平台
                    <select id="novel-platform" aria-label="来源平台" value={platformId} disabled={busy || parsing} onChange={event => {
                      setPlatformId(event.target.value)
                      const author = catalog.authors.find(author => author.platformId === Number(event.target.value) && author.name?.trim() === importedAuthor.trim())
                      setAuthorId(author ? String(author.id) : "")
                    }}>
                      <option value="" disabled>选择平台</option>{catalog.platforms.map(platform => <option key={platform.id} value={platform.id}>{platform.name}</option>)}
                    </select>
                  </label>
                  {catalog.policy.review_required && <p className="novel-submit-review"><Clock3 aria-hidden="true" />提交后需审核通过才会入库。</p>}
                  {error && <p className="novel-submit-error" role="alert">{error}</p>}
                  <Button type="submit" disabled={busy || parsing || !chapters.length} className="novel-submit-action">{busy ? <Loader2 className="animate-spin" aria-hidden="true" /> : <Send aria-hidden="true" />}{busy ? "正在提交…" : catalog.policy.review_required ? "提交审核" : "提交小说"}</Button>
                </section>
              </aside>
              <NovelUploadPreview key={previewRevision} title={title} summary={summary} chapters={chapters} volumes={volumes} disabled={busy || parsing}
                editingDetails={editingDetails} onEditingDetails={setEditingDetails} onChapterChange={updateChapter}
                onChapterRemove={key => setChapters(current => current.filter(chapter => chapter.key !== key))}
                details={<fieldset disabled={busy || parsing} className="novel-submit-details">
                  <label className="novel-submit-field novel-submit-wide" htmlFor="novel-title">小说名称<Input id="novel-title" value={title} onChange={event => setTitle(event.target.value)} /></label>
                  <label className="novel-submit-field" htmlFor="novel-author">作者
                    <select id="novel-author" aria-label="作者" value={authorId} disabled={!platformId || busy || parsing} onChange={event => setAuthorId(event.target.value)}>
                      <option value="">{importedAuthor ? `使用 EPUB 作者：${importedAuthor}` : "不关联作者"}</option>
                      {importedAuthor && <option value="none">不关联作者</option>}
                      {authorOptions.map(author => <option key={author.id} value={author.id}>{author.name || `作者 #${author.id}`}</option>)}
                    </select>
                  </label>
                  <label className="novel-submit-field" htmlFor="novel-status">连载状态
                    <select id="novel-status" aria-label="连载状态" value={status} onChange={event => setStatus(event.target.value as typeof status)}><option value="ONGOING">连载中</option><option value="COMPLETED">已完结</option><option value="ABANDONED">太监</option></select>
                  </label>
                  <div className="novel-submit-wide novel-upload-cover-field">
                    <label className="novel-submit-field" htmlFor="novel-cover">封面链接<Input id="novel-cover" type="url" value={coverUrl} placeholder={epubCover?.file ? "提交时自动生成封面链接" : ""} onChange={event => setCoverUrl(event.target.value)} /></label>
                    {epubCover?.file && <p>已提取：{epubCover.path}</p>}
                    {(epubCover || coverUrl) && <Button type="button" variant="ghost" size="sm" onClick={() => { setEpubCover(null); setCoverUrl("") }}>移除封面</Button>}
                  </div>
                  <label className="novel-submit-field novel-submit-wide" htmlFor="novel-summary">简介<Textarea id="novel-summary" rows={5} value={summary} onChange={event => setSummary(event.target.value)} /></label>
                  {catalog.tags.length > 0 && <fieldset className="novel-submit-tags novel-submit-wide"><legend>标签</legend><div>{catalog.tags.map(tag => <button key={tag.id} type="button" aria-pressed={tagIds.includes(tag.id)} onClick={() => setTagIds(current => current.includes(tag.id) ? current.filter(id => id !== tag.id) : [...current, tag.id])}>
                    {tagIds.includes(tag.id) && <Check aria-hidden="true" />}{tag.name}
                  </button>)}</div></fieldset>}
                </fieldset>} />
            </div>
          </>}
        </form>}
  </div>
}
