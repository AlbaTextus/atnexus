"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { ArrowDown, ArrowRight, ArrowUpRight, BookOpen, Check, Code2, Fingerprint, GitBranch, Github, Globe2, HeartHandshake, History, KeyRound, LayoutGrid, LockKeyhole, MessageCircle, Moon, ShieldCheck, Sparkles, Sun, UserPlus, UserRound } from "lucide-react"
import { api } from "@/lib/api"
import { isAdminRole } from "@/lib/applications"
import { cn } from "@/lib/utils"
import type { AccountUser } from "@/components/account-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"

const repository = "https://github.com/AlbaTextus/oauth"
const guide = [
  { number: "01", icon: UserRound, title: "拥有一个账户", description: "使用邮箱注册，或选择已开放的第三方账号登录。你的资料，在这里统一管理。" },
  { number: "02", icon: ShieldCheck, title: "选择你的应用", description: "在用户中心找到已开放的应用，确认它申请的信息，再决定是否授权。" },
  { number: "03", icon: Fingerprint, title: "让登录更从容", description: "在账户安全中添加通行密钥，下次通过指纹、面容或设备 PIN 轻松登录。" },
]

function HomeBrand({ footer = false }: { footer?: boolean }) {
  return <Link href="/" aria-label="AlbaTextus 首页" className="home-brand">
    <span className="home-brand-mark" aria-hidden="true">A<span /></span>
    <span>AlbaTextus{footer && <small>让每一次连接，都从信任开始。</small>}</span>
  </Link>
}

export function HomePage() {
  const [theme, setTheme] = useState<"dark" | "light">("dark")
  const [user, setUser] = useState<AccountUser | null>(null)
  const [activeSection, setActiveSection] = useState("top")
  const admin = Boolean(user && isAdminRole(user.role))

  useEffect(() => {
    let frame = 0
    function updateSection() {
      frame = 0
      const atBottom = window.scrollY > 0 && window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 12
      const boundary = (document.querySelector(".home-header")?.getBoundingClientRect().bottom ?? 100) + 40
      let current = "top"
      for (const id of ["home-applications", "guide", "community"]) {
        if ((document.getElementById(id)?.getBoundingClientRect().top ?? Infinity) <= boundary) current = id
      }
      // Several anchors may share the final scroll position on a tall screen.
      // Preserve the requested section when the browser clamps to the bottom.
      const hash = window.location.hash.slice(1)
      const requested = document.getElementById(hash)
      const visibleAnchor = ["guide", "community"].includes(hash) && requested && requested.getBoundingClientRect().top < window.innerHeight
      setActiveSection(atBottom ? (visibleAnchor ? hash : "community") : current)
    }
    function scheduleUpdate() { if (!frame) frame = window.requestAnimationFrame(updateSection) }
    updateSection()
    window.addEventListener("scroll", scheduleUpdate, { passive: true })
    window.addEventListener("resize", scheduleUpdate)
    window.addEventListener("hashchange", scheduleUpdate)
    return () => {
      window.removeEventListener("scroll", scheduleUpdate)
      window.removeEventListener("resize", scheduleUpdate)
      window.removeEventListener("hashchange", scheduleUpdate)
      window.cancelAnimationFrame(frame)
    }
  }, [])

  useEffect(() => {
    try {
      const saved = window.localStorage.getItem("albatextus-home-theme")
      if (saved === "light" || saved === "dark") setTheme(saved)
    } catch { /* Browsers may disable local storage. The default theme still works. */ }
    const controller = new AbortController()
    api<AccountUser>("/api/v1/me", { signal: controller.signal }).then(value => {
      if (!controller.signal.aborted) setUser(value)
    }).catch(() => { /* The public homepage also works without a session or API connection. */ })
    return () => controller.abort()
  }, [])

  function toggleTheme() {
    const next = theme === "dark" ? "light" : "dark"
    setTheme(next)
    try { window.localStorage.setItem("albatextus-home-theme", next) } catch { /* Keep the selection for this visit. */ }
  }

  return <div className="home-shell" data-theme={theme} id="top">
    <a href="#home-main" className="home-skip">跳转到主要内容</a>
    <div className="home-announcement">
      <span><span className="home-status-dot" aria-hidden="true" />你的统一账户中心，已就绪</span>
      <span className="home-announcement-divider" aria-hidden="true" />
      <a href="#guide">认识 AlbaTextus <ArrowUpRight size={13} aria-hidden="true" /></a>
    </div>

    <header className="home-header">
      <div className="home-container home-header-inner">
        <HomeBrand />
        <nav aria-label="主页导航" className="home-navigation">
          <a href="#top" className="home-nav-link" aria-current={activeSection === "top" ? "page" : undefined}>主页</a>
          <Link href={admin ? "/admin" : "/account"} className={cn("home-nav-link", admin && "home-admin-link")}>{admin ? "管理" : "账户"}</Link>
          <a href="#home-applications" className="home-nav-link" aria-current={activeSection === "home-applications" ? "location" : undefined}>应用</a>
          <a href="#guide" className="home-nav-link" aria-current={activeSection === "guide" ? "location" : undefined}>指南</a>
          <a href="#community" className="home-nav-link" aria-current={activeSection === "community" ? "location" : undefined}>社区</a>
          <a href={`${repository}/blob/main/README.md`} target="_blank" rel="noopener noreferrer" className="home-nav-link">文档</a>
        </nav>
        <div className="home-header-actions">
          <Button variant="ghost" size="icon" aria-label={theme === "dark" ? "切换为浅色主题" : "切换为深色主题"} onClick={toggleTheme}>{theme === "dark" ? <Sun /> : <Moon />}</Button>
          <Button variant="ghost" size="icon" nativeButton={false} role="link" render={<a href={repository} target="_blank" rel="noopener noreferrer" />} aria-label="GitHub 项目（新窗口）" className="home-repo-link"><Github /></Button>
          {user ? <>
            <Button nativeButton={false} role="link" render={<Link href="/account" />}>用户中心<ArrowUpRight data-icon="inline-end" /></Button>
          </> : <>
            <Button variant="ghost" nativeButton={false} role="link" render={<Link href="/login" />}>登录</Button>
            <Button nativeButton={false} role="link" render={<Link href="/register" />}>注册<ArrowUpRight data-icon="inline-end" /></Button>
          </>}
        </div>
      </div>
    </header>

    <main id="home-main" tabIndex={-1}>
      <section className="home-hero" aria-labelledby="home-title">
        <div className="home-hero-grid" aria-hidden="true" />
        <div className="home-orbit home-orbit-one" aria-hidden="true" />
        <div className="home-orbit home-orbit-two" aria-hidden="true" />
        <div className="home-container home-hero-content">
          <Badge variant="outline" className="h-8 gap-2 px-3"><Sparkles data-icon="inline-start" />ALBATEXTUS ACCOUNT</Badge>
          <h1 id="home-title">一个账户，<br /><span>连接每个应用。</span></h1>
          <p className="home-hero-description">少一点登录的繁琐，多一点连接的自由。<br />在 AlbaTextus，安全访问你的应用，轻松管理你的数字身份。</p>

          <div className="home-entries" id="home-applications">
            <Link href={user ? "/account/profile" : "/register"} className="home-entry home-entry-primary">
              <Card className="h-full gap-6 p-7" interactive>
                <CardHeader className="gap-5 px-0">
                  <div className="home-entry-top"><span className="home-entry-icon">{user ? <UserRound size={27} aria-hidden="true" /> : <UserPlus size={27} aria-hidden="true" />}</span><ArrowUpRight size={21} className="home-entry-arrow" aria-hidden="true" /></div>
                  <CardTitle><h2>{user ? "我的个人资料" : "创建你的账户"}</h2></CardTitle>
                  <CardDescription>{user ? "更新昵称、选择头像，让每个应用都认识你。" : "创建一个账户，开启你的 AlbaTextus 体验。"}</CardDescription>
                </CardHeader>
                <CardContent className="mt-auto px-0"><span className="home-entry-action">{user ? "管理个人资料" : "立即注册"}<ArrowRight size={16} aria-hidden="true" /></span></CardContent>
              </Card>
            </Link>
            <Link href="/account" className="home-entry">
              <Card className="h-full gap-6 p-7" interactive>
                <CardHeader className="gap-5 px-0">
                  <div className="home-entry-top"><span className="home-entry-icon"><LayoutGrid size={27} aria-hidden="true" /></span><ArrowUpRight size={21} className="home-entry-arrow" aria-hidden="true" /></div>
                  <CardTitle><h2>进入用户中心</h2></CardTitle>
                  <CardDescription>发现已开放的应用，管理你的账户安全。</CardDescription>
                </CardHeader>
                <CardContent className="mt-auto px-0"><span className="home-entry-action">前往用户中心<ArrowRight size={16} aria-hidden="true" /></span></CardContent>
              </Card>
            </Link>
          </div>

          <p className="home-login-hint"><LockKeyhole size={15} aria-hidden="true" />{user ? <><span>欢迎回来，{user.display_name || (user.email_is_alias ? "朋友" : user.email.split("@")[0])}</span><Link href="/account/security">查看账户安全<ArrowRight size={13} aria-hidden="true" /></Link></> : <><span>已经有账户了？</span><Link href="/login">登录，继续你的旅程<ArrowRight size={13} aria-hidden="true" /></Link></>}</p>

          <div className="home-trust" aria-label="账户服务特点">
            <span><ShieldCheck size={16} aria-hidden="true" />授权由你决定</span>
            <span><Fingerprint size={16} aria-hidden="true" />支持通行密钥</span>
            <span><Globe2 size={16} aria-hidden="true" />一个账户，多个应用</span>
          </div>
          <a href="#guide" className="home-scroll-link" aria-label="向下查看使用指南"><ArrowDown size={18} /></a>
        </div>
      </section>

      <section id="guide" className="home-guide home-container" aria-labelledby="home-guide-title">
        <div className="home-section-heading"><div><p className="home-eyebrow">简单开始，自在连接</p><h2 id="home-guide-title">从这里，找到你的下一站。</h2></div><p>把账户留在一处，<br />把时间留给你真正想做的事。</p></div>
        <div className="home-guide-grid">{guide.map(step => <article key={step.number} className="home-guide-step"><div className="home-step-top"><step.icon size={22} aria-hidden="true" /><span>{step.number}</span></div><h3>{step.title}</h3><p>{step.description}</p></article>)}</div>
        <div className="home-guide-note"><HeartHandshake size={19} aria-hidden="true" /><p>你的账户，你来掌握。<span>随时在用户中心管理资料、登录方式和账户安全。</span></p><Link href="/account/security">了解账户安全<ArrowUpRight size={16} aria-hidden="true" /></Link></div>
      </section>
    </main>

    <footer className="home-footer" id="community">
      <div className="home-container">
        <div className="home-footer-grid">
          <div className="home-footer-brand"><HomeBrand footer /><p>一个账户，连接每个应用。<br />为你的应用提供统一、安全的登录体验。</p><span className="home-footer-signature"><span className="home-status-dot" aria-hidden="true" />Designed for connection.</span></div>
          <nav aria-label="页脚导航" className="home-footer-column"><h2>导航</h2><a href="#guide">使用指南</a><Link href="/account">用户中心</Link><Link href="/privacy">隐私政策</Link><Link href="/terms">用户协议</Link></nav>
          <nav aria-label="项目与社区" className="home-footer-column"><h2>项目与社区</h2><a href={repository} target="_blank" rel="noopener noreferrer"><Github size={17} aria-hidden="true" />GitHub<ArrowUpRight size={13} aria-hidden="true" /></a><a href={`${repository}/issues`} target="_blank" rel="noopener noreferrer"><MessageCircle size={17} aria-hidden="true" />问题反馈</a><a href={`${repository}/commits/main`} target="_blank" rel="noopener noreferrer"><History size={17} aria-hidden="true" />更新记录</a><p className="home-community-note">每一个想法，<br />都能让下一次连接更好。</p></nav>
          <div className="home-footer-column"><h2>技术资源</h2><div className="home-resource-links"><Button variant="outline" size="sm" nativeButton={false} role="link" render={<a href={`${repository}/blob/main/README.md`} target="_blank" rel="noopener noreferrer" />}><BookOpen data-icon="inline-start" />项目文档</Button><Button variant="outline" size="sm" nativeButton={false} role="link" render={<a href={`${repository}/blob/main/README.md#api`} target="_blank" rel="noopener noreferrer" />}><Code2 data-icon="inline-start" />API 文档</Button><Button variant="outline" size="sm" nativeButton={false} role="link" render={<a href="/.well-known/oauth-authorization-server" target="_blank" rel="noopener noreferrer" />}><KeyRound data-icon="inline-start" />OAuth 元数据</Button><Button variant="outline" size="sm" nativeButton={false} role="link" render={<a href={repository} target="_blank" rel="noopener noreferrer" />}><GitBranch data-icon="inline-start" />源代码</Button></div><p className="home-resource-note"><Check size={13} aria-hidden="true" />OAuth 2.0 · PKCE · Passkey</p></div>
        </div>
        <Separator />
        <div className="home-footer-bottom"><p>© {new Date().getFullYear()} AlbaTextus</p><span>每个应用，同一个你。</span><a href="#top">回到顶部<ArrowUpRight size={14} aria-hidden="true" /></a></div>
      </div>
    </footer>
  </div>
}
