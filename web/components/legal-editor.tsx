"use client"

import { useDeferredValue, useEffect, useState, type FormEvent } from "react"
import { AlertCircle, Check, Code2, Copy, ExternalLink, Eye, FileText, PanelsTopLeft, RefreshCw, Save, Shield } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { legalNames, type LegalDocument, type LegalSlug } from "@/lib/legal"
import { cn } from "@/lib/utils"
import { MarkdownContent } from "@/components/markdown-content"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

type Documents = Partial<Record<LegalSlug, LegalDocument>>
const isDirty = (draft?: LegalDocument, saved?: LegalDocument) => Boolean(draft && saved && (draft.title !== saved.title || draft.description !== saved.description || draft.content !== saved.content))

export function LegalEditor() {
  const [slug, setSlug] = useState<LegalSlug>("privacy")
  const [documents, setDocuments] = useState<Documents>({})
  const [drafts, setDrafts] = useState<Documents>({})
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const [conflict, setConflict] = useState(false)
  const [retry, setRetry] = useState(0)
  const [view, setView] = useState("split")
  const draft = drafts[slug]
  const dirty = isDirty(draft, documents[slug])
  const unsaved = (Object.keys(drafts) as LegalSlug[]).some(key => isDirty(drafts[key], documents[key]))
  const markdown = useDeferredValue(draft?.content || "")

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError("")
    Promise.all((["privacy", "terms"] as LegalSlug[]).map(key => api<LegalDocument>(`/api/v1/admin/legal/${key}`, { signal: controller.signal, cache: "no-store" })))
      .then(values => {
        if (controller.signal.aborted) return
        const loaded = Object.fromEntries(values.map(value => [value.slug, value]))
        setDocuments(loaded); setDrafts(loaded)
      })
      .catch(reason => { if (!controller.signal.aborted) setError(managementError(reason)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [retry])

  useEffect(() => {
    if (!unsaved) return
    const preventLoss = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = "" }
    window.addEventListener("beforeunload", preventLoss)
    return () => window.removeEventListener("beforeunload", preventLoss)
  }, [unsaved])

  function change(field: "title" | "description" | "content", value: string) {
    if (!draft) return
    setDrafts(current => ({ ...current, [slug]: { ...draft, [field]: value } }))
    setNotice("")
    if (!conflict) setError("")
  }

  async function save(event: FormEvent) {
    event.preventDefault()
    if (!draft || busy || !dirty) return
    setBusy(true); setError(""); setNotice(""); setConflict(false)
    try {
      const updated = await api<LegalDocument>(`/api/v1/admin/legal/${slug}`, { method: "PUT", body: JSON.stringify({ title: draft.title, description: draft.description, content: draft.content, revision: draft.revision }) })
      setDocuments(current => ({ ...current, [slug]: updated })); setDrafts(current => ({ ...current, [slug]: updated }))
      setNotice(`${legalNames[slug]}已保存，公开页面已同步。`)
    } catch (reason) {
      setConflict(reason instanceof ApiError && reason.code === "legal_conflict")
      setError(managementError(reason))
    } finally { setBusy(false) }
  }

  async function reloadCurrent() {
    setBusy(true); setNotice("")
    try {
      const latest = await api<LegalDocument>(`/api/v1/admin/legal/${slug}`, { cache: "no-store" })
      setDocuments(current => ({ ...current, [slug]: latest })); setDrafts(current => ({ ...current, [slug]: latest }))
      setError(""); setConflict(false); setNotice("已加载最新版本。")
    } catch (reason) { setError(managementError(reason)) } finally { setBusy(false) }
  }

  async function copyDraft() {
    try { await navigator.clipboard.writeText(draft?.content || ""); setNotice("Markdown 草稿已复制。") }
    catch { setError("复制失败，请在编辑框中手动选择并复制正文。") }
  }

  return <div className="flex min-w-0 flex-col gap-6">
    <div className="center-page-heading"><div><p className="center-eyebrow">LEGAL DOCUMENTS</p><h1>协议管理</h1><p>编写隐私政策与用户协议，保存后同步到公开页面。</p></div><Badge variant="secondary">Markdown</Badge></div>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <ToggleGroup aria-label="选择协议" variant="outline" value={[slug]} disabled={busy} onValueChange={values => { if (values.length) { setSlug(values[0] as LegalSlug); setError(""); setNotice(""); setConflict(false) } }}>
        <ToggleGroupItem value="privacy" aria-label="编辑隐私政策"><Shield />隐私政策{isDirty(drafts.privacy, documents.privacy) && <span aria-hidden="true"> ·</span>}</ToggleGroupItem>
        <ToggleGroupItem value="terms" aria-label="编辑用户协议"><FileText />用户协议{isDirty(drafts.terms, documents.terms) && <span aria-hidden="true"> ·</span>}</ToggleGroupItem>
      </ToggleGroup>
      <Button variant="outline" size="sm" nativeButton={false} role="link" render={<a href={`/${slug}`} target="_blank" rel="noopener noreferrer" />}>查看公开页面<ExternalLink data-icon="inline-end" /></Button>
    </div>
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}{conflict ? <div className="flex flex-wrap gap-2"><Button variant="link" disabled={busy} onClick={copyDraft}><Copy data-icon="inline-start" />复制草稿</Button><Button variant="link" disabled={busy} onClick={reloadCurrent}><RefreshCw data-icon="inline-start" />用最新版本替换草稿</Button></div> : !draft && <Button variant="link" onClick={() => setRetry(value => value + 1)}>重新加载</Button>}</AlertDescription></Alert>}
    {notice && <Alert><Check /><AlertDescription role="status">{notice}</AlertDescription></Alert>}
    {loading ? <div className="flex flex-col gap-4" role="status" aria-label="正在加载协议"><Skeleton className="h-44 w-full" /><Skeleton className="h-96 w-full" /></div> : draft && <form onSubmit={save} className="flex min-w-0 flex-col gap-5" aria-busy={busy}>
      <Card>
        <CardHeader><CardTitle>{legalNames[slug]}</CardTitle><CardDescription>标题和简介会展示在协议正文上方。</CardDescription></CardHeader>
        <CardContent><FieldGroup><Field data-disabled={busy}><FieldLabel htmlFor="legal-document-title">标题</FieldLabel><Input id="legal-document-title" value={draft.title} onChange={event => change("title", event.target.value)} maxLength={100} required disabled={busy} /></Field><Field data-disabled={busy}><FieldLabel htmlFor="legal-document-description">简介</FieldLabel><Input id="legal-document-description" value={draft.description} onChange={event => change("description", event.target.value)} maxLength={300} disabled={busy} /></Field></FieldGroup></CardContent>
        <CardFooter className="flex-wrap justify-between gap-3"><div className="flex items-center gap-2"><Badge variant={dirty ? "secondary" : "success"}>{dirty ? "未保存" : "已同步"}</Badge><span className="text-xs text-muted-foreground">版本 {documents[slug]?.revision} · {new Date(documents[slug]?.updated_at || draft.updated_at).toLocaleString("zh-CN")}</span></div><ToggleGroup aria-label="编辑视图" size="sm" variant="outline" value={[view]} onValueChange={values => { if (values.length) setView(values[0]) }}><ToggleGroupItem value="split" aria-label="分栏"><PanelsTopLeft />分栏</ToggleGroupItem><ToggleGroupItem value="edit" aria-label="编辑"><Code2 />编辑</ToggleGroupItem><ToggleGroupItem value="preview" aria-label="预览"><Eye />预览</ToggleGroupItem></ToggleGroup></CardFooter>
      </Card>
      <div className={cn("grid min-w-0 items-start gap-5", view === "split" && "xl:grid-cols-2")}>
        {view !== "preview" && <Card className="min-w-0"><CardHeader><CardTitle>Markdown 编辑</CardTitle><CardDescription>支持标题、列表、链接、表格和代码块。</CardDescription></CardHeader><CardContent><FieldGroup><Field data-disabled={busy}><FieldLabel htmlFor="legal-markdown" className="sr-only">Markdown 正文</FieldLabel><Textarea id="legal-markdown" value={draft.content} onChange={event => change("content", event.target.value)} className="h-[28rem] resize-y field-sizing-fixed" maxLength={100000} required disabled={busy} spellCheck={false} aria-describedby="legal-markdown-help" /><FieldDescription id="legal-markdown-help">最多 100,000 个字符。预览不会执行 HTML 或脚本。</FieldDescription></Field></FieldGroup></CardContent><CardFooter><Button type="button" variant="ghost" size="sm" onClick={copyDraft}><Copy data-icon="inline-start" />复制 Markdown</Button></CardFooter></Card>}
        {view !== "edit" && <Card className="min-w-0"><CardHeader><CardTitle>实时预览</CardTitle><CardDescription>预览与公开页面使用相同排版。</CardDescription></CardHeader><CardContent><div className="legal-editor-preview" role="region" aria-label="协议实时预览" tabIndex={0}><h2 className="mb-2 text-lg font-semibold">{draft.title || legalNames[slug]}</h2>{draft.description && <p className="mb-6 text-sm text-muted-foreground">{draft.description}</p>}{markdown.trim() ? <MarkdownContent content={markdown} /> : <p className="text-sm text-muted-foreground">在编辑框中输入 Markdown，预览会显示在这里。</p>}</div></CardContent></Card>}
      </div>
      <div className="flex flex-wrap items-center gap-3"><Button type="submit" disabled={busy || !dirty || !draft.title.trim() || !draft.content.trim() || conflict}>{busy ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}保存并同步</Button><Button type="button" variant="outline" disabled={busy || !dirty} onClick={() => { setDrafts(current => ({ ...current, [slug]: documents[slug] })); setError(""); setNotice(""); setConflict(false) }}>放弃草稿</Button><p className="text-xs text-muted-foreground">切换协议会保留当前未保存的草稿。</p></div>
    </form>}
  </div>
}
