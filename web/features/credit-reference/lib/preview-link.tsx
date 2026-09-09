"use client"
import Link from "next/link"
import { forwardRef, type ComponentProps } from "react"
import { previewDestination, previewUnavailable } from "./preview-navigation"

const PreviewLink = forwardRef<HTMLAnchorElement, ComponentProps<typeof Link>>(function PreviewLink({ href, onClick, ...props }, ref) {
  const path = typeof href === "string" ? href : href.pathname || ""
  const external = /^https?:\/\//.test(path)
  const destination = external ? path : previewDestination(path)
  return <Link {...props} ref={ref} href={destination || "/account/wallet"} onClick={event => {
    onClick?.(event)
    if (!destination && !event.defaultPrevented) { event.preventDefault(); previewUnavailable() }
  }} />
})
export default PreviewLink
