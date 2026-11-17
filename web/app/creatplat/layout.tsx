import type { Metadata } from "next"
import type { ReactNode } from "react"
import { NovelAccessProvider } from "@/components/novel/novel-access"
import { CreatplatShell } from "@/components/creatplat/creatplat-shell"
import { TooltipProvider } from "@/components/ui/tooltip"
import "@/styles/novel/novel-workspace.css"
import "@/styles/novel/novel-pages.css"
import "@/styles/novel/novel-integration.css"
import "@/styles/novel/novel-motion.css"
import "@/styles/creatplat/creatplat.css"

export const metadata: Metadata = {
  title: { default: "创作者平台 · atnexus", template: "%s · 创作者平台 · atnexus" },
  description: "ALBATEXTUS 创作者平台",
}

export default function CreatplatLayout({ children }: { children: ReactNode }) {
  return <NovelAccessProvider><TooltipProvider><CreatplatShell>{children}</CreatplatShell></TooltipProvider></NovelAccessProvider>
}
