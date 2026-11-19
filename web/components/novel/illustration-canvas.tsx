"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent, type ReactNode } from "react";
import { Loader2, Magnet, Plus, RotateCcw, Save, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useNovelAccess } from "@/components/novel/novel-access";
import { useNovelEditor } from "@/components/novel/novel-inline-editor";
import { api } from "@/lib/api";
import type { IllustrationBlock, IllustrationLayout, NovelIllustration } from "@/lib/athub/athub-client";
import { contentErrorMessage } from "@/lib/athub/athub-errors";
import { allowsEndpoint, type RolePolicy } from "@/lib/athub/athub-permissions";
import { createIllustrationBlock, fitIllustrationBlock, normalizeIllustrationBlock, transformIllustrationBlock, ILLUSTRATION_CANVAS_WIDTH as WIDTH, type BlockOperation, type LayoutGuides, type ResizeHandle } from "@/lib/novel/illustration-layout";
import { IllustrationImage } from "./illustration-image";
import { NovelAutoHeight } from "@/components/novel/novel-motion";
import { NovelPendingLayouts, NovelPendingUploads } from "@/components/novel/novel-pending-changes";

const RESOURCE_MIME = "application/x-albatextus-illustration";
const handles: { handle: ResizeHandle; label: string }[] = [
  { handle: "n", label: "上边" }, { handle: "ne", label: "右上角" }, { handle: "e", label: "右边" }, { handle: "se", label: "右下角" },
  { handle: "s", label: "下边" }, { handle: "sw", label: "左下角" }, { handle: "w", label: "左边" }, { handle: "nw", label: "左上角" },
];
type Gesture = {
  pointer: number; x: number; y: number; clientX: number; clientY: number; altKey: boolean; height: number; operation: BlockOperation;
  block: IllustrationBlock; before: IllustrationBlock[]; layout: IllustrationBlock[]; moved: boolean;
};

export function IllustrationCanvas({ novelId, images, editing, policy, onPreview, resourceActions, onDisplayChange }: {
  novelId: number; images: NovelIllustration[]; editing: boolean; policy: RolePolicy | null;
  onPreview: (id: number) => void; resourceActions: (image: NovelIllustration, index: number) => ReactNode;
  onDisplayChange: (visible: boolean) => void;
}) {
  const { request } = useNovelAccess();
  const editor = useNovelEditor();
  const registerLayout = editor?.registerLayout;
  const canvas = useRef<HTMLDivElement>(null);
  const gesture = useRef<Gesture | null>(null);
  const saving = useRef(false);
  const dirtyRef = useRef(false);
  const [saved, setSaved] = useState<IllustrationLayout | null>(null);
  const [blocks, setBlocks] = useState<IllustrationBlock[]>([]);
  const [selected, setSelected] = useState<number | null>(null);
  const [guides, setGuides] = useState<LayoutGuides>({});
  const [dragging, setDragging] = useState(false);
  const [snapping, setSnapping] = useState(false);
  const [dropOver, setDropOver] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const dirty = Boolean(saved && JSON.stringify(saved.blocks) !== JSON.stringify(blocks));
  dirtyRef.current = dirty;
  const allowed = Boolean(policy && allowsEndpoint(policy, "edit", "PUT", "/api/v2/illustration-layout"));
  const interactive = editing && allowed && !busy && !editor?.busy && !loading && Boolean(saved);
  const imageMap = useMemo(() => new Map(images.map(image => [image.id, image])), [images]);
  const normalizeBlocks = useCallback((items: IllustrationBlock[]) => items.map(block => {
    const image = imageMap.get(block.imageId);
    return image ? normalizeIllustrationBlock(block, image) : block;
  }), [imageMap]);

  useEffect(() => {
    const abort = new AbortController();
    setLoading(true);
    request<IllustrationLayout>(`/api/v4/account/athub/novels/${novelId}/illustration-layout`, abort.signal)
      .then(value => {
        if (abort.signal.aborted) return;
        if (value.novelId !== novelId || !Array.isArray(value.blocks) || !Number.isInteger(value.revision)) throw new Error("插图布局数据无效。");
        if (!dirtyRef.current) { setSaved(value); setBlocks(value.blocks); setError(""); }
      })
      .catch(reason => { if (!abort.signal.aborted) setError(reason instanceof Error ? reason.message : "插图布局加载失败。"); })
      .finally(() => { if (!abort.signal.aborted) setLoading(false); });
    return () => abort.abort();
  }, [novelId, request, revision]);

  // A resource deleted from this book must also disappear from any local draft.
  useEffect(() => {
    const ids = new Set(images.map(image => image.id));
    setBlocks(current => current.some(block => !ids.has(block.imageId)) ? current.filter(block => ids.has(block.imageId)) : current);
  }, [images]);

  const reset = useCallback(() => {
    if (saving.current) return;
    setBlocks(saved?.blocks || []); setSelected(null); setError(""); setNotice("");
  }, [saved]);

  const save = useCallback(async (): Promise<"saved" | "pending" | null> => {
    if (saving.current || !saved) return null;
    if (!dirty) return "saved";
    saving.current = true; setBusy(true); setError(""); setNotice("");
    try {
      const value = await api<IllustrationLayout | { status: "pending"; review_id: string }>(
        `/api/v4/account/athub/novels/${novelId}/illustration-layout`,
        { method: "PUT", body: JSON.stringify({ novelId, revision: saved.revision, blocks: normalizeBlocks(blocks) }) });
      if ("status" in value && value.status === "pending" && value.review_id) {
        setBlocks(saved.blocks); setNotice("插图布局已提交审核，通过后生效。"); return "pending";
      }
      if (!("novelId" in value) || value.novelId !== novelId || !Array.isArray(value.blocks) || value.revision <= saved.revision) {
        throw new Error("未能确认布局保存结果，请刷新查看后再操作。");
      }
      setSaved(value); setBlocks(value.blocks); setNotice("插图布局已保存。"); return "saved";
    } catch (reason) {
      setError(contentErrorMessage(reason, "布局保存失败，请重试。", {
        403: "当前账户没有调整插图布局的权限。",
        409: "布局已被其他人修改，请重新加载布局后再调整。",
      }));
      return null;
    } finally { saving.current = false; setBusy(false); }
  }, [novelId, saved, blocks, dirty, normalizeBlocks]);

  useEffect(() => {
    registerLayout?.({ dirty, busy, save, cancel: reset });
    return () => registerLayout?.(null);
  }, [registerLayout, dirty, busy, save, reset]);

  function add(image: NovelIllustration, point?: { x: number; y: number }) {
    if (!interactive) return;
    setBlocks(current => {
      if (current.some(block => block.imageId === image.id)) return current;
      const layout = normalizeBlocks(current);
      return [...layout, createIllustrationBlock(image, layout, point)];
    });
    setSelected(image.id); setError(""); setNotice("");
  }
  function remove(id: number) {
    if (!interactive) return;
    setBlocks(current => current.filter(block => block.imageId !== id)); setSelected(null); setNotice("");
  }
  function canvasPoint(clientX: number, clientY: number) {
    const rect = canvas.current!.getBoundingClientRect();
    // clientLeft/clientWidth are CSS pixels; the rect includes the site's zoom.
    const zoom = rect.width / canvas.current!.offsetWidth;
    const scale = WIDTH / (canvas.current!.clientWidth * zoom);
    return { x: (clientX - rect.left - canvas.current!.clientLeft * zoom) * scale,
      y: (clientY - rect.top - canvas.current!.clientTop * zoom) * scale, scale };
  }
  function start(event: PointerEvent<HTMLElement>, block: IllustrationBlock, operation: BlockOperation) {
    if (!interactive || !event.isPrimary || event.button !== 0 || gesture.current || !canvas.current) return;
    event.preventDefault(); event.stopPropagation();
    event.currentTarget.focus({ preventScroll: true });
    const point = canvasPoint(event.clientX, event.clientY);
    gesture.current = { pointer: event.pointerId, x: point.x, y: point.y, clientX: event.clientX, clientY: event.clientY, altKey: event.altKey, height, operation, block: { ...block }, before: blocks, layout: normalizeBlocks(blocks), moved: false };
    canvas.current.setPointerCapture(event.pointerId);
    setSelected(block.imageId); setNotice("");
  }
  const movePointer = useCallback((clientX: number, clientY: number, altKey: boolean) => {
    const active = gesture.current;
    if (!active || !canvas.current) return;
    if (!active.moved && Math.hypot(clientX - active.clientX, clientY - active.clientY) < 2) return;
    active.clientX = clientX; active.clientY = clientY; active.altKey = altKey;
    const point = canvasPoint(clientX, clientY);
    active.moved = true;
    const next = transformIllustrationBlock(active.block, active.operation, point.x - active.x, point.y - active.y,
      active.layout.filter(block => block.imageId !== active.block.imageId), snapping && !altKey ? 8 * point.scale : 0);
    active.height = Math.max(active.height, next.block.y + next.block.height + 140);
    setBlocks(active.layout.map(block => block.imageId === active.block.imageId ? next.block : block));
    setGuides(next.guides); setDragging(true);
  }, [snapping]);
  function move(event: PointerEvent) {
    if (gesture.current?.pointer === event.pointerId) movePointer(event.clientX, event.clientY, event.altKey);
  }
  useEffect(() => {
    const onScroll = () => {
      const active = gesture.current;
      if (active?.moved) movePointer(active.clientX, active.clientY, active.altKey);
    };
    window.addEventListener("scroll", onScroll, true);
    return () => window.removeEventListener("scroll", onScroll, true);
  }, [movePointer]);
  function end(cancel = false) {
    const active = gesture.current;
    if (!active) return;
    gesture.current = null;
    if (cancel) setBlocks(active.before);
    if (canvas.current?.hasPointerCapture(active.pointer)) canvas.current.releasePointerCapture(active.pointer);
    setGuides({}); setDragging(false);
  }
  const displayed = editing ? blocks : saved?.blocks || [];
  const visible = normalizeBlocks(displayed).filter(block => imageMap.has(block.imageId));
  const height = Math.max(editing ? 400 : 1, gesture.current?.height || 0, ...visible.map(block => block.y + block.height + (editing ? 140 : 0)));
  const legacy = !editing && saved?.revision === 0;
  const hasDisplay = loading || Boolean(error) || !saved || (saved.revision === 0 ? images.length > 0 : saved.blocks.some(block => imageMap.has(block.imageId)));

  useEffect(() => {
    onDisplayChange(hasDisplay);
  }, [hasDisplay, onDisplayChange]);

  return <NovelAutoHeight className="illustration-composer" disabled={dragging}>
    {error && <div className="novel-illustration-status" role="alert">{error} <Button variant="ghost" size="sm" disabled={busy} onClick={() => { dirtyRef.current = false; setSaved(null); setBlocks([]); setRevision(value => value + 1); }}>重新加载布局</Button></div>}
    {editing && notice && <p className="novel-illustration-status" role="status">{notice}</p>}
    {editing && <div className="illustration-canvas-toolbar">
      <span>展示区</span>
      {allowed && <div className="flex items-center gap-2">
        <Button variant={snapping ? "secondary" : "ghost"} size="sm" disabled={!interactive} aria-pressed={snapping}
          title="启用边缘对齐吸附，按住 Alt 可临时自由移动" onClick={() => setSnapping(value => !value)}><Magnet aria-hidden="true" />对齐吸附</Button>
        <Button variant="ghost" size="sm" disabled={!dirty || busy || Boolean(editor?.busy)} onClick={reset}><RotateCcw aria-hidden="true" />还原布局</Button>
        <Button variant="outline" size="sm" disabled={!dirty || busy || Boolean(editor?.busy)} onClick={() => { void save(); }}>
          {busy ? <Loader2 className="animate-spin" aria-hidden="true" /> : <Save aria-hidden="true" />}{policy?.review_required ? "提交布局审核" : "保存布局"}
        </Button>
      </div>}
    </div>}
    {loading && !saved ? <p role="status" className="novel-illustration-status">正在加载插图布局…</p> : legacy ?
      <div className="novel-illustration-grid">{images.map((image, index) => <button key={image.id} className="novel-illustration-thumbnail" onClick={() => onPreview(image.id)} aria-label={`查看插图 ${index + 1}`}><IllustrationImage image={image} label={`插图 ${index + 1}`} /></button>)}</div>
      : saved && (editing || visible.length > 0) && <div ref={canvas} className="illustration-canvas" role="group" aria-label="插图展示画布"
        data-editing={editing || undefined} data-dragging={dragging || undefined} data-drop-over={dropOver || undefined} style={{ aspectRatio: `${WIDTH} / ${height}` }}
        onPointerDown={() => setSelected(null)} onPointerMove={move} onPointerUp={() => end()} onPointerCancel={() => end(true)} onLostPointerCapture={() => end(true)}
        onKeyDown={event => { if (event.key === "Escape" && gesture.current) { event.preventDefault(); end(true); } }}
        onDragOver={event => { if (interactive && event.dataTransfer.types.includes(RESOURCE_MIME)) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; setDropOver(true); } }}
        onDragLeave={event => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropOver(false); }}
        onDrop={event => {
          event.preventDefault(); setDropOver(false);
          if (!interactive) return;
          const image = imageMap.get(Number(event.dataTransfer.getData(RESOURCE_MIME)));
          if (!image) return;
          add(image, canvasPoint(event.clientX, event.clientY));
        }}>
        {editing && !visible.length && <div className="illustration-canvas-empty">{allowed ? "从下方拖入图片" : "暂无展示插图"}</div>}
        {visible.map((block, index) => {
          const image = imageMap.get(block.imageId)!;
          return <div key={block.imageId} className="illustration-block" data-image-id={block.imageId} data-selected={editing && selected === block.imageId || undefined}
            style={{ left: `${block.x / WIDTH * 100}%`, top: `${block.y / height * 100}%`, width: `${block.width / WIDTH * 100}%`, height: `${block.height / height * 100}%` }}
            role={editing ? "group" : "button"} tabIndex={0} aria-label={editing ? `图片块 ${index + 1}` : `查看插图 ${images.findIndex(item => item.id === block.imageId) + 1}`}
            onFocus={() => { if (editing) setSelected(block.imageId); }}
            onPointerDown={event => start(event, block, "move")}
            onClick={() => { if (!editing) onPreview(image.id); }}
            onKeyDown={event => {
              if (!editing && event.key === "Enter") { onPreview(image.id); return; }
              if (!interactive || gesture.current) return;
              if (event.key === "Delete" || event.key === "Backspace") { event.preventDefault(); remove(block.imageId); return; }
              const delta = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[event.key];
              if (!delta) return;
              event.preventDefault(); event.stopPropagation();
              const step = event.shiftKey ? 10 : 1;
              setBlocks(current => current.map(item => item.imageId === block.imageId ? fitIllustrationBlock({ ...block, x: block.x + delta[0] * step, y: block.y + delta[1] * step }) : item));
              setSelected(block.imageId);
            }}>
            <IllustrationImage image={image} label={`插图 ${images.findIndex(item => item.id === block.imageId) + 1}`} />
            {interactive && <>
              <button type="button" className="illustration-block-remove" aria-label={`将插图 ${index + 1} 移出画布`}
                onPointerDown={event => event.stopPropagation()} onClick={event => { event.stopPropagation(); remove(block.imageId); }}><X aria-hidden="true" /></button>
              {handles.map(({ handle, label }) => <span key={handle} className="illustration-resize-handle" data-handle={handle} role="button" tabIndex={0}
                aria-label={`调整图片块 ${index + 1} ${label}大小`} onPointerDown={event => start(event, block, handle)}
                onKeyDown={event => {
                  const delta = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[event.key];
                  if (!delta) return;
                  event.preventDefault(); event.stopPropagation();
                  const step = event.shiftKey ? 20 : 2;
                  const next = transformIllustrationBlock(block, handle, delta[0] * step, delta[1] * step, [], 0).block;
                  setBlocks(current => current.map(item => item.imageId === block.imageId ? next : item));
                }} />)}
            </>}
          </div>;
        })}
        {guides.x !== undefined && <span className="illustration-snap-guide is-vertical" style={{ left: `${guides.x / WIDTH * 100}%` }} />}
        {guides.y !== undefined && <span className="illustration-snap-guide is-horizontal" style={{ top: `${guides.y / height * 100}%` }} />}
      </div>}
    {editing && <NovelPendingLayouts images={imageMap} />}
    {editing && <section className="illustration-resources novel-motion-enter" aria-label="插图资源">
      <h3>资源</h3>
      {images.length ? <div className="illustration-resource-list">{images.map((image, index) => {
        const placed = blocks.some(block => block.imageId === image.id);
        return <div key={image.id} className="illustration-resource" data-placed={placed || undefined} data-resource-id={image.id}
          draggable={interactive && !placed} onDragStart={event => {
            if (!interactive || placed) { event.preventDefault(); return; }
            event.dataTransfer.setData(RESOURCE_MIME, String(image.id)); event.dataTransfer.effectAllowed = "copy";
          }} onDragEnd={() => setDropOver(false)}>
          <button type="button" className="illustration-resource-preview" onClick={() => onPreview(image.id)} aria-label={`查看插图 ${index + 1}`}><IllustrationImage image={image} label={`插图 ${index + 1}`} /></button>
          {allowed && <Button variant="ghost" size="sm" disabled={!interactive || placed} aria-label={`添加插图 ${index + 1} 到画布`} onClick={() => add(image)}>
            {!placed && <Plus aria-hidden="true" />}{placed ? "已添加" : "添加"}
          </Button>}
          {resourceActions(image, index)}
        </div>;
      })}</div> : <p className="illustration-resources-empty">暂无图片资源</p>}
      <NovelPendingUploads />
    </section>}
  </NovelAutoHeight>;
}
