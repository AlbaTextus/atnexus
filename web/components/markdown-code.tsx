"use client"

import { lazy, Suspense } from "react"

const Highlighted = lazy(() => import("@/components/code-highlighted/code-highlighted").then(module => ({ default: module.StandaloneCodeHighlighted })))

export function MarkdownCodeBlock({ code, lang }: { code: string; lang: string }) {
  return <Suspense fallback={<pre tabIndex={0} aria-label="代码示例"><code>{code}</code></pre>}><Highlighted code={code} lang={lang} showCopyButton className="legal-code" /></Suspense>
}
