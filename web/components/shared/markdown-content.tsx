import Markdown, { defaultUrlTransform } from "react-markdown"
import remarkGfm from "remark-gfm"
import { Children, isValidElement, type ReactNode } from "react"
import { MarkdownCodeBlock } from "@/components/shared/markdown-code"

function safeURL(value: string, key: string) {
  const safe = defaultUrlTransform(value)
  if (!safe || /[\u0000-\u0020\\]/.test(safe)) return ""
  if (key === "src" && !safe.startsWith("https://") && !(safe.startsWith("/") && !safe.startsWith("//"))) return ""
  return safe
}

export function MarkdownContent({ content }: { content: string }) {
  return <div className="legal-markdown">
    <Markdown remarkPlugins={[remarkGfm]} skipHtml urlTransform={safeURL} components={{
      h1: ({ children }) => <h2>{children}</h2>,
      pre: ({ children }) => {
        const code = Children.toArray(children).find(child => isValidElement(child) && child.type === "code")
        if (!isValidElement<{ className?: string; children?: ReactNode }>(code)) return <pre>{children}</pre>
        const lang = /(?:^|\s)language-(\S+)/.exec(code.props.className || "")?.[1] || "text"
        return <MarkdownCodeBlock code={String(code.props.children ?? "").replace(/\n$/, "")} lang={lang} />
      },
      a: ({ href, children }) => href ? <a href={href} target={/^(https?:)?\/\//.test(href) ? "_blank" : undefined} rel="noopener noreferrer">{children}</a> : <span>{children}</span>,
      img: ({ src, alt }) => typeof src === "string" && src ? <img src={src} alt={alt || ""} loading="lazy" decoding="async" referrerPolicy="no-referrer" /> : <span>{alt}</span>,
      table: ({ children }) => <div className="legal-table-scroll" role="region" aria-label="协议表格" tabIndex={0}><table>{children}</table></div>,
    }}>{content}</Markdown>
  </div>
}
