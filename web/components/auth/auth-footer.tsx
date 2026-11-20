import Link from "next/link"
import { safeReturnTo } from "@/lib/api"
import { Button } from "@/components/ui/button"

function authHref(path: "/login" | "/register", returnTo?: string | null) {
  const query = new URLSearchParams()
  if (returnTo) query.set("return_to", safeReturnTo(returnTo))
  return path + (query.size ? `?${query}` : "")
}

export function AuthFooter({ returnTo, from = "login", registrationEnabled = true }: { returnTo?: string | null; from?: "login" | "register"; registrationEnabled?: boolean }) {
  const toLogin = from === "register"
  // The row keeps its height without a link, so the card does not move once
  // the login page learns that registration is open.
  return <div className="flex min-h-7 w-full items-center justify-center">
    {(toLogin || registrationEnabled) && <nav aria-label="账户服务">
      <Button variant="link" size="sm" className="px-1" nativeButton={false} role="link" render={<Link href={authHref(toLogin ? "/login" : "/register", returnTo)} />}>{toLogin ? "返回登录" : "创建用户"}</Button>
    </nav>}
  </div>
}
