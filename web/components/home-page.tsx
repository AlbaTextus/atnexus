"use client"

import { Link as TransitionLink } from "next-view-transitions"
import { useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import { motion, useInView } from "framer-motion"
import { Check, Code2, Fingerprint, Globe2, ShieldCheck, Sparkles, UserRound } from "lucide-react"
import { Button } from "@/components/ui/button"
import { SiteFooter } from "@/components/site-footer"
import { CloudflareNetwork } from "@/components/cloudflare-network"
import { ServiceArchitecture } from "@/components/service-architecture"
import { HomeHeader, IconButton } from "@albatextus/ui"
import { CodeHighlighted, ShikiProvider } from "@albatextus/ui/code"
import { ThemeToggle } from "@/components/theme-toggle"
import { AnnouncementCenter } from "@/components/announcement-center"
import { AccountEntryLink } from "@/components/account-entry-link"
import { wikiUrl } from "@/lib/wiki"

const authorizationExample = `GET /oauth/authorize
?client_id=…&response_type=code&…

# 已登录 · 跳转授权确认页
HTTP/1.1 302 Found
Location: https://…/consent?request=…
Cache-Control: no-store

…`

const reducedMotionQuery = "(prefers-reduced-motion: reduce)"
function subscribeReducedMotion(onChange: () => void) {
  const query = window.matchMedia(reducedMotionQuery)
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getReducedMotion = () => window.matchMedia(reducedMotionQuery).matches
const getServerReducedMotion = () => true

function ScrollReveal({ children, className, delay = 0 }: { children: ReactNode; className?: string; delay?: number }) {
  const ref = useRef<HTMLDivElement>(null)
  const inView = useInView(ref, { once: true, amount: 0.12, margin: "0px 0px -35px 0px" })
  const reducedMotion = useSyncExternalStore(subscribeReducedMotion, getReducedMotion, getServerReducedMotion)
  const [focused, setFocused] = useState(false)
  const visible = reducedMotion || inView || focused
  return <motion.div ref={ref} className={className} data-scroll-reveal data-revealed={visible} initial={false}
    animate={visible ? { opacity: 1, y: 0, scale: 1 } : { opacity: 0, y: 32, scale: 0.985 }}
    transition={{ duration: reducedMotion || focused ? 0 : 0.65, delay: visible && !reducedMotion && !focused ? delay : 0, ease: [0.22, 1, 0.36, 1] }}
    onFocusCapture={() => setFocused(true)}>{children}</motion.div>
}

export function HomePage() {
  const reducedMotion = useSyncExternalStore(subscribeReducedMotion, getReducedMotion, getServerReducedMotion)
  return <div className="home-shell home-credit-inspired" id="top">
    <a href="#home-main" className="home-skip">跳转到主要内容</a>
    <HomeHeader brandHref="#top" LinkComponent={TransitionLink} actions={<>
      <AnnouncementCenter /><ThemeToggle />
      <IconButton nativeButton={false} role="link" render={<a href={wikiUrl()} />} label="前往 Wiki">W</IconButton>
      <AccountEntryLink render={<TransitionLink href="/account" />} />
    </>} />

    <main id="home-main" tabIndex={-1}>
      <section className="landing-hero home-container" aria-labelledby="home-title">
        <ScrollReveal className="landing-copy">
          <p className="landing-eyebrow"><Sparkles size={14} aria-hidden="true" /> ALBATEXTUS ACCOUNT</p>
          <h1 id="home-title">一个账户<br /><span>让连接更简单</span></h1>
          <p className="landing-description">你的数字身份，从这里开始。<br />统一登录、管理资料、安全授权，让每一次连接都轻松自然。</p>
          <div className="landing-benefits"><span><Fingerprint aria-hidden="true" />通行密钥</span><span><Globe2 aria-hidden="true" />统一身份</span><span><ShieldCheck aria-hidden="true" />安全授权</span></div>
        </ScrollReveal>
        <motion.div className="landing-visual" initial={false} animate={reducedMotion ? {y:0} : {y:[0,-8,0]}} transition={{duration:7,repeat:Infinity,ease:"easeInOut"}} aria-label="账户功能示意">
          <div className="landing-glow" aria-hidden="true" />
          <div className="identity-card">
            <div className="identity-top"><span><Fingerprint size={24} />ALBATEXTUS</span></div>
            <div className="identity-person"><div className="identity-avatar"><UserRound size={32} /></div><div><strong>每个应用，同一个你。</strong><p>你的专属数字身份</p></div></div>
            <div className="identity-bottom"><span>ONE ACCOUNT. EVERY CONNECTION.</span><ShieldCheck size={23} /></div>
          </div>
          <div className="identity-floating identity-passkey"><span className="identity-feature-icon"><Fingerprint size={21} /></span><div><strong>轻触，即可登录</strong><small>Passkey · 无需记住密码</small></div><Check size={16} /></div>
          <div className="identity-floating identity-connect"><span className="identity-feature-icon"><ShieldCheck size={21} /></span><div><strong>授权，由你决定</strong><small>只分享你允许的信息</small></div></div>
        </motion.div>
      </section>
      <section className="landing-developer home-container" aria-labelledby="developer-title">
        <ScrollReveal className="landing-code dark">
          <div className="landing-code-bar"><span className="code-window-dots" aria-hidden="true"><i /><i /><i /></span><span className="code-window-title"><Code2 size={14} aria-hidden="true" />授权接口示例</span><span aria-hidden="true" /></div>
          <section aria-label="OAuth 授权请求与重定向响应示例">
            <ShikiProvider engine="javascript" languages={["http"]}>
              <CodeHighlighted code={authorizationExample} lang="http" variant="plain" />
            </ShikiProvider>
          </section>
        </ScrollReveal>
        <ScrollReveal className="landing-developer-copy" delay={0.12}><p className="landing-eyebrow">BUILT FOR COMMUNITY</p><h2 id="developer-title">专为社区打造</h2><p>为社区成员与应用提供统一的账户入口。从登录、个人资料到应用授权，让社区里的每一次连接都更简单。</p><ul><li><Check />一个账户，连接社区里的应用</li><li><Check />安全授权，分享的信息由你决定</li><li><Check />开放源代码，与社区一起完善</li></ul></ScrollReveal>
      </section>

      <section className="architecture-section home-container" aria-label="服务架构">
        <ScrollReveal><ServiceArchitecture /></ScrollReveal>
      </section>

      <section className="edge-section home-container" aria-labelledby="edge-title">
        <div className="edge-layout">
          <ScrollReveal className="edge-copy">
            <img className="edge-cloudflare-logo" src="/brands/cloudflare.svg" alt="Cloudflare" width="341" height="156" />
            <h2 id="edge-title">全球连接，就近响应。</h2>
            <p className="edge-description">Cloudflare 提供 DNS 解析与代理服务，<br className="edge-desktop-break" />通过全球网络，让社区的每一次连接更顺畅。</p>
          </ScrollReveal>
          <ScrollReveal className="edge-brand-reveal" delay={0.18}>
            <CloudflareNetwork />
          </ScrollReveal>
        </div>
      </section>
      <section className="agi-section home-container" aria-labelledby="agi-title">
        <ScrollReveal className="agi-heading">
          <p className="landing-eyebrow">A NEW ERA OF POSSIBILITIES</p>
          <h2 id="agi-title">欢迎来到 <span>AGI</span> 的时代</h2>
          <p>由 AGI 强力构成</p>
        </ScrollReveal>
        <div className="agi-brands">
          {[
            { name: "Claude", image: "/brands/claude.svg" },
            { name: "OpenAI", image: "/brands/openai.svg" },
            { name: "Gemini", image: "/brands/gemini.svg" },
          ].map((brand, index) => <ScrollReveal key={brand.name} delay={index * 0.12}>
            <div className="agi-brand">
              <img src={brand.image} alt={brand.name} width="64" height="64" loading="lazy" />
            </div>
          </ScrollReveal>)}
        </div>
      </section>
    </main>

    <SiteFooter />
  </div>
}
