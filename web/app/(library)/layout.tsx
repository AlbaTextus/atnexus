import type { ReactNode } from "react"
import { NovelShell } from "@/components/novel-shell"
import { NovelAccessProvider } from "@/components/novel-access"
import "@/styles/novel-workspace.css"
import "@/styles/novel-demo.css"
import "@/styles/novel-integration.css"

export default function LibraryLayout({ children }: { children: ReactNode }) {
  return <NovelAccessProvider><NovelShell>{children}</NovelShell></NovelAccessProvider>
}
