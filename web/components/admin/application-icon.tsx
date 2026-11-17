"use client"

import { useState } from "react"
import { Monitor } from "lucide-react"

export function ApplicationIcon({ url, name }: { url?: string; name: string }) {
  const [failedURL, setFailedURL] = useState("")
  return <span className="application-icon">{url && url !== failedURL ? <img src={url} alt={`${name} 图标`} loading="lazy" referrerPolicy="no-referrer" onError={() => setFailedURL(url)} /> : <Monitor size={23} aria-hidden="true" />}</span>
}
