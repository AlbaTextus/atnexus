"use client"

import Link from "next/link"
import { SiteFooter as SharedSiteFooter } from "@/components/presentation/site-footer"

export function SiteFooter() {
  return <SharedSiteFooter LinkComponent={Link} />
}
