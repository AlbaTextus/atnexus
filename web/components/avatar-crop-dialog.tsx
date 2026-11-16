"use client"

import { useEffect, useRef, useState } from "react"
import Cropper, { type Area } from "react-easy-crop"
import { AlertCircle, Crop, RotateCcw } from "lucide-react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { Slider } from "@/components/ui/slider"
import { Spinner } from "@/components/ui/spinner"

function canvasBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => canvas.toBlob(blob => blob ? resolve(blob) : reject(new Error("无法生成头像，请重新选择图片。")), "image/png"))
}

export function AvatarCropDialog({ file, onCancel, onConfirm, target = "avatar" }: { file: File; onCancel: () => void; onConfirm: (file: File) => void; target?: "avatar" | "background" }) {
  const isBackground = target === "background"
  const label = isBackground ? "背投" : "头像"
  const [imageURL, setImageURL] = useState("")
  const [crop, setCrop] = useState({ x: 0, y: 0 })
  const [zoom, setZoom] = useState(1)
  const [area, setArea] = useState<Area | null>(null)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const source = useRef<HTMLCanvasElement | null>(null)
  const gifSource = useRef(false)

  useEffect(() => {
    let active = true
    let url = ""
    async function prepare() {
      try {
        const signature = await file.slice(0, 6).text()
        const isGIF = signature === "GIF89a" || signature === "GIF87a"
        const bitmap = await createImageBitmap(file, { imageOrientation: "from-image" })
        try {
          if (bitmap.width > 8192 || bitmap.height > 8192 || bitmap.width * bitmap.height > 16_000_000) throw new Error("图片尺寸过大，请选择不超过 1600 万像素的图片。")
          if (isGIF) {
            if (!active) return
            gifSource.current = true
            url = URL.createObjectURL(file)
            setImageURL(url)
            return
          }
          // Static images honor EXIF orientation; GIFs use the original animation.
          const canvas = document.createElement("canvas")
          const scale = Math.min(1, 2048 / Math.max(bitmap.width, bitmap.height))
          canvas.width = Math.max(1, Math.round(bitmap.width * scale))
          canvas.height = Math.max(1, Math.round(bitmap.height * scale))
          const context = canvas.getContext("2d")
          if (!context) throw new Error("浏览器无法处理图片，请重试。")
          context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
          const blob = await canvasBlob(canvas)
          if (!active) return
          source.current = canvas
          url = URL.createObjectURL(blob)
          setImageURL(url)
        } finally { bitmap.close() }
      } catch (reason) {
        if (active) setError(reason instanceof Error && reason.message.includes("请") ? reason.message : "无法读取这张图片，请选择其他图片。")
      }
    }
    void prepare()
    return () => { active = false; if (url) URL.revokeObjectURL(url); source.current = null; gifSource.current = false }
  }, [file])

  async function confirm() {
    if ((!source.current && !gifSource.current) || !area || busy) return
    setBusy(true); setError("")
    try {
      if (gifSource.current) {
        const form = new FormData()
        form.append("file", file)
        form.append("crop", JSON.stringify(area))
        const response = await fetch("/api/v1/account/avatar/preview", { method: "POST", credentials: "include", headers: { "X-AlbaTextus-Client": "account-center" }, body: form, signal: AbortSignal.timeout(30000) })
        if (!response.ok) {
          const payload = await response.json().catch(() => null)
          throw new Error(payload?.error_description || "动图裁剪失败，请稍后重试。")
        }
        onConfirm(new File([await response.blob()], "avatar.gif", { type: "image/gif" }))
        return
      }
      const canvas = document.createElement("canvas")
      canvas.width = isBackground ? 1536 : 512
      canvas.height = 512
      const context = canvas.getContext("2d")
      if (!context) throw new Error("浏览器无法处理图片，请重试。")
      context.imageSmoothingQuality = "high"
      context.drawImage(source.current!, area.x, area.y, area.width, area.height, 0, 0, canvas.width, canvas.height)
      onConfirm(new File([await canvasBlob(canvas)], `${target}.png`, { type: "image/png" }))
    } catch (reason) { setError(reason instanceof Error ? reason.message : "裁剪失败，请重试或重新选择图片。"); setBusy(false) }
  }

  return <Dialog open onOpenChange={open => { if (!open && !busy) onCancel() }}>
    <DialogContent className="center-dialog avatar-crop-dialog max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-lg" showCloseButton={!busy}>
      <DialogHeader><DialogTitle>裁剪{label}</DialogTitle><DialogDescription>拖动、缩放图片，{isBackground ? "横框内是卡片背投的显示范围。" : "圆框内是头像显示范围。"}</DialogDescription></DialogHeader>
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
      <div className="avatar-crop-stage" aria-busy={!imageURL || busy}>
        {imageURL ? <Cropper image={imageURL} crop={crop} zoom={zoom} aspect={isBackground ? 3 : 1} cropShape={isBackground ? "rect" : "round"} showGrid={isBackground} minZoom={1} maxZoom={4} onCropChange={setCrop} onZoomChange={setZoom} onCropAreaChange={(_, pixels) => setArea(pixels)} cropperProps={{ "aria-label": `移动${label}裁剪区域`, "aria-describedby": "avatar-crop-keyboard-help" }} /> : !error && <Spinner aria-label="正在读取图片" />}
      </div>
      <Field data-disabled={busy || !imageURL}>
        <div className="flex items-center justify-between"><FieldLabel>缩放</FieldLabel><span className="text-xs text-muted-foreground">{Math.round(zoom * 100)}%</span></div>
        <Slider aria-label={`缩放${label}`} min={1} max={4} step={0.01} value={[zoom]} onValueChange={value => setZoom(Array.isArray(value) ? value[0] : value)} disabled={busy || !imageURL} className="py-3" />
      </Field>
      <p id="avatar-crop-keyboard-help" className="text-xs text-muted-foreground">支持触屏拖动、双指缩放；聚焦图片后可用方向键微调。</p>
      <DialogFooter className="flex-wrap">
        <Button type="button" variant="ghost" disabled={busy || !imageURL} onClick={() => { setCrop({ x: 0, y: 0 }); setZoom(1) }}><RotateCcw data-icon="inline-start" />重置</Button>
        <Button type="button" variant="outline" disabled={busy} onClick={onCancel}>取消</Button>
        <Button type="button" disabled={busy || !area} onClick={() => void confirm()}>{busy ? <Spinner data-icon="inline-start" /> : <Crop data-icon="inline-start" />}确认裁剪</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
