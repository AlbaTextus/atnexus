"use client"

export default function WikiError({ reset }: { reset: () => void }) {
  return <main className="mx-auto max-w-3xl px-6 py-16"><h1 className="text-2xl font-semibold">文档暂时无法加载</h1><p className="mt-4 text-muted-foreground">请稍后重试。</p><button type="button" onClick={reset} className="mt-6 rounded-lg border px-4 py-2">重新加载</button></main>
}
