import { ApiError } from "@/lib/api"

export type Application = {
  id: string
  name: string
  owner_id: string | null
  homepage_url: string
  description: string
  icon_url: string
  minimum_role: string
  redirect_uris: string[]
  allowed_scopes: string[]
  allowed_grant_types: string[]
  is_active: boolean
  is_public: boolean
  secret_available: boolean
  created_at: string
  updated_at: string
}

export type OAuthMetadata = {
  issuer: string
  authorization_endpoint: string
  token_endpoint: string
  userinfo_endpoint: string
  revocation_endpoint: string
  introspection_endpoint: string
}

export const roleNames: Record<string, string> = { USER: "1 级 · 普通用户", LEVEL_2: "2 级 · 用户", ADMIN: "3 级 · 管理员", SUPER_ADMIN: "4 级 · 超级管理员" }
export const isAdminRole = (role: string) => role === "ADMIN" || role === "SUPER_ADMIN"

export function managementError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return "登录已过期，请重新登录。"
    if (error.code === "admin_required") return "应用接入管理仅限管理员使用。"
    if (error.code === "application_not_found") return "应用不存在，或已被删除。"
    if (error.status === 403) return "当前请求无法通过权限验证，请刷新页面后重试。"
    if (error.status >= 500) return "暂时无法连接用户中心，请稍后重试。"
    return error.message
  }
  return "网络连接失败，请检查网络后重试。"
}
