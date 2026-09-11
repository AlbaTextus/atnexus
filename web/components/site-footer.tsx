import Link from "next/link"
import { Github } from "lucide-react"
import { Separator } from "@/components/ui/separator"

export function SiteFooter() {
  return <footer className="home-footer" id="community">
    <div className="home-container">
      <div className="home-footer-grid">
        <div className="home-footer-brand">
          <Link href="/" className="home-footer-wordmark" aria-label="ALBATEXTUS 首页">ALBATEXTUS</Link>
          <a className="home-footer-github" href="https://github.com/AlbaTextus" target="_blank" rel="noopener noreferrer" aria-label="ALBATEXTUS GitHub（新窗口）"><Github size={18} aria-hidden="true" /></a>
        </div>
        <nav aria-label="社区项目" className="home-footer-column home-footer-projects">
          <h2>社区项目</h2>
          {["connect", "imgbed", "staus"].map(project => <a key={project} href={`https://github.com/AlbaTextus/${project}`} target="_blank" rel="noopener noreferrer">{project}</a>)}
        </nav>
      </div>
      <Separator />
      <div className="home-footer-bottom"><p>© 2026 ALBATEXTUS</p><nav className="home-footer-legal" aria-label="法律信息"><Link href="/wiki/privacy">隐私政策</Link><Link href="/wiki/terms">服务条款</Link></nav></div>
      <div className="home-footer-watermark" aria-hidden="true">ALBATEXTUS</div>
    </div>
  </footer>
}
