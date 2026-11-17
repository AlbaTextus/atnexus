import { AuthLayout as SharedAuthLayout, type AuthLayoutProps } from "@/components/presentation/auth-layout"
import { ThemeToggle } from "@/components/layout/theme-toggle"

export function AuthLayout(props: Omit<AuthLayoutProps, "toolbar">) {
  return <SharedAuthLayout {...props} toolbar={<ThemeToggle />} />
}
