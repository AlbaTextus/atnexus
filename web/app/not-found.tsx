"use client"

import { useEffect } from "react"

export default function NotFound() {
  // Server redirect() throws while Next.js prepares this boundary, triggering
  // invalid performance timings even on valid routes. Load the dedicated error
  // page on mount with a fresh document, without keeping the missing URL in history.
  useEffect(() => {
    window.location.replace("/404")
  }, [])

  return null
}
