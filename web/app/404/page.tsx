import type { Metadata } from "next"
import { Button } from "@/components/ui/button"
import { ThemeToggle } from "@/components/theme-toggle"
import { AccountEntryLink } from "@/components/account-entry-link"
import { BrandWordmark } from "@/components/brand-wordmark"
import { wikiUrl } from "@/lib/wiki"
import { NotFoundScreen } from "@/components/not-found-screen"
import { SiteFooter } from "@/components/site-footer"

export const metadata: Metadata = {
  title: "ALBATEXTUS",
  robots: { index: false, follow: false },
  description: "这一页偏离了轨道。听听空白的回声，或返回 ALBATEXTUS 首页与用户中心。",
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
            <Button variant="ghost" size="icon" nativeButton={false} role="link" render={<a href={wikiUrl()} />} aria-label="前往 Wiki" title="前往 Wiki">W</Button>
            <AccountEntryLink />
          </nav>
        </div>
      </header>

      <main id="not-found-main" className="not-found-main" tabIndex={-1}>
        <NotFoundScreen />
      </main>

      <SiteFooter />
    </div>
  )
}
