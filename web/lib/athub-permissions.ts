export type Operation = "read" | "upload" | "edit" | "delete" | "review"
export type AccessRule = { operation: Operation; method: string; path: string; allowed: boolean }
export type RolePolicy = { role: string; review_required: boolean; updated_at: string; rules: AccessRule[] }
export type Endpoint = { method: string; path: string; operation: Operation }
export const operationNames: Record<Operation, string> = { read: "查看 / 下载", upload: "上传 / 新建", edit: "修改", delete: "删除", review: "审核" }
export const permissionLevels = [{ role: "USER", label: "LT1" }, { role: "LEVEL_2", label: "LT2" }, { role: "ADMIN", label: "LT3" }, { role: "SUPER_ADMIN", label: "LT4" }]
export const endpointNames: Record<string, string> = {
  "/api/v2/platforms": "平台", "/api/v2/authors": "作者", "/api/v2/tags": "标签",
  "/api/v2/novelinfo": "小说资料", "/api/v2/content": "章节正文", "/api/v2/toc": "目录",
  "/api/v2/volumes": "分卷", "/api/v2/upload": "批量上传", "/api/v2/illustrations": "插图", "/api/v2/illustration-layout": "插图布局", "/api/v3/history": "变更历史", "/api/v3/contributors": "贡献者",
}
export function hasOperation(policy: RolePolicy, operation: Operation) {
  return policy.rules.some(rule => rule.operation === operation && rule.method === "" && rule.allowed)
}
export function allowsEndpoint(policy: RolePolicy, operation: Operation, method: string, path: string) {
  return policy.rules.find(rule => rule.operation === operation && rule.method === method && rule.path === path)?.allowed ?? hasOperation(policy, operation)
}
export type ReviewItem = {
  id: string; user_id: string | null; contributor: { id: string; name: string; avatar: string }
  operation: Operation; method: string; path: string; query: string; payload?: Record<string, unknown>
  status: "pending" | "approved" | "rejected" | "failed"; note: string
  reviewer?: { id: string; name: string; role: string; avatar?: string }; result_status?: number; result?: unknown
  created_at: string; reviewed_at?: string
}
