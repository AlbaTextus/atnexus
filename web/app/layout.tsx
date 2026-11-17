import type { Metadata } from "next"
import type React from "react"
import { ToastProvider } from "@/components/ui/toast"
import { ViewTransitions } from "next-view-transitions"
import { MaintenanceWatcher } from "@/components/maintenance/maintenance-watcher"
import { ThemeProvider } from "@/components/layout/theme-provider"
import { ScrollbarBehavior } from "@/components/layout/scrollbar-behavior"
// Separate font entries keep Turbopack's PostCSS IPC payloads small on Windows.
import "../fonts/maple-mono-cn/regular.css"
import "../fonts/maple-mono-cn/medium.css"
import "../fonts/maple-mono-cn/compact-regular.css"
import "../fonts/maple-mono-cn/compact-medium.css"
import "./globals.css"

export const metadata: Metadata = {
  title: "ALBATEXTUS",
  description: "ALBATEXTUS OAuth 账户中心",
}

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <ViewTransitions><html lang="zh-CN" suppressHydrationWarning>
      <body><ThemeProvider><ToastProvider><ScrollbarBehavior /><MaintenanceWatcher />{children}</ToastProvider></ThemeProvider></body>
    </html></ViewTransitions>
  )
}
