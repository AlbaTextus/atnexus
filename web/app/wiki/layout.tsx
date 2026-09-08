import type { ReactNode } from "react"
import Link from "next/link"
import { Footer, Layout, Navbar } from "nextra-theme-docs"
import { normalizePageMap } from "nextra/page-map"
import { WikiSearch } from "@/components/wiki-search"
import { WikiBrand, WikiHeader } from "@/components/wiki-header"
import "./wiki.css"

const pageMap = normalizePageMap([{ data: {
  index: "欢迎来到 AlbaTextus",
  overview: { type: "separator", title: "社区概览" },
  community: { title: "AlbaTextus 社区介绍", theme: { collapsed: false } },
  services: { type: "separator", title: "服务导航" },
  integration: "接入指南",
  policies: { type: "separator", title: "政策与协议" },
  privacy: "隐私政策",
  terms: "用户协议",
} },
  { name: "index", route: "/wiki", frontMatter: { title: "欢迎来到 AlbaTextus" } },
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
      navbar={<WikiHeader><Navbar logoLink={false} logo={<WikiBrand />} className="home-header-inner wiki-navbar-inner"><Link href="/wiki" className="wiki-account-link">Wiki</Link><Link href="/account" className="wiki-account-link">用户中心 ↗</Link></Navbar></WikiHeader>}
      search={<WikiSearch />} footer={<Footer><span>© {new Date().getFullYear()} AlbaTextus · Wiki</span></Footer>}
      darkMode={false} nextThemes={{ forcedTheme: "light", attribute: "data-wiki-theme" }}
      editLink={null} feedback={{ content: null }}
      sidebar={{ defaultMenuCollapseLevel: 2 }} toc={{ title: "在此页中", backToTop: "返回顶部" }}>
      {children}
    </Layout>
  </div>
}
