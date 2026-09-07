import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

export function AuthLayout({ children, appearance }: { children: ReactNode; appearance?: "white" }) {
  return (
    <div className={cn("auth-shell", appearance === "white" && "auth-shell-white")}>
      <a className="auth-skip-link" href="#auth-content">跳转到表单</a>
      <main id="auth-content" className="auth-main" tabIndex={-1}>
        {children}
      </main>
    </div>
  )
}
