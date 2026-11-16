import { Boxes, LibraryBig } from "lucide-react"

export const novelSections = [
  { id: "library", href: "/library", title: "小说库", icon: LibraryBig, tone: "blue" },
  { id: "resources", href: "/library/resources", title: "资源", icon: Boxes, tone: "violet" },
] as const

export function findNovelSection(id: string) {
  return novelSections.find(section => section.id === id)
}
