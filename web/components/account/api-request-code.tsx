import { cn } from "cn"
import { publicEndpointPath } from "@/lib/athub/athub-permissions"

const methodColors: Record<string, string> = {
  GET: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  HEAD: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  POST: "bg-blue-500/10 text-blue-700 dark:text-blue-400",
  PUT: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
  PATCH: "bg-violet-500/10 text-violet-700 dark:text-violet-400",
  DELETE: "bg-rose-500/10 text-rose-700 dark:text-rose-400",
  OPTIONS: "bg-slate-500/10 text-slate-700 dark:text-slate-400",
}

export function APIRequestCode({ method, path, className }: { method: string; path: string; className?: string }) {
  return <span className={cn("inline-flex items-center gap-3 font-mono text-xs", className)}>
    <span className={cn("inline-flex min-w-14 shrink-0 justify-center rounded-md px-2 py-1 font-semibold", methodColors[method.toUpperCase()] || "bg-muted text-muted-foreground")}>{method}</span>
    <code className="min-w-0 break-all rounded-md bg-sky-500/5 px-2 py-1 text-sky-700 dark:text-sky-300">{publicEndpointPath(path)}</code>
  </span>
}

export function HTTPStatusCode({ status }: { status: number | null }) {
  const color = status === null ? "bg-muted text-muted-foreground"
    : status >= 500 ? "bg-rose-500/10 text-rose-700 dark:text-rose-400"
    : status >= 400 ? "bg-amber-500/10 text-amber-700 dark:text-amber-400"
    : status >= 300 ? "bg-sky-500/10 text-sky-700 dark:text-sky-400"
    : status >= 200 ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"
    : "bg-muted text-muted-foreground"

  return <span className={cn("inline-flex min-w-12 justify-center whitespace-nowrap rounded-md px-2 py-1 font-mono text-xs font-medium tabular-nums", color)}>{status ?? "处理中"}</span>
}
