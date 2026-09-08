"use client"

import { useState } from "react"
import { ChevronRight, KeyRound, LogOut, Settings2 } from "lucide-react"
import { useAccountUser } from "@/components/account-shell"
import { UserAvatar } from "@/components/user-avatar"
import { UserManagementDialog, type UserAction } from "@/components/user-management-dialog"
import { roleNames } from "@/lib/applications"
import { canManageUser, type ManagedUser } from "@/lib/users"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"

export function UserDetailsRow({ user, onSaved }: { user: ManagedUser; onSaved: (message: string) => void }) {
  const actor = useAccountUser()
  const [open, setOpen] = useState(false)
  const [action, setAction] = useState<UserAction | null>(null)
  const name = user.display_name || (user.email_is_alias ? "第三方登录用户" : user.email.split("@")[0])
  const self = actor.id === user.id
  const manageable = canManageUser(actor, user)
  const details = [
    { label: "用户 ID", value: String(user.numeric_id) },
    { label: "用户等级", value: roleNames[user.role] || user.role },
    { label: user.email_is_alias ? "登录标识" : "电子邮箱", value: user.email },
    { label: "加入日期", value: new Date(user.created_at).toLocaleDateString("zh-CN") },
    { label: "密码状态", value: user.has_password ? "已设置密码" : "第三方登录 / 未设置密码" },
    { label: "内部编号", value: user.id },
  ]
  return <Dialog open={open} onOpenChange={value => { if (!action) setOpen(value) }}>
    <DialogTrigger className="managed-user-row" aria-label={`查看 ${name} 的详细信息`}>
      <UserAvatar src={user.avatar_url} name={name} className="size-11" decorative />
      <span className="managed-user-summary"><span className="managed-user-name">{name}</span><span className="managed-user-id">ID {user.numeric_id}{self ? " · 当前账户" : ""}</span></span>
      <Badge variant="outline" aria-label={`等级 ${user.level}`}>LT{user.level}</Badge>
      <Badge variant={user.is_active ? "success" : "secondary"}>{user.is_active ? "已启用" : "已停用"}</Badge>
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </DialogTrigger>
    <DialogContent className="center-dialog max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-lg">
      <DialogHeader><DialogTitle>用户详情</DialogTitle><DialogDescription>查看账户信息并管理登录权限。</DialogDescription></DialogHeader>
      <div className="flex min-w-0 items-center gap-4"><UserAvatar src={user.avatar_url} name={name} className="size-16" /><div className="min-w-0"><p className="break-all text-lg font-semibold">{name}</p><div className="mt-2 flex flex-wrap gap-2"><Badge variant={user.is_active ? "success" : "secondary"}>{user.is_active ? "已启用" : "已停用"}</Badge>{self && <Badge variant="outline">当前账户</Badge>}</div></div></div>
      <dl className="managed-user-details">{details.map(detail => <div key={detail.label}><dt>{detail.label}</dt><dd>{detail.value}</dd></div>)}</dl>
      <DialogFooter className="flex-wrap">
        {manageable ? <><Button variant="outline" onClick={() => setAction({ kind: "edit", user })}><Settings2 data-icon="inline-start" />编辑</Button>{!self && <><Button variant="outline" onClick={() => setAction({ kind: "password", user })}><KeyRound data-icon="inline-start" />重置密码</Button><Button variant="outline" onClick={() => setAction({ kind: "revoke", user })}><LogOut data-icon="inline-start" />撤销登录</Button></>}</> : <p className="text-xs text-muted-foreground">{self ? "个人资料与密码请在用户中心修改" : "由超级管理员管理"}</p>}
      </DialogFooter>
      {action && <UserManagementDialog action={action} onClose={() => setAction(null)} onSaved={message => { setAction(null); setOpen(false); onSaved(message) }} />}
    </DialogContent>
  </Dialog>
}
