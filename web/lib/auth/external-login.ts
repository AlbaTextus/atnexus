export type LoginProvider = { id: string; name: string; enabled: boolean }
export type ExternalIdentity = { id: string; provider: string; email: string; display_name: string; avatar_url?: string; created_at: string }

const messages: Record<string, string> = {
  provider_unavailable: "该登录方式暂未开放，请使用其他方式登录。",
  invalid_state: "登录验证已过期或不属于当前浏览器，请重新发起登录。",
  link_session_expired: "账户登录状态已变化，请重新登录后绑定。",
  authorization_cancelled: "你已取消第三方授权，可以随时重试。",
  provider_failed: "暂时无法完成第三方验证，请稍后重试。",
  email_link_required: "此邮箱已有账户。请先使用原来的方式登录，再到“账户安全”绑定第三方账号。",
  verified_email_required: "第三方账号没有提供已验证的邮箱。请先验证邮箱，或登录已有 ALBATEXTUS 账户后绑定。",
  identity_linked: "此第三方账号已被绑定，或当前账户已绑定该平台的其他账号。",
  account_disabled: "此账户已停用，请联系管理员。",
  registration_disabled: "全站注册已关闭，请使用已有账户登录。",
  external_login_disabled: "第三方登录已关闭，请使用邮箱或通行密钥登录。",
  login_provider_disabled: "该第三方登录方式已关闭，请选择其他登录方式。",
  site_features_unavailable: "暂时无法读取站点功能设置，请稍后重试。",
  login_failed: "第三方登录未完成，请重试。",
}

export function externalLoginError(code: string | null) {
  if (!code) return ""
  return Object.hasOwn(messages, code) ? messages[code] : messages.login_failed
}
