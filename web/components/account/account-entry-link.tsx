import type { ReactElement } from "react"
import { UserRound } from "lucide-react"
import { IconButton } from "@/components/shared/icon-button"

export function AccountEntryLink({ render = <a href="/account" /> }: { render?: ReactElement }) {
  return <IconButton nativeButton={false} role="link" render={render} label="前往用户中心"><UserRound aria-hidden="true" /></IconButton>
}
