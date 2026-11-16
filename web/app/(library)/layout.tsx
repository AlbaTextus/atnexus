import type { ReactNode } from "react"
import { NovelShell } from "@/components/novel-shell"
import { NovelAccessProvider } from "@/components/novel-access"
import { TooltipProvider } from "@/components/ui/tooltip"
import "@/styles/novel-workspace.css"
import "@/styles/novel-demo.css"
import "@/styles/novel-integration.css"
import "@/styles/novel-motion.css"
import "@/styles/novel-catalog.css"

export default function LibraryLayout({ children }: { children: ReactNode }) {
  return <NovelAccessProvider><TooltipProvider><NovelShell>{children}</NovelShell></TooltipProvider></NovelAccessProvider>
}
