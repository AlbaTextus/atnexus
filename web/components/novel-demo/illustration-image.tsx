"use client";

import { useState } from "react";
import { safeContentURL, type NovelIllustration } from "@/lib/athub-client";

export function IllustrationImage({ image, label, preview = false }: { image: NovelIllustration; label: string; preview?: boolean }) {
  const [failed, setFailed] = useState(false);
  const source = safeContentURL(image.url);
  return failed || !source ? <span className="novel-illustration-unavailable">图片加载失败</span>
    : <img src={source} alt={label} width={image.width} height={image.height} draggable={false} loading={preview ? "eager" : "lazy"}
      decoding="async" referrerPolicy="no-referrer" onError={() => setFailed(true)} />;
}
