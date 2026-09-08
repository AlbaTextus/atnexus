import type { ReactNode } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import remarkParse from "remark-parse"
import { unified } from "unified"
import GithubSlugger from "github-slugger"
import { useMDXComponents } from "nextra-theme-docs"
import { cn } from "@/lib/utils"

const theme = useMDXComponents()
const Wrapper = theme.wrapper

type Node = { type: string; value?: string; depth?: number; children?: Node[]; data?: Record<string, unknown> }
function nodeText(node: Node): string { return node.value || node.children?.map(nodeText).join("") || "" }
function headings(tree: Node) {
  const slugger = new GithubSlugger()
  const toc: { depth: 2 | 3 | 4 | 5 | 6; value: string; id: string }[] = []
  function walk(node: Node) {
    if (node.type === "heading") {
      const value = nodeText(node), id = slugger.slug(value)
      node.data = { ...node.data, hProperties: { id } }
      toc.push({ depth: Math.max(2, node.depth || 2) as 2 | 3 | 4 | 5 | 6, value, id })
    }
    node.children?.forEach(walk)
  }
  walk(tree)
  return toc
}
function headingIds() { return (tree: Node) => { headings(tree) } }

export function WikiDocument({ title, content, description, updatedAt, children }: { title: string; content: string; description?: string; updatedAt?: string; children?: ReactNode }) {
  const tree = unified().use(remarkParse).use(remarkGfm).parse(content)
  const toc = headings(tree as Node)
  return <Wrapper toc={toc} metadata={{ title, filePath: "" }}>
    <theme.h1>{title}</theme.h1>
    {description && <p className="wiki-description">{description}</p>}
    {updatedAt && <p className="wiki-updated">最近更新：{new Date(updatedAt).toLocaleDateString("zh-CN")}</p>}
    <Markdown skipHtml remarkPlugins={[remarkGfm, headingIds]} components={{
      h1: theme.h2, h2: theme.h2, h3: theme.h3, h4: theme.h4, h5: theme.h5, h6: theme.h6,
      p: theme.p, ul: theme.ul, ol: theme.ol, li: theme.li, blockquote: theme.blockquote, hr: theme.hr,
      code: ({ children, className }) => <code className={cn("wiki-code", className)}>{children}</code>,
      strong: ({ children }) => <strong className="wiki-term">{children}</strong>,
      a: ({ href, children }) => <theme.a href={href}>{children}</theme.a>,
      img: ({ src, alt }) => typeof src === "string" && /^(https:\/\/|\/(?!\/))/.test(src) ? <img src={src} alt={alt || ""} loading="lazy" referrerPolicy="no-referrer" /> : <span>{alt}</span>,
      table: ({ children }) => <div className="wiki-table"><table>{children}</table></div>,
    }}>{content}</Markdown>
    {children}
  </Wrapper>
}
