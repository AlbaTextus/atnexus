export type LegalSlug = "privacy" | "terms"
export type LegalDocument = {
  slug: LegalSlug
  title: string
  description: string
  content: string
  revision: number
  updated_at: string
}

export const legalNames: Record<LegalSlug, string> = { privacy: "隐私政策", terms: "用户协议" }
