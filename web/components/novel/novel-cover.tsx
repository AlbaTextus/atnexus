"use client"

import { useState } from "react"

export function NovelCover({ src, title, full = false }: { src: string | null; title: string; full?: boolean }) {
  const [failed, setFailed] = useState<string | null>(null)
  return src && failed !== src ? <img className={full ? "full-cover" : undefined} src={src} alt={title} width={600} height={800} referrerPolicy="no-referrer" onError={() => setFailed(src)} />
    : <span className={`library-cover-fallback${full ? " full-cover" : ""}`}>暂无图片</span>
}
