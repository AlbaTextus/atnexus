"use client"

import Link from "next/link"
import { BrandWordmark } from "@albatextus/ui/components/brand-wordmark"

export function AuthWordmark({ verified }: { verified: boolean }) {
  return <BrandWordmark variant="auth" collapsed={verified} LinkComponent={Link} />
}
