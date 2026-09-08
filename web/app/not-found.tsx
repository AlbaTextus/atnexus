import type { Metadata } from "next"
import Link from "next/link"
import { ArrowLeft, ArrowUpRight, BookOpen, Compass, UserRound } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ThemeToggle } from "@/components/theme-toggle"
import { HomeBrand } from "@/components/home-brand"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"

export const metadata: Metadata = {
  title: "页面未找到 · ALBATEXTUS",
  description: "这一页暂时走丢了。返回 ALBATEXTUS 首页，或前往用户中心。",
}

export default function NotFound() {
  return (
    <div className="home-shell home-credit-inspired not-found-shell">
      <a href="#not-found-main" className="home-skip">跳转到主要内容</a>
      <header className="home-header">
        <div className="home-container home-header-inner">
          <HomeBrand href="/" />
          <nav aria-label="页面导航" className="home-header-actions">
            <ThemeToggle />
            <Button variant="ghost" nativeButton={false} role="link" render={<Link href="/wiki" />}>Wiki</Button>
            <Button nativeButton={false} role="link" render={<Link href="/account" />}>用户中心<ArrowUpRight data-icon="inline-end" /></Button>
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
            <Button size="lg" nativeButton={false} role="link" render={<Link href="/" />}>
              <ArrowLeft data-icon="inline-start" />返回首页
            </Button>
            <nav className="not-found-shortcuts" aria-label="继续访问">
              <Link href="/account"><UserRound aria-hidden="true" />用户中心<ArrowUpRight aria-hidden="true" /></Link>
              <span aria-hidden="true" />
              <Link href="/wiki"><BookOpen aria-hidden="true" />浏览 Wiki<ArrowUpRight aria-hidden="true" /></Link>
            </nav>
          </EmptyContent>
        </Empty>
      </main>

      <footer className="not-found-footer home-container">
        <p>© {new Date().getFullYear()} ALBATEXTUS</p>
        <p>每一次连接，都有归处。</p>
        <nav aria-label="法律信息"><Link href="/wiki/privacy">隐私政策</Link><Link href="/wiki/terms">服务条款</Link></nav>
      </footer>
    </div>
  )
}
