"use client"

import { useAccountUser } from "@/components/account-shell"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

// Keep email settings separate so verification and address changes can be
// added here when their server-side flows are available.
export function AccountEmail() {
  const user = useAccountUser()
  const label = user.email_is_alias ? "登录标识" : "电子邮箱"
  return <Card>
    <CardHeader><CardTitle>{label}</CardTitle><CardDescription>管理用于登录和账户识别的信息。</CardDescription></CardHeader>
    <CardContent><Field>
      <FieldLabel htmlFor="security-email">{label}</FieldLabel>
      <Input id="security-email" value={user.email} readOnly aria-describedby="security-email-help" className="h-11" />
      <FieldDescription id="security-email-help">{user.email_is_alias ? "这是第三方账户的专属登录标识，不是收件邮箱。设置密码后可用它登录，也可继续使用第三方登录。" : "用于登录与账户识别。如需更改，请联系管理员。"}</FieldDescription>
    </Field></CardContent>
  </Card>
}
