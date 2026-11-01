import { AuthLayout as SharedAuthLayout, type AuthLayoutProps } from "@albatextus/ui/components/auth-layout"
import { ThemeToggle } from "@/components/theme-toggle"

export function AuthLayout(props: Omit<AuthLayoutProps, "toolbar">) {
  return <SharedAuthLayout {...props} toolbar={<ThemeToggle />} />
}
