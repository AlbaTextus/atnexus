"use client"
import { useRouter as useNextRouter } from "next/navigation"
import { feedback } from "@/lib/feedback"
export { usePathname } from "next/navigation"

export function previewDestination(value: string): string | null {
  if (value === "/account/wallet" || value.startsWith("/account/wallet?")) return value
  if (value.startsWith("/settings")) return "/account/security"
  if (value.startsWith("/docs")) return "/wiki/integration"
  if (value === "/account") return value
  return null
}
export function previewUnavailable() { feedback.error("模拟数据不支持此操作") }
export function useRouter() {
  const router = useNextRouter()
  return { ...router, push: (href: string) => {
    const destination = previewDestination(href)
    if (destination) router.push(destination)
    else previewUnavailable()
  } }
}
