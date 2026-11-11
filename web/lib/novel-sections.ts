import { GitFork, House, Images, Settings2 } from "lucide-react"

export const novelSections = [
  { id: "library", href: "/library", title: "小说库", icon: House },
  { id: "categories", href: "/library/categories", title: "分类", icon: Images },
  { id: "sources", href: "/library/sources", title: "数据来源", icon: GitFork },
  { id: "settings", href: "/library/settings", title: "设置", icon: Settings2 },
] as const

export function findNovelSection(id: string) {
  return novelSections.find(section => section.id === id)
}
