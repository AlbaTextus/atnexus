import type { ReactNode } from "react"
import { cn } from "cn"

export type AuthLayoutProps = {
  children: ReactNode
  toolbar?: ReactNode
  appearance?: "white"
  background?: "identity"
}

export function AuthLayout({ children, toolbar, appearance, background }: AuthLayoutProps) {
  return <div className={cn("auth-shell", appearance === "white" && "auth-shell-white", background === "identity" && "auth-shell-identity")}>
    {background === "identity" && <div className="auth-ambient" aria-hidden="true" />}
    <a className="auth-skip-link" href="#auth-content">跳转到表单</a>
    {toolbar && <div className="auth-theme-toolbar">{toolbar}</div>}
    <main id="auth-content" className="auth-main" tabIndex={-1}>{children}</main>
  </div>
}
