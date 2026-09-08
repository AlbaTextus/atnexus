import type { Metadata } from "next"
import type React from "react"
import { ToastProvider } from "@/components/ui/toast"
import { ViewTransitions } from "next-view-transitions"
import "./globals.css"

export const metadata: Metadata = {
  title: "AlbaTextus 账户",
  description: "AlbaTextus OAuth 账户中心",
}

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <ViewTransitions><html lang="zh-CN" suppressHydrationWarning>
      <body><ToastProvider>{children}</ToastProvider></body>
    </html></ViewTransitions>
  )
}
