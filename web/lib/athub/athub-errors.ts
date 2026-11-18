import { ApiError } from "@/lib/api"

// The creator and content APIs describe some refusals in Chinese. The rest
// carry only a code or the content engine's English sentence, which is not
// written for readers of this site, so the status decides the wording then.
// `overrides` names a status more precisely where the caller knows what it
// means for its own operation.
export function contentErrorMessage(reason: unknown, fallback: string, overrides: Partial<Record<number, string>> = {}) {
  if (!(reason instanceof ApiError)) {
    // Only errors raised by this app carry a message for the reader; a failed
    // fetch or an unreadable reply raises the browser's own.
    return reason instanceof Error && reason.name === "Error" ? reason.message : fallback
  }
  const { status } = reason
  const described = reason.code !== "request_failed" && reason.message !== reason.code
  return overrides[status] || (described ? reason.message
    : status === 401 ? "登录已失效，请重新登录后再试。"
    : status === 403 ? "当前账户没有此操作权限，或权限已发生变化。"
    : status === 404 ? "没有找到对应的内容，可能已被删除。"
    : status === 409 ? "内容已发生变化或与现有内容冲突，请刷新后重试。"
    : status === 413 ? "提交内容过大，请缩减后重试。"
    // The cover or illustration it points at was replaced or removed meanwhile.
    : status === 422 ? "引用的封面或插图已从图床删除，请重新上传图片后再提交。"
    : status === 429 ? "请求过于频繁，请稍后重试。"
    : status >= 500 ? "服务暂时不可用，请稍后重试。"
    : status >= 400 ? "请求内容未通过校验，请检查后重试。"
    : fallback)
}
