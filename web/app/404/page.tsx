import type { Metadata } from "next"
import { ThemeToggle } from "@/components/layout/theme-toggle"
import { AccountEntryLink } from "@/components/account/account-entry-link"
import { HomeHeader } from "@/components/home/home-header"
import { NotFoundScreen } from "@/components/layout/not-found-screen"
import { SiteFooter } from "@/components/layout/site-footer"

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
      <HomeHeader actions={<nav aria-label="页面导航" className="home-header-actions">
            <ThemeToggle />
            <AccountEntryLink />
      </nav>} />

      <main id="not-found-main" className="not-found-main" tabIndex={-1}>
        <NotFoundScreen />
      </main>

      <SiteFooter />
    </div>
  )
}
