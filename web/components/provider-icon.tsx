import Image from "next/image"
import { Github, Globe } from "lucide-react"
import { cn } from "@/lib/utils"
import linuxdoIcon from "@/public/providers/linuxdo.png"

export function ProviderIcon({ provider, className, "data-icon": dataIcon }: {
  provider: string
  className?: string
  "data-icon"?: string
}) {
  const iconClassName = cn("size-4 shrink-0", className)

  if (provider === "linuxdo") {
    return <Image src={linuxdoIcon} alt="" width={20} height={20} className={cn(iconClassName, "object-contain")} data-icon={dataIcon} aria-hidden="true" />
  }

  const Icon = provider === "github" ? Github : Globe
  return <Icon className={iconClassName} data-icon={dataIcon} aria-hidden="true" />
}
