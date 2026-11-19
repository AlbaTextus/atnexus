import { ApiError } from "@/lib/api"

export type ChannelQuota = {
  enabled: boolean
  limitGB: number
  threshold: number
}

export type StorageChannel = {
  name: string
  enabled: boolean
  endpoint?: string
  bucketName?: string
  region?: string
  accessKeyId?: string
  secretAccessKey?: string
  pathStyle: boolean
  cdnDomain?: string
  savePath?: string
  quota: ChannelQuota
}

export type ChannelGroup = {
  channels: StorageChannel[]
  loadBalance: { enabled: boolean }
}

export type UploadConfig = {
  cfr2: ChannelGroup
  s3: ChannelGroup
}

export type ChannelUsage = {
  usedMB: number
  usedBytes?: number
  fileCount: number
}

export type SpaceStat = {
  key: string
  group: string
  fileCount: number
  usedBytes: number
  usedMB: number
}

export type StateStats = {
  ready: number
  pending: number
  deleting: number
  blocked: number
  activeUploadSessions: number
}

export type ImageRuntimeConfig = {
  publicURL: string
  maxUploadBytes: number
  maxChunkBytes: number
  maxChunks: number
  uploadTTLSeconds: number
  maxSessionsPerOwner: number
}

export type QuotaResponse = {
  success: boolean
  quotaStats: Record<string, ChannelUsage>
  channelStats: Record<string, ChannelUsage>
  spaceStats?: SpaceStat[]
  stateStats?: StateStats
  runtime?: ImageRuntimeConfig
  totalSizeMB: number
  totalBytes?: number
  totalCount: number
  lastUpdated: number
}

export type ImageKeyPermission = "upload" | "list" | "delete" | "manage"

export type ImageApiKey = {
  id: string
  name: string
  owner: string
  permissions: ImageKeyPermission[]
  expiresAt: string | null
  disabled: boolean
  createdAt: string
  updatedAt: string
  autoDelete: boolean
}

export type CreatedImageApiKey = {
  id: string
  name: string
  owner: string
  token: string
  permissions: ImageKeyPermission[]
  expiresAt: string | null
}

export const imageKeyPermissions: { value: ImageKeyPermission; label: string; description: string }[] = [
  { value: "upload", label: "上传 (upload)", description: "允许普通与分块上传" },
  { value: "list", label: "列表 (list)", description: "允许查询目录树与文件列表" },
  { value: "delete", label: "删除 (delete)", description: "允许删除单个或批量文件" },
  { value: "manage", label: "管理 (manage)", description: "允许修改配置、配额、标签、封禁状态与预览" },
]

export const permissionShortLabels: Record<ImageKeyPermission, string> = {
  upload: "上传",
  list: "列表",
  delete: "删除",
  manage: "管理",
}

const spaceLabels: Record<string, string> = {
  "user/avatars": "用户头像",
  "user/backgrounds": "用户背景",
  "user/other": "用户其他文件",
  "book/covers": "作品封面",
  "book/illustrations": "作品插图",
  "book/other": "作品其他文件",
  forum: "论坛空间",
  _root: "根目录",
}

const groupLabels: Record<string, string> = {
  user: "用户空间",
  book: "作品空间",
  forum: "论坛空间",
  _root: "根目录",
}

export function spaceLabel(key: string): string {
  return spaceLabels[key] ?? `目录 · ${key}`
}

export function groupLabel(group: string): string {
  return groupLabels[group] ?? `目录 · ${group}`
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index++
  }
  return `${value >= 100 || index === 0 ? value.toFixed(0) : value.toFixed(2)} ${units[index]}`
}

export function channelBytes(usage?: ChannelUsage): number {
  if (!usage) return 0
  if (typeof usage.usedBytes === "number" && Number.isFinite(usage.usedBytes)) return usage.usedBytes
  return Math.round((usage.usedMB || 0) * 1048576)
}

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "—"
  if (seconds % 86400 === 0) return `${seconds / 86400} 天`
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`
  return `${seconds} 秒`
}

export function formatDateTime(value: string | number | null | undefined): string {
  if (value == null || value === "") return "—"
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return "—"
  return parsed.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function imgbedError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return "登录已过期，请重新登录。"
    if (error.code === "permission_denied" || error.code === "admin_required") return "图床管理仅限管理员访问。"
    if (error.code === "invalid_origin") return "来源校验失败，请刷新页面后重试。"
    if (error.code === "invalid_image_request") return "配置参数不合法，请检查名称、地址、配额或凭据后重试。"
    if (error.code === "invalid_key_request") return "Key 参数不合法，请检查名称、权限与有效期（须为未来时间）。"
    if (error.code === "key_not_found") return "API Key 不存在或已被移除。"
    if (error.code === "image_storage_unavailable") return "图床存储服务暂时不可用，请稍后重试。"
    if (error.code === "key_store_unavailable" || error.code === "authentication_unavailable") return "账户认证服务暂时不可用，请稍后重试。"
    if (error.status >= 500) return "图床服务响应异常，请稍后重试。"
    return error.message
  }
  return "网络连接失败，请检查网络后重试。"
}
