import type { Metadata } from "next"
import { ArrowLeft, Compass } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ThemeToggle } from "@/components/theme-toggle"
import { AccountEntryLink } from "@/components/account-entry-link"
import { BrandWordmark } from "@/components/brand-wordmark"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"

export const metadata: Metadata = {
  title: "ALBATEXTUS",
  robots: { index: false, follow: false },
  description: "这一页暂时走丢了。返回 ALBATEXTUS 首页，或前往用户中心。",
}

export default function NotFoundPage() {
  // Error-page exits load a fresh document even if client routing or transitions stall.
  return (
    <div className="home-shell home-credit-inspired not-found-shell">
      <a href="#not-found-main" className="home-skip">跳转到主要内容</a>
      <header className="home-header">
        <div className="home-container home-header-inner">
          <BrandWordmark href="/" />
          <nav aria-label="页面导航" className="home-header-actions">
            <ThemeToggle />
            <Button variant="ghost" size="icon" nativeButton={false} role="link" render={<a href="/wiki" />} aria-label="前往 Wiki" title="前往 Wiki">W</Button>
            <AccountEntryLink />
          </nav>
        </div>
      </header>

      <main id="not-found-main" className="not-found-main" tabIndex={-1}>
        <Empty className="not-found-empty">
          <div className="not-found-art" aria-hidden="true">
            <div className="not-found-orbit" />
            <span className="not-found-coordinate not-found-coordinate-start">A / 起点</span>
            <span className="not-found-coordinate not-found-coordinate-end">B / 未知坐标</span>
            <svg className="not-found-path" viewBox="0 0 560 280" fill="none">
              <path d="M42 207C100 207 106 78 200 78S326 237 407 192S416 55 507 66" />
              <circle cx="42" cy="207" r="4" />
              <circle cx="507" cy="66" r="5" />
            </svg>
            <span className="not-found-number">4<span>0</span>4</span>
            <span className="not-found-compass"><Compass strokeWidth={1.4} /></span>
            <span className="not-found-spark not-found-spark-one">+</span>
            <span className="not-found-spark not-found-spark-two">+</span>
          </div>

          <EmptyHeader className="not-found-heading">
            <p className="not-found-eyebrow">PAGE NOT FOUND</p>
            <EmptyTitle><h1>这一页，暂时走丢了。</h1></EmptyTitle>
            <EmptyDescription>
              <p>你访问的页面不存在，或已经搬到了别处。<br />没关系，回到熟悉的地方，继续下一次连接。</p>
            </EmptyDescription>
          </EmptyHeader>

          <EmptyContent className="not-found-content">
            <Button size="lg" nativeButton={false} role="link" render={<a href="/" />}>
              <ArrowLeft data-icon="inline-start" />返回首页
            </Button>
          </EmptyContent>
        </Empty>
      </main>

      <footer className="not-found-footer home-container">
        <p>© 2026 ALBATEXTUS</p>
        <nav aria-label="法律信息"><a href="/wiki/privacy">隐私政策</a><a href="/wiki/terms">服务条款</a></nav>
      </footer>
    </div>
  )
}
