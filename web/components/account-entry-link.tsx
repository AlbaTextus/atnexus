import type { ReactElement } from "react"
import { UserRound } from "lucide-react"
import { Button } from "@/components/ui/button"

export function AccountEntryLink({ render = <a href="/account" /> }: { render?: ReactElement }) {
  return <Button variant="ghost" size="icon" nativeButton={false} role="link" render={render} aria-label="前往用户中心" title="前往用户中心"><UserRound aria-hidden="true" /></Button>
}
