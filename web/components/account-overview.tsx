"use client"

import Link from "next/link"
import { ShieldCheck, UserRound } from "lucide-react"
import { roleNames } from "@/lib/applications"
import { useAccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"

export function AccountOverview() {
  const user = useAccountUser()
  const name = user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])
  const details = [
    { label: "昵称", value: user.display_name || "未设置" },
    { label: user.email_is_alias ? "登录标识" : "电子邮箱", value: user.email },
    { label: "账户身份", value: roleNames[user.role] || "用户" },
    { label: "加入日期", value: user.created_at ? new Date(user.created_at).toLocaleDateString("zh-CN") : "暂无记录" },
  ]

  return <div className="flex flex-col gap-7">
    <div className="center-page-heading">
      <div><p className="center-eyebrow">YOUR PROFILE</p><h1>你好，{name}</h1><p>在这里查看你的个人资料与账户信息。</p></div>
    </div>
    <Card>
      <CardHeader><CardTitle><h2>个人资料</h2></CardTitle><CardDescription>你的头像、昵称和账户信息。</CardDescription></CardHeader>
      <CardContent className="flex flex-col gap-7">
        <div className="flex min-w-0 items-center gap-4">
          <UserAvatar src={user.avatar_url} name={name} className="size-16" />
          <div className="flex min-w-0 flex-col items-start gap-2"><p className="break-all text-lg font-medium">{name}</p><Badge variant="secondary">{roleNames[user.role] || "用户"}</Badge></div>
        </div>
        <dl className="grid gap-x-8 gap-y-6 sm:grid-cols-2">
          {details.map(detail => <div key={detail.label} className="flex min-w-0 flex-col gap-2"><dt className="text-sm text-muted-foreground">{detail.label}</dt><dd className="break-all text-sm font-medium">{detail.value}</dd></div>)}
        </dl>
      </CardContent>
      <CardFooter className="flex flex-wrap gap-2">
        <Button variant="outline" nativeButton={false} role="link" render={<Link href="/account/profile" />}><UserRound data-icon="inline-start" />编辑资料</Button>
        <Button variant="outline" nativeButton={false} role="link" render={<Link href="/account/security" />}><ShieldCheck data-icon="inline-start" />安全设置</Button>
      </CardFooter>
    </Card>
  </div>
}
