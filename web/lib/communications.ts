export type Communication = {
  id: string; kind: "announcement" | "email" | "both"; title: string; content: string
  audience: "all" | "admins" | "self"; status: "draft" | "published" | "archived" | "queued"
  revision: number; created_at: string; updated_at: string; published_at: string | null
  total: number; sent: number; failed: number
}
export type CommunicationsResponse = { items: Communication[]; smtp_ready: boolean; audience: Record<Communication["audience"], number> }
export const audienceNames = { all: "全部已验证用户", admins: "管理员", self: "仅自己 · 测试" }
export const channelNames = { announcement: "站内公告", email: "邮箱推送", both: "站内公告 + 邮箱推送" }
export function messageDate(value: string) {
  return new Date(value).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false })
}
export function messageStatus(item: Communication) {
  if (item.total) return item.sent + item.failed < item.total ? "邮件发送中" : item.failed ? item.sent ? "邮件部分失败" : "邮件发送失败" : "邮件已发送"
  if (item.status === "queued") return "邮件发送中"
  return { draft: "草稿", published: "已发布", archived: "已撤下" }[item.status]
}
