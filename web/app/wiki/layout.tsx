import type { ReactNode } from "react"
import Link from "next/link"
import { BookOpen, UserRound } from "lucide-react"
import { Layout, Navbar } from "nextra-theme-docs"
import { SiteFooter } from "@/components/site-footer"
import { normalizePageMap } from "nextra/page-map"
import { WikiSearch } from "@/components/wiki-search"
import { ThemeToggle } from "@/components/theme-toggle"
import { AnnouncementCenter } from "@/components/announcement-center"
import { WikiBrand, WikiHeader } from "@/components/wiki-header"
import "./wiki.css"

const pageMap = normalizePageMap([{ data: {
  index: "欢迎来到 ALBATEXTUS",
  overview: { type: "separator", title: "社区概览" },
  community: { title: "ALBATEXTUS 社区介绍", theme: { collapsed: false } },
  services: { type: "separator", title: "服务导航" },
  integration: "接入指南",
  policies: { type: "separator", title: "政策与协议" },
  privacy: "隐私政策",
  terms: "用户协议",
} },
  { name: "index", route: "/wiki", frontMatter: { title: "欢迎来到 ALBATEXTUS" } },
  { name: "community", route: "/wiki/community", children: [
    { data: { index: "社区介绍", levels: "等级介绍" } },
    { name: "index", route: "/wiki/community", frontMatter: { title: "社区介绍" } },
    { name: "levels", route: "/wiki/community/levels", frontMatter: { title: "等级介绍" } },
  ] },
  { name: "integration", route: "/wiki/integration", frontMatter: { title: "接入指南" } },
  { name: "privacy", route: "/wiki/privacy", frontMatter: { title: "隐私政策" } },
  { name: "terms", route: "/wiki/terms", frontMatter: { title: "用户协议" } }])

export default function WikiLayout({ children }: { children: ReactNode }) {
  return <div className="wiki-nextra">
    <Layout pageMap={pageMap}
      navbar={<WikiHeader><Navbar logoLink={false} logo={<WikiBrand />} className="home-header-inner wiki-navbar-inner"><AnnouncementCenter /><ThemeToggle /><Link href="/wiki" className="wiki-account-link size-9 justify-center" aria-label="Wiki" title="Wiki"><BookOpen size={20} aria-hidden="true" /></Link><Link href="/account" className="wiki-account-link size-9 justify-center" aria-label="用户中心" title="用户中心"><UserRound size={20} aria-hidden="true" /></Link></Navbar></WikiHeader>}
      search={<WikiSearch />} footer={<SiteFooter />}
      darkMode={false} nextThemes={{ attribute: "class", storageKey: "albatextus-color-mode", defaultTheme: "light" }}
      editLink={null} feedback={{ content: null }}
      sidebar={{ defaultMenuCollapseLevel: 2 }} toc={{ title: "在此页中", backToTop: "返回顶部" }}>
      {children}
    </Layout>
  </div>
}
