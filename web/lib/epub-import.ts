import { unzip, type Unzipped } from "fflate"

export type EpubVolume = { order: number; title: string }
export type EpubChapter = { title: string; content: string; volumeOrder: number | null }
export type EpubCover = { file: File | null; url: string | null; path: string }
export type EpubImport = {
  title: string; author: string; summary: string; publisher: string; source: string
  volumes: EpubVolume[]; chapters: EpubChapter[]; cover: EpubCover | null
}

const MiB = 1024 * 1024
export const MAX_EPUB_BYTES = 32 * MiB
export const MAX_CHAPTERS = 1000
export const MAX_VOLUMES = 200
export const MAX_COVER_BYTES = 5 * MiB
const MAX_TEXT_BYTES = 8 * MiB
type Target = { path: string; fragment: string; title: string; volumeOrder: number | null; isVolume: boolean }

const elements = (root: Document | Element, name: string) => Array.from(root.getElementsByTagNameNS("*", name))
const clean = (value: string) => value.replace(/\0/g, "").replace(/[\t \u00a0]+/g, " ").trim()
const normalized = (value: string) => value.replace(/\s/g, "")
const volumeTitle = (title: string) => /^(?:第[〇零一二三四五六七八九十百千万两\d]+卷(?:\s|$|[：:·、])|(?:volume|book)\s+[\divxlcdm]+\b)/i.test(title)
const children = (root: Element, name: string) => Array.from(root.children).filter(child => child.localName === name)
function externalURL(value: string): string | null {
  try {
    const url = new URL(value)
    return ["http:", "https:"].includes(url.protocol) && !url.username && !url.password ? url.href : null
  } catch { return null }
}

function xml(text: string, label: string, repairTextEntities = false): Document {
  const parser = new DOMParser()
  let document = parser.parseFromString(text.replace(/&nbsp;/g, "&#160;"), "application/xml")
  if (repairTextEntities && document.getElementsByTagName("parsererror").length) {
    // Some exporters leave literal "&" or HTML entities in XHTML prose.
    // Repair text entities only; malformed document structure must still fail.
    const decoder = globalThis.document.createElement("textarea")
    const normalized = text.replace(/<!\[CDATA\[[\s\S]*?\]\]>|<!--[\s\S]*?-->|&(?:#[xX][\da-fA-F]+;|#\d+;|[a-zA-Z][a-zA-Z\d]*;)?/g, token => {
      if (!token.startsWith("&")) return token
      if (/^&(?:amp|lt|gt|quot|apos|#\d+|#[xX][\da-fA-F]+);$/.test(token)) return token
      decoder.innerHTML = token
      const decoded = decoder.value
      return decoded === token ? token.replace("&", "&amp;")
        : Array.from(decoded, character => `&#${character.codePointAt(0)};`).join("")
    })
    document = parser.parseFromString(normalized, "application/xml")
  }
  if (document.getElementsByTagName("parsererror").length) throw new Error(`${label}格式损坏，无法解析 EPUB。`)
  return document
}

// Resolve archive-relative references without allowing remote URLs or escaping the ZIP root.
function target(base: string, href: string): Pick<Target, "path" | "fragment"> {
  if (/^[a-z][a-z\d+.-]*:|^[/\\]{2}/i.test(href)) throw new Error("EPUB 包含无效的外部章节地址。")
  const [relative, fragment = ""] = href.split("#", 2)
  const path = relative ? [...base.split("/").slice(0, -1)] : base.split("/")
  if (relative) {
    for (const part of decodeURIComponent(relative.split("?")[0]).split("/")) {
      if (part === "..") {
        if (!path.length) throw new Error("EPUB 章节路径无效。")
        path.pop()
      } else if (part && part !== ".") path.push(part)
    }
  }
  return { path: path.join("/"), fragment: decodeURIComponent(fragment) }
}

function plainText(root: Node): string {
  const blocks = new Set(["p", "div", "section", "article", "h1", "h2", "h3", "h4", "h5", "h6", "li", "blockquote", "pre", "tr"])
  const walk = (node: Node): string => {
    if (node.nodeType === Node.TEXT_NODE || node.nodeType === Node.CDATA_SECTION_NODE) return node.textContent || ""
    const name = node.nodeName.toLowerCase().split(":").pop()!
    if (["script", "style", "noscript", "nav", "svg", "iframe", "object", "template"].includes(name)) return ""
    if (name === "br" || name === "hr") return "\n"
    const text = Array.from(node.childNodes).map(walk).join("")
    return blocks.has(name) ? `\n${text}\n` : text
  }
  return walk(root).split(/\r?\n/).map(clean).join("\n").replace(/\n{3,}/g, "\n\n").trim()
}

async function extract(bytes: Uint8Array, imagePath?: string): Promise<Unzipped> {
  let total = 0
  let count = 0
  return new Promise((resolve, reject) => {
    unzip(bytes, {
      filter: entry => {
        if (++count > 10000) throw new Error("EPUB 内的文件数量过多。")
        if (imagePath) {
          if (entry.name !== imagePath) return false
          if (entry.originalSize > MAX_COVER_BYTES) throw new Error("EPUB 封面超过 5 MiB。")
          return true
        }
        // Extract the selected cover separately, without inflating unrelated images or fonts.
        if (!/\.(xml|opf|ncx|xhtml|html|htm)$/i.test(entry.name)) return false
        total += entry.originalSize
        if (entry.originalSize > MAX_TEXT_BYTES || total > 32 * MiB) throw new Error("EPUB 解压后的文本过大，请拆分文件后导入。")
        return true
      },
    }, (error, files) => error ? reject(error) : resolve(files))
  })
}

/** EPUB 2/3: container -> package -> spine; NAV/NCX anchors split shared XHTML files. */
export async function parseEpub(file: File): Promise<EpubImport> {
  if (!/\.epub$/i.test(file.name)) throw new Error("请选择 .epub 文件。")
  if (!file.size || file.size > MAX_EPUB_BYTES) throw new Error("请选择不超过 32 MiB 的 EPUB 文件。")
  let files: Unzipped
  const archive = new Uint8Array(await file.arrayBuffer())
  try {
    files = await extract(archive)
  } catch (error) {
    if (error instanceof Error && error.message.startsWith("EPUB")) throw error
    throw new Error("无法解压 EPUB，请确认文件完整且未加密。")
  }
  const read = (path: string) => {
    if (!files[path]) throw new Error(`EPUB 缺少文件：${path}`)
    const bytes = files[path]
    const encoding = bytes[0] === 0xff && bytes[1] === 0xfe ? "utf-16le"
      : bytes[0] === 0xfe && bytes[1] === 0xff ? "utf-16be"
        : new TextDecoder().decode(bytes.subarray(0, 200)).match(/encoding=["']([^"']+)/i)?.[1] || "utf-8"
    try { return new TextDecoder(encoding, { fatal: true }).decode(bytes) }
    catch { throw new Error(`EPUB 文件编码无效：${path}`) }
  }
  const container = xml(read("META-INF/container.xml"), "EPUB 容器")
  const root = elements(container, "rootfile").find(item => item.getAttribute("media-type") === "application/oebps-package+xml")
    || elements(container, "rootfile")[0]
  const packagePath = target("", root?.getAttribute("full-path") || "").path
  if (!packagePath) throw new Error("EPUB 缺少书籍描述文件。")
  const book = xml(read(packagePath), "书籍描述")
  const metadata = elements(book, "metadata")[0]
  const value = (name: string) => clean(metadata ? elements(metadata, name)[0]?.textContent || "" : "")
  const manifest = new Map(elements(book, "item").map(item => [item.getAttribute("id"), {
    ...(externalURL(item.getAttribute("href") || "") ? { path: externalURL(item.getAttribute("href") || "")!, fragment: "" } : target(packagePath, item.getAttribute("href") || "")),
    type: item.getAttribute("media-type"),
    properties: (item.getAttribute("properties") || "").split(/\s+/),
  }]))
  const spine = elements(book, "spine")[0]
  if (!spine) throw new Error("EPUB 缺少阅读顺序。")
  const targets: Target[] = []
  const volumes: EpubVolume[] = []
  function addVolume(title: string): number {
    if (volumes.length >= MAX_VOLUMES) throw new Error("EPUB 超过 200 卷，请拆分文件后导入。")
    const order = volumes.length + 1
    volumes.push({ order, title })
    return order
  }
  const nav = [...manifest.values()].find(item => item.properties.includes("nav"))
  if (nav) {
    const document = xml(read(nav.path), `EPUB 目录（${nav.path}）`, true)
    const toc = elements(document, "nav").find(item =>
      (item.getAttributeNS("http://www.idpf.org/2007/ops", "type") || item.getAttribute("epub:type") || "").split(/\s+/).includes("toc"))
    const walk = (list: Element, parentVolume: number | null) => {
      for (const entry of children(list, "li")) {
        const label = Array.from(entry.children).find(child => ["a", "span"].includes(child.localName))
        const nested = children(entry, "ol")
        const title = clean(label?.textContent || "")
        const isVolume = Boolean(title && nested.length)
        const volumeOrder = isVolume ? addVolume(title) : parentVolume
        const href = label?.getAttribute("href")
        if (href) targets.push({ ...target(nav.path, href), title, volumeOrder, isVolume })
        for (const child of nested) walk(child, volumeOrder)
      }
    }
    if (toc) for (const list of children(toc, "ol")) walk(list, null)
  }
  if (!targets.length) {
    const ncx = manifest.get(spine.getAttribute("toc")) || [...manifest.values()].find(item => item.type === "application/x-dtbncx+xml")
    if (ncx) {
      const walk = (parent: Element, parentVolume: number | null) => {
        for (const point of children(parent, "navPoint")) {
          const title = clean(children(point, "navLabel")[0]?.textContent || "")
          const isVolume = Boolean(title && children(point, "navPoint").length)
          const volumeOrder = isVolume ? addVolume(title) : parentVolume
          const href = children(point, "content")[0]?.getAttribute("src")
          if (href) targets.push({ ...target(ncx.path, href), title, volumeOrder, isVolume })
          walk(point, volumeOrder)
        }
      }
      const navMap = elements(xml(read(ncx.path), "EPUB 目录"), "navMap")[0]
      if (navMap) walk(navMap, null)
    }
  }
  if (!volumes.length) {
    let volumeOrder: number | null = null
    for (const link of targets) {
      if (volumeTitle(link.title)) { volumeOrder = addVolume(link.title); link.isVolume = true }
      link.volumeOrder = volumeOrder
    }
  }

  const coverMeta = metadata && elements(metadata, "meta").find(item => item.getAttribute("name") === "cover")?.getAttribute("content")
  let coverItem = [...manifest.values()].find(item => item.properties.includes("cover-image")) || manifest.get(coverMeta || "")
  const coverGuide = elements(book, "reference").find(item => item.getAttribute("type")?.split(/\s+/).includes("cover"))?.getAttribute("href")
  let coverPage = coverGuide ? target(packagePath, coverGuide).path : targets.find(item => /^(?:封面|cover)$/i.test(item.title))?.path
  if (!coverItem && coverPage) coverItem = [...manifest.values()].find(item => item.path === coverPage)
  if (coverItem?.type?.includes("html")) coverPage = coverItem.path
  if ((!coverItem || coverItem.type?.includes("html")) && coverPage && files[coverPage]) {
    const document = xml(read(coverPage), `封面（${coverPage}）`, true)
    const image = elements(document, "img")[0] || elements(document, "image")[0]
    const href = image?.getAttribute("src") || image?.getAttribute("href") || image?.getAttributeNS("http://www.w3.org/1999/xlink", "href")
    if (href) {
      const path = externalURL(href) || target(coverPage, href).path
      coverItem = [...manifest.values()].find(item => item.path === path) || {
        path, fragment: "", type: path.match(/\.png$/i) ? "image/png" : path.match(/\.webp$/i) ? "image/webp" : "image/jpeg", properties: [],
      }
    }
  }
  let cover: EpubCover | null = null
  if (coverItem) {
    const url = externalURL(coverItem.path)
    if (url) cover = { file: null, url, path: coverItem.path }
    else if (["image/jpeg", "image/png", "image/webp", "image/gif"].includes(coverItem.type || "")) {
      const image = (await extract(archive, coverItem.path))[coverItem.path]
      if (!image?.length) throw new Error(`EPUB 缺少封面文件：${coverItem.path}`)
      cover = { file: new File([new Uint8Array(image)], coverItem.path.split("/").pop() || "cover.jpg", { type: coverItem.type! }), url: null, path: coverItem.path }
    }
  }

  const chapters: EpubChapter[] = []
  const visited = new Set<string>()
  let textBytes = 0
  let currentVolume: number | null = null
  for (const reference of elements(spine, "itemref")) {
    if (reference.getAttribute("linear") === "no") continue
    const item = manifest.get(reference.getAttribute("idref"))
    if (!item) throw new Error("EPUB 阅读顺序引用了不存在的章节。")
    if (item.properties.includes("nav") || visited.has(item.path) || cover && item.path === coverPage) continue
    if (!["application/xhtml+xml", "text/html"].includes(item.type || "")) {
      throw new Error("EPUB 包含不支持的正文格式，请使用文字版 EPUB。")
    }
    visited.add(item.path)
    const document = xml(read(item.path), `章节（${item.path}）`, true)
    const body = elements(document, "body")[0]
    if (!body) throw new Error("EPUB 章节缺少正文。")
    // Never attach imported markup to the page; only return plain strings.
    for (const element of Array.from(body.querySelectorAll("script, style, noscript, nav, svg, iframe, object, template"))) element.remove()
    const links = targets.filter(link => link.path === item.path)
    if (links.length) currentVolume = links[0].volumeOrder
    const boundaries: { node: Element; title: string; volumeOrder: number | null; isVolume: boolean }[] = []
    for (const link of links) {
      if (!link.fragment) continue
      const anchor = Array.from(body.querySelectorAll("[id], [name]")).find(element =>
        element.getAttribute("id") === link.fragment || element.getAttribute("name") === link.fragment)
      if (!anchor) continue
      const node = anchor.closest("h1, h2, h3, h4, h5, h6") || anchor
      if (!boundaries.some(boundary => boundary.node === node)) boundaries.push({ node, title: link.title, volumeOrder: link.volumeOrder, isVolume: link.isVolume })
    }
    if (!boundaries.length) {
      for (const level of ["h1", "h2", "h3"]) {
        const headings = Array.from(body.querySelectorAll(level))
        if (headings.length > 1) {
          boundaries.push(...headings.map(node => ({ node, title: clean(node.textContent || ""), volumeOrder: currentVolume, isVolume: false })))
          break
        }
      }
    }
    boundaries.sort((a, b) => a.node.compareDocumentPosition(b.node) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1)
    const fallback = links[0]?.title || clean(elements(body, "h1")[0]?.textContent || elements(body, "h2")[0]?.textContent
      || elements(document, "title")[0]?.textContent || "") || `第 ${chapters.length + 1} 章`
    // Include any text before the first anchor, so front matter is never silently discarded.
    for (let index = -1; index < boundaries.length; index++) {
      const range = document.createRange()
      range.selectNodeContents(body)
      if (index >= 0) range.setStartBefore(boundaries[index].node)
      if (index + 1 < boundaries.length) range.setEndBefore(boundaries[index + 1].node)
      const content = plainText(range.cloneContents())
      if (!content) continue
      const title = (index >= 0 ? boundaries[index].title : "") || fallback
      const boundary = index >= 0 ? boundaries[index] : links[0]
      if (boundary) currentVolume = boundary.volumeOrder
      // A volume's heading-only XHTML is a catalog container, not a text chapter.
      if (boundary?.isVolume && normalized(content) === normalized(title)) continue
      if (!targets.length && volumeTitle(title) && normalized(content) === normalized(title)) {
        currentVolume = addVolume(title)
        continue
      }
      textBytes += new TextEncoder().encode(JSON.stringify({ title, content })).length
      if (textBytes > MAX_TEXT_BYTES) throw new Error("拆章后的内容超过 8 MiB，请拆分 EPUB 后导入。")
      chapters.push({ title, content, volumeOrder: currentVolume })
      if (chapters.length > MAX_CHAPTERS) throw new Error("EPUB 超过 1000 章，请拆分文件后导入。")
    }
  }
  if (!chapters.length) throw new Error("EPUB 中没有可导入的文字正文，暂不支持纯图片或加密书籍。")
  const description = value("description")
  return {
    title: value("title") || file.name.replace(/\.epub$/i, ""),
    author: value("creator"),
    publisher: value("publisher"),
    source: value("source"),
    summary: description.includes("<") ? plainText(new DOMParser().parseFromString(description, "text/html").body) : description,
    volumes,
    cover,
    chapters,
  }
}
