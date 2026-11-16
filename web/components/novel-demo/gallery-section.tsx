"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, Loader2, Trash2, Upload } from "lucide-react";
import { useNovelAccess } from "@/components/novel-access";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { api, ApiError } from "@/lib/api";
import { safeContentURL, type NovelIllustration } from "@/lib/athub-client";
import { allowsEndpoint, type RolePolicy } from "@/lib/athub-permissions";
import { readAllContent } from "@/lib/novel-demo-data";
import { ContentSectionHeading } from "./content-section-heading";
import { useNovelEditor } from "@/components/novel-inline-editor";
import { IllustrationImage } from "./illustration-image";
import { IllustrationCanvas } from "./illustration-canvas";

type FailedUpload = { file: File; message: string };
type UploadResult = NovelIllustration | { status: "pending"; review_id: string };
const canUpload = (policy: RolePolicy) => allowsEndpoint(policy, "upload", "POST", "/api/v2/illustrations");
const canDelete = (policy: RolePolicy) => allowsEndpoint(policy, "delete", "DELETE", "/api/v2/illustrations");

export function GallerySection({ novelId }: { novelId: number }) {
  const { request } = useNovelAccess();
  const editor = useNovelEditor();
  const editing = Boolean(editor?.mode && !editor.loading);
  const input = useRef<HTMLInputElement>(null);
  const uploading = useRef(false);
  const previewTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [images, setImages] = useState<NovelIllustration[]>([]);
  const [hasDisplay, setHasDisplay] = useState(true);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [revision, setRevision] = useState(0);
  const [policy, setPolicy] = useState<RolePolicy | null>(null);
  const [permissionError, setPermissionError] = useState("");
  const [progress, setProgress] = useState("");
  const [message, setMessage] = useState("");
  const [failures, setFailures] = useState<FailedUpload[]>([]);
  const [selected, setSelected] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null);
  const [pendingDeletes, setPendingDeletes] = useState<number[]>([]);

  useEffect(() => () => { if (previewTimer.current) clearTimeout(previewTimer.current); }, []);
  useEffect(() => { if (!editing) setConfirmDelete(null); }, [editing]);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setLoadError("");
    readAllContent<NovelIllustration>(request, `/api/v2/illustrations?novelId=${novelId}`, controller.signal)
      .then(value => { if (!controller.signal.aborted) setImages(value); })
      .catch(reason => { if (!controller.signal.aborted) setLoadError(reason instanceof Error ? reason.message : "插图加载失败。"); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [novelId, request, revision]);

  useEffect(() => {
    const controller = new AbortController();
    setPermissionError("");
    api<RolePolicy>("/api/v1/account/athub/permissions", { signal: controller.signal, cache: "no-store" })
      .then(value => { if (!controller.signal.aborted) setPolicy(value); })
      .catch(() => { if (!controller.signal.aborted) setPermissionError("无法读取上传权限，请重试。"); });
    return () => controller.abort();
  }, [revision]);

  async function upload(files: File[]) {
    if (!editing || uploading.current || !files.length) return;
    uploading.current = true; setBusy(true); setMessage(""); setFailures([]);
    let saved = 0, pending = 0;
    const failed: FailedUpload[] = [];
    try {
      // Recheck before starting; the server checks current permissions again.
      const current = await api<RolePolicy>("/api/v1/account/athub/permissions", { cache: "no-store" });
      setPolicy(current);
      if (!canUpload(current)) throw new Error("当前账户没有上传插图的权限。");
      for (const [index, file] of files.entries()) {
        setProgress(`正在上传 ${index + 1} / ${files.length}`);
        try {
          if (!file.size || file.size > 5 * 1024 * 1024) throw new Error("图片不能为空，且不能超过 5 MiB。");
          if (!["image/jpeg", "image/png", "image/webp", "image/gif"].includes(file.type)) throw new Error("请选择 JPG、PNG、WebP 或 GIF 图片。");
          const form = new FormData();
          form.append("file", file);
          const result = await api<UploadResult>(`/api/v1/account/athub/novels/${novelId}/illustrations`, { method: "POST", body: form });
          if ("review_id" in result && result.status === "pending" && result.review_id) pending++;
          else if ("id" in result && result.novelId === novelId && safeContentURL(result.url)) saved++;
          else throw new Error("未能确认保存结果，请刷新查看后再重试。");
        } catch (reason) {
          const message = reason instanceof ApiError && reason.status === 401 ? "登录已失效，请重新登录。"
            : reason instanceof Error ? reason.message : "未能确认上传结果，请刷新查看后重试。";
          failed.push({ file, message });
        }
      }
      setFailures(failed);
      setMessage([saved ? `${saved} 张插图已保存。` : "", pending ? `${pending} 张插图已提交审核，通过后显示。` : ""].filter(Boolean).join(" "));
      if (saved) setRevision(value => value + 1);
    } catch (reason) {
      setFailures(files.map(file => ({ file, message: reason instanceof Error ? reason.message : "上传失败，请重试。" })));
    } finally {
      uploading.current = false; setBusy(false); setProgress("");
      if (input.current) input.current.value = "";
    }
  }

  async function remove(image: NovelIllustration) {
    if (!editing || uploading.current) return;
    uploading.current = true; setBusy(true); setMessage("");
    try {
      const current = await api<RolePolicy>("/api/v1/account/athub/permissions", { cache: "no-store" });
      setPolicy(current);
      if (!canDelete(current)) throw new Error("当前账户没有删除插图的权限。");
      const result = await api<{ status: "pending"; review_id: string } | null>(
        `/api/v1/account/athub/novels/${novelId}/illustrations/${image.id}`, { method: "DELETE" });
      if (result?.status === "pending" && result.review_id) {
        setPendingDeletes(value => [...value, image.id]);
        setMessage("删除已提交审核，通过后生效。");
      } else if (result === null) {
        setImages(value => value.filter(item => item.id !== image.id));
        setSelected(null);
        setMessage("插图已删除。");
      } else throw new Error("未能确认删除结果，请刷新查看后再操作。");
      setConfirmDelete(null);
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "删除失败，请重试。");
    } finally {
      uploading.current = false; setBusy(false);
    }
  }

  const active = selected === null ? null : images[selected];
  return (
    <section className="panel overflow-hidden p-5 sm:p-6" aria-label="插图"
      hidden={!editing && !loading && !loadError && (images.length === 0 || !hasDisplay)} onDoubleClick={event => {
      if (!editing && editor && policy && (canUpload(policy) || canDelete(policy) || allowsEndpoint(policy, "edit", "PUT", "/api/v2/illustration-layout"))) {
        event.preventDefault();
        if (previewTimer.current) clearTimeout(previewTimer.current);
        editor.begin();
      }
    }}>
      <div className="novel-illustration-heading">
        <ContentSectionHeading className="illustrations-heading">插图</ContentSectionHeading>
        <input ref={input} type="file" multiple accept="image/jpeg,image/png,image/webp,image/gif" className="sr-only" aria-label="选择插图"
          disabled={!editing || busy || !policy || !canUpload(policy)} onChange={event => { void upload(Array.from(event.target.files || [])); }} />
        {editing && (!policy || canUpload(policy)) && <Button type="button" variant="ghost" size="sm" disabled={busy || !policy} onClick={() => input.current?.click()}>
          {busy ? <Loader2 className="animate-spin" aria-hidden="true" /> : <Upload aria-hidden="true" />}{busy ? progress || "正在检查权限…" : "上传插图"}
        </Button>}
      </div>
      {editing && <p className="novel-illustration-edit-note">图片上传、删除单独提交。</p>}
      {editing && permissionError && <p role="alert">{permissionError} <Button variant="ghost" size="sm" onClick={() => setRevision(value => value + 1)}>重试</Button></p>}
      {editing && message && <p className="novel-illustration-status" role="status">{message}</p>}
      {failures.length > 0 && <div className="novel-illustration-status" role="alert">
        <ul>{failures.map(({ file, message }, index) => <li key={index}>{file.name}：{message}</li>)}</ul>
        {editing && <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => { void upload(failures.map(item => item.file)); }}>重试失败的图片</Button>}
      </div>}
      {loadError ? <div className="novel-illustration-status" role="alert">{loadError} <Button variant="ghost" size="sm" onClick={() => setRevision(value => value + 1)}>重试</Button></div>
        : loading && images.length === 0 ? <p className="novel-illustration-status" role="status">正在加载插图…</p>
        : <IllustrationCanvas novelId={novelId} images={images} editing={editing} policy={policy} onDisplayChange={setHasDisplay}
          onPreview={id => {
            if (previewTimer.current) clearTimeout(previewTimer.current);
            const show = () => setSelected(images.findIndex(image => image.id === id));
            if (!editing && editor && policy && (canUpload(policy) || canDelete(policy) || allowsEndpoint(policy, "edit", "PUT", "/api/v2/illustration-layout"))) previewTimer.current = setTimeout(show, 280);
            else show();
          }}
          resourceActions={(image, index) => editing && policy && canDelete(policy) && <div className="novel-illustration-item-actions">
            {pendingDeletes.includes(image.id) ? <span role="status">删除待审核</span> : confirmDelete === image.id ? <>
              <Button type="button" size="sm" variant="destructive" disabled={busy} onClick={() => { void remove(image); }}>确认删除</Button>
              <Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => setConfirmDelete(null)}>取消</Button>
            </> : <Button type="button" size="sm" variant="ghost" className="text-red-500 hover:text-red-600"
              aria-label={`删除插图 ${index + 1}`} disabled={busy} onClick={() => setConfirmDelete(image.id)}><Trash2 className="size-4" aria-hidden="true" />删除</Button>}
          </div>}
        />}
      <Dialog open={Boolean(active)} onOpenChange={open => { if (!open) setSelected(null); }}>
        <DialogContent className="novel-illustration-dialog" aria-describedby={undefined}>
          <DialogHeader><DialogTitle>插图 {selected === null ? "" : selected + 1} / {images.length}</DialogTitle></DialogHeader>
          {active && <IllustrationImage key={active.id} image={active} label={`插图 ${selected! + 1}`} preview />}
          {images.length > 1 && <div className="novel-illustration-pagination">
            <Button variant="ghost" disabled={selected === 0} onClick={() => setSelected(index => Math.max(0, (index ?? 0) - 1))}><ChevronLeft aria-hidden="true" />上一张</Button>
            <Button variant="ghost" disabled={selected === images.length - 1} onClick={() => setSelected(index => Math.min(images.length - 1, (index ?? 0) + 1))}>下一张<ChevronRight aria-hidden="true" /></Button>
          </div>}
        </DialogContent>
      </Dialog>
    </section>
  );
}
