import { ApiError } from "@/lib/api"
import type { AccountUser } from "@/components/account-shell"

export type ManagedUser = AccountUser & { is_active: boolean; created_at: string; updated_at: string }
export type UserListResult = {
  users: ManagedUser[]
  total: number
  page: number
  page_size: number
  stats: { total: number; active: number; admins: number }
}

export function canManageUser(actor: AccountUser, target: AccountUser) {
  return actor.role === "SUPER_ADMIN" || (actor.role === "ADMIN" && target.role === "USER")
}

export function userManagementError(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 401) return "登录已过期，请重新登录。"
    if (error.status >= 500) return "暂时无法连接用户中心，请稍后重试。"
    if (error.code === "invalid_origin") return "请求未通过来源验证，请刷新页面后重试。"
    return error.message
  }
  return "网络连接失败，请检查网络后重试。"
}
