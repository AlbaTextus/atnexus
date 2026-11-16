"use client"

import { Menu } from "@base-ui/react/menu"
import { Moon, SquarePen, Sun, UserRound } from "lucide-react"
import { useTheme } from "next-themes"
import { Link } from "next-view-transitions"
import type { AccountUser } from "@/components/account-shell"
import { Button } from "@/components/ui/button"
import { UserAvatar } from "@/components/user-avatar"

const itemClass = "flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2.5 text-sm outline-none select-none data-highlighted:bg-muted data-highlighted:text-foreground [&_svg]:size-4 [&_svg]:shrink-0"

export function NovelUserMenu({ user }: { user: AccountUser | null }) {
  const { setTheme } = useTheme()
  const accountHref = user ? `/account/${user.numeric_id}` : "/account"

  return <Menu.Root modal={false}>
    <Menu.Trigger render={<Button variant="ghost" size="icon" className="rounded-full" />} aria-label="用户菜单">
      {user ? <UserAvatar src={user.avatar_url} name={user.display_name || user.email} decorative />
        : <UserRound aria-hidden="true" />}
    </Menu.Trigger>
    <Menu.Portal style={{ zoom: "calc(1 / var(--site-root-scale, var(--site-scale, 1)))" }}>
      <Menu.Positioner side="bottom" align="center" sideOffset={10} collisionPadding={12} className="z-50">
        <Menu.Popup aria-label="用户菜单" style={{ zoom: "var(--site-scale, 1)" }}
          className="w-56 max-w-[calc(100vw-2rem)] origin-(--transform-origin) rounded-xl border border-border bg-popover p-1.5 text-popover-foreground shadow-lg outline-none">
          <Menu.LinkItem render={<Link href={accountHref} />} closeOnClick className={itemClass}>
            <UserRound className="text-blue-500 dark:text-blue-400" aria-hidden="true" />用户中心
          </Menu.LinkItem>
          <Menu.LinkItem render={<Link href="/creatplat" />} closeOnClick className={itemClass}>
            <SquarePen className="text-pink-500 dark:text-pink-400" aria-hidden="true" />创作者平台
          </Menu.LinkItem>
          <Menu.Item closeOnClick={false} className={`${itemClass} theme-toggle`} onClick={() => setTheme(document.documentElement.classList.contains("dark") ? "light" : "dark")}>
            <Moon className="theme-icon-light text-violet-500" aria-hidden="true" />
            <Sun className="theme-icon-dark text-amber-400" aria-hidden="true" />
            明暗调整
          </Menu.Item>
        </Menu.Popup>
      </Menu.Positioner>
    </Menu.Portal>
  </Menu.Root>
}
