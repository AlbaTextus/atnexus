const messages: Record<string, string> = {
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
  return messages[reason.message] || fallback
}
