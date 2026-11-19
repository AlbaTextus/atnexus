"use client"

import { useState } from "react"

// `fallback` is shown in place of a picture that is missing or cannot be loaded.
export function NovelCover({ src, title, full = false, fallback = "暂无图片" }: { src: string | null; title: string; full?: boolean; fallback?: string }) {
  const [failed, setFailed] = useState<string | null>(null)
  return src && failed !== src ? <img className={full ? "full-cover" : undefined} src={src} alt={title} width={600} height={800} referrerPolicy="no-referrer" onError={() => setFailed(src)} />
    : <span className={`library-cover-fallback${full ? " full-cover" : ""}`}>{fallback}</span>
}
