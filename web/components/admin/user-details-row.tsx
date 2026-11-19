"use client"

import { useState } from "react"
import { ChevronRight, KeyRound, LogOut, Settings2 } from "lucide-react"
import { useAccountUser } from "@/components/account/account-shell"
import { UserAvatar } from "@/components/shared/user-avatar"
import { UserIdentityCard } from "@/components/account/account-identity-card"
import { UserManagementDialog, type UserAction } from "@/components/admin/user-management-dialog"
import { canManageUser, type ManagedUser } from "@/lib/admin/users"
import { Badge } from "@/components/ui/badge"
import { UserDeviceCard } from "@/components/admin/user-device-card"
import { ManagedUserKey } from "@/components/admin/managed-user-key"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"

export function UserDetailsRow({ user, onSaved }: { user: ManagedUser; onSaved: (message: string) => void }) {
  const actor = useAccountUser()
  const [open, setOpen] = useState(false)
  const [action, setAction] = useState<UserAction | null>(null)
  const [keyLocked, setKeyLocked] = useState(false)
  const name = user.display_name || (user.email_is_alias ? "第三方登录用户" : user.email.split("@")[0])
  const self = actor.id === user.id
  const manageable = canManageUser(actor, user)
  const details = [
    { label: user.email_is_alias ? "登录标识" : "电子邮箱", value: user.email },
    { label: "密码状态", value: user.has_password ? "已设置密码" : "第三方登录 / 未设置密码" },
  ]
  return <Dialog open={open} onOpenChange={value => { if (!action && !keyLocked) setOpen(value) }}>
    <DialogTrigger className="managed-user-row" aria-label={`查看 ${name} 的详细信息`}>
      <UserAvatar src={user.avatar_url} name={name} className="size-11" decorative />
      <span className="managed-user-summary"><span className="managed-user-name">{name}</span><span className="managed-user-id">ID {user.numeric_id}{self ? " · 当前账户" : ""}</span></span>
      <Badge variant="outline" aria-label={`等级 ${user.level}`}>LT{user.level}</Badge>
      <Badge variant={user.is_active ? "success" : "secondary"}>{user.is_active ? "已启用" : "已停用"}</Badge>
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </DialogTrigger>
    <DialogContent showCloseButton={!keyLocked} className="center-dialog managed-user-dialog">
      <DialogHeader className="managed-user-dialog-heading"><DialogTitle>用户详情</DialogTitle><DialogDescription>查看账户信息、管理登录权限。</DialogDescription></DialogHeader>
      <div className="managed-user-dialog-body">
        <div className="managed-user-profile">
          <section className="managed-user-panel managed-user-identity" aria-label="身份信息"><UserIdentityCard user={user} /></section>
          <section className="managed-user-login managed-user-panel" aria-label="登录信息">
            <h3><Settings2 className="size-4" aria-hidden="true" />账户管理</h3>
            <div className="flex flex-wrap gap-2"><Badge variant={user.is_active ? "success" : "secondary"}>{user.is_active ? "已启用" : "已停用"}</Badge>{self && <Badge variant="outline">当前账户</Badge>}</div>
            <dl>{details.map(detail => <div key={detail.label}><dt>{detail.label}</dt><dd>{detail.value}</dd></div>)}</dl>
            <div className="managed-user-dialog-actions" inert={keyLocked}>
              {manageable ? <><Button variant="outline" onClick={() => setAction({ kind: "edit", user })}><Settings2 data-icon="inline-start" />编辑</Button>{!self && <><Button variant="outline" onClick={() => setAction({ kind: "password", user })}><KeyRound data-icon="inline-start" />重置密码</Button><Button variant="outline" onClick={() => setAction({ kind: "revoke", user })}><LogOut data-icon="inline-start" />撤销登录</Button></>}</> : <p className="text-xs text-muted-foreground">{self ? "个人资料与密码请在用户中心修改" : "由超级管理员管理"}</p>}
            </div>
          </section>
        </div>
        {open && manageable && !self && <ManagedUserKey user={user} onSaved={onSaved} onLockChange={setKeyLocked} />}
        <UserDeviceCard device={user.last_login_device} />
      </div>
      {action && <UserManagementDialog action={action} onClose={() => setAction(null)} onSaved={message => { setAction(null); setOpen(false); onSaved(message) }} />}
    </DialogContent>
  </Dialog>
}
