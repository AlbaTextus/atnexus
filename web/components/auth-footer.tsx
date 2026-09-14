import Link from "next/link"
import { safeReturnTo } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"

export function authHref(path: "/login" | "/register" | "/privacy" | "/terms", returnTo?: string | null, from: "login" | "register" = "login") {
  const query = new URLSearchParams()
  if (returnTo) query.set("return_to", safeReturnTo(returnTo))
  if ((path === "/privacy" || path === "/terms") && from === "register") query.set("from", "register")
  const destination = path === "/privacy" || path === "/terms" ? `/wiki${path}` : path
  return destination + (query.size ? `?${query}` : "")
}

export function AuthFooter({ returnTo, from = "login", registrationEnabled = true }: { returnTo?: string | null; from?: "login" | "register"; registrationEnabled?: boolean }) {
  return <nav aria-label="账户服务" className="flex w-full items-center justify-center gap-1">
    <Button variant="link" size="sm" className="min-w-0 flex-1 px-1" nativeButton={false} role="link" render={<Link href={authHref("/privacy", returnTo, from)} />}>隐私政策</Button>
    <Separator orientation="vertical" className="my-auto h-3" aria-hidden="true" />
    <Button variant="link" size="sm" className="min-w-0 flex-1 px-1" nativeButton={false} role="link" render={<Link href={authHref("/terms", returnTo, from)} />}>用户协议</Button>
    {(from === "register" || registrationEnabled) && <><Separator orientation="vertical" className="my-auto h-3" aria-hidden="true" />
    <Button variant="link" size="sm" className="min-w-0 flex-1 px-1" nativeButton={false} role="link" render={<Link href={authHref(from === "register" ? "/login" : "/register", returnTo)} />}>{from === "register" ? "返回登录" : "创建用户"}</Button></>}
  </nav>
}
