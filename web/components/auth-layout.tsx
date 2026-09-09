import type { ReactNode } from "react"
import { cn } from "@/lib/utils"
import { ThemeToggle } from "@/components/theme-toggle"
import { AnnouncementCenter } from "@/components/announcement-center"

export function AuthLayout({ children, appearance, background }: { children: ReactNode; appearance?: "white"; background?: "identity" }) {
  return (
    <div className={cn("auth-shell", appearance === "white" && "auth-shell-white", background === "identity" && "auth-shell-identity")}>
      {background === "identity" && <div className="auth-ambient" aria-hidden="true" />}
      <a className="auth-skip-link" href="#auth-content">跳转到表单</a>
      <div className="auth-theme-toolbar"><AnnouncementCenter /><ThemeToggle /></div>
      <main id="auth-content" className="auth-main" tabIndex={-1}>
        {children}
      </main>
    </div>
  )
}
