import { ApiError } from "@/lib/api"

const messages: Record<string, string> = {
  maintenance: "网站维护中，目前仅超级管理员可以登录。",
  maintenance_unavailable: "暂时无法确认服务状态，请稍后重试。",
  email_unavailable: "邮箱服务暂不可用，请稍后重试。",
  invalid_email: "请填写有效的收件邮箱。",
  invalid_email_code: "验证码错误、已过期或尝试次数过多，请重新获取。",
  email_rate_limited: "发送过于频繁，请至少等待 60 秒；达到每小时上限时需稍后再试。",
  incorrect_password: "当前密码不正确。",
  account_changed: "账户信息已变更，请刷新页面或重新登录后重试。",
  turnstile_required: "请先完成安全验证。",
  turnstile_failed: "安全验证未通过或已过期，请重新验证。",
  turnstile_unavailable: "安全验证服务暂时不可用，请稍后重试。",
  registration_disabled: "全站注册已关闭，已有账户仍可正常登录。",
  external_login_disabled: "第三方登录已关闭，请使用邮箱或通行密钥登录。",
  login_provider_disabled: "该第三方登录方式已关闭，请选择其他登录方式。",
  site_features_unavailable: "暂时无法读取站点功能设置，请稍后重试。",
  login_rate_limited: "登录失败次数过多，请稍后再试。",
  invalid_credentials: "邮箱或密码不正确，请检查后重试。",
  email_exists: "这个邮箱已经注册过了，请直接登录。",
  invalid_request: "请检查填写的信息后重试。",
  "email and a password of 12-72 characters are required": "请填写有效邮箱，并使用 12–72 个字符的密码。",
  server_error: "服务暂时不可用，请稍后再试。",
  passkey_unavailable: "通行密钥暂时不可用，请使用邮箱和密码登录。",
  invalid_passkey: "通行密钥验证失败，请重试或使用邮箱登录。",
  invalid_passkey_login: "通行密钥验证失败，请重试或使用邮箱登录。",
  "Failed to fetch": "无法连接到服务，请检查网络后重试。",
  "NetworkError when attempting to fetch resource.": "无法连接到服务，请检查网络后重试。",
  "Load failed": "无法连接到服务，请检查网络后重试。",
}

export function authErrorMessage(reason: unknown, fallback: string): string {
  if (!(reason instanceof Error)) return fallback
  if (reason.name === "NotAllowedError" || reason.name === "AbortError") {
    return "通行密钥验证已取消或超时，你可以重试或使用邮箱登录。"
  }
  return (reason instanceof ApiError && messages[reason.code]) || messages[reason.message] || fallback
}
