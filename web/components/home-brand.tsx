"use client"

import Link from "next/link"
import { Link as TransitionLink } from "next-view-transitions"
import { BrandWordmark } from "./brand-wordmark"

export function HomeBrand({ footer = false, collapsed = false, href }: { footer?: boolean; collapsed?: boolean; href?: string }) {
  const BrandLink = href === "/" ? TransitionLink : Link
  return <BrandWordmark href={href ?? (footer ? "/" : "#top")} footer={footer} collapsed={collapsed} LinkComponent={BrandLink} />
}
