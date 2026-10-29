import Link from "next/link"
import { AuthCard as SharedAuthCard, type AuthCardProps } from "@albatextus/ui/components/auth-card"

export function AuthCard(props: Omit<AuthCardProps, "LinkComponent" | "brandHref">) {
  return <SharedAuthCard {...props} LinkComponent={Link} />
}
