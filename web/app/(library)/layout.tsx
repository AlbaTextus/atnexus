import type { ReactNode } from "react"
import { NovelShell } from "@/components/novel/novel-shell"
import { NovelAccessProvider } from "@/components/novel/novel-access"
import { TooltipProvider } from "@/components/ui/tooltip"
import "@/styles/novel/novel-workspace.css"
import "@/styles/novel/novel-pages.css"
import "@/styles/novel/novel-integration.css"
import "@/styles/novel/novel-motion.css"
import "@/styles/novel/novel-catalog.css"

export default function LibraryLayout({ children }: { children: ReactNode }) {
  return <NovelAccessProvider><TooltipProvider><NovelShell>{children}</NovelShell></TooltipProvider></NovelAccessProvider>
}
