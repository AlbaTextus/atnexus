import type { ReactNode } from "react"

export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="auth-shell">
      <a className="auth-skip-link" href="#auth-content">跳转到表单</a>
      <main id="auth-content" className="auth-main" tabIndex={-1}>
        {children}
      </main>
    </div>
  )
}
