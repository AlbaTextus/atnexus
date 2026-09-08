"use client"

import { CalendarDays, IdCard, ShieldCheck } from "lucide-react"
import { roleNames } from "@/lib/applications"
import { useAccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { Badge } from "@/components/ui/badge"
import { MarkdownContent } from "@/components/markdown-content"
import { Card, CardContent } from "@/components/ui/card"
import { AccountIdentityCard } from "@/components/account-identity-card"

export function AccountOverview() {
  const user = useAccountUser()
  const name = user.display_name || (user.email_is_alias ? "用户" : user.email.split("@")[0])
  const joinedAt = user.created_at ? new Date(user.created_at) : null
  const validJoinedAt = joinedAt && !Number.isNaN(joinedAt.getTime())
  return <section className="account-profile-hero" aria-labelledby="account-profile-name">
    <div className="account-profile-avatar">
      <UserAvatar src={user.avatar_url} name={name} className="size-24 sm:size-28" />
    </div>
    <h1 id="account-profile-name">{name}</h1>
    <p aria-label={user.email_is_alias ? "登录标识" : "电子邮箱"}>{user.email}</p>
    <dl className="account-profile-details">
      <div className="account-profile-detail">
        <dt><span className="account-profile-detail-icon" aria-hidden="true"><IdCard /></span><span>用户 ID</span></dt>
        <dd><Badge variant="secondary">ID {user.numeric_id}</Badge></dd>
      </div>
      <div className="account-profile-detail">
        <dt><span className="account-profile-detail-icon account-profile-level-icon" aria-hidden="true"><ShieldCheck /></span><span>用户等级</span></dt>
        <dd><Badge variant="secondary">{roleNames[user.role] || `${user.level} 级`}</Badge></dd>
      </div>
      <div className="account-profile-detail">
        <dt><span className="account-profile-detail-icon account-profile-joined-icon" aria-hidden="true"><CalendarDays /></span><span>加入时间</span></dt>
        <dd><Badge variant="secondary">{validJoinedAt ? <time dateTime={user.created_at}>{joinedAt.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" })}</time> : "暂无记录"}</Badge></dd>
      </div>
    </dl>
    <section className="account-bio" aria-labelledby="account-bio-title"><h2 id="account-bio-title">个人简介</h2><Card><CardContent>{user.bio?.trim()?<MarkdownContent content={user.bio}/>:<p className="text-sm text-muted-foreground">还没有填写简介。</p>}</CardContent></Card></section>
    <AccountIdentityCard />
  </section>
}
