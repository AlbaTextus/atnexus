"use client"
import { feedback } from "@/lib/feedback"
import { wikiUrl } from "@/lib/wiki"

export function previewDestination(value: string): string | null {
  if (value === "/account/wallet" || value.startsWith("/account/wallet?")) return value
  if (value.startsWith("/settings")) return "/account/security"
  if (value.startsWith("/docs")) return wikiUrl("/integration")
  if (value === "/account") return value
  return null
}
export function previewUnavailable() { feedback.error("模拟数据不支持此操作") }
