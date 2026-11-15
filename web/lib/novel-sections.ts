import { Boxes, FilePlus2, LibraryBig } from "lucide-react"

export const novelSections = [
  { id: "library", href: "/library", title: "小说库", icon: LibraryBig, tone: "blue" },
  { id: "resources", href: "/library/resources", title: "资源", icon: Boxes, tone: "violet" },
  { id: "upload", href: "/library/upload", title: "提交小说", icon: FilePlus2, tone: "pink" },
] as const

export function findNovelSection(id: string) {
  return novelSections.find(section => section.id === id)
}
