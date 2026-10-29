import { AuthLayout as SharedAuthLayout, type AuthLayoutProps } from "@albatextus/ui/components/auth-layout"
import { ThemeToggle } from "@/components/theme-toggle"
import { AnnouncementCenter } from "@/components/announcement-center"

export function AuthLayout(props: Omit<AuthLayoutProps, "toolbar">) {
  return <SharedAuthLayout {...props} toolbar={<><AnnouncementCenter /><ThemeToggle /></>} />
}
