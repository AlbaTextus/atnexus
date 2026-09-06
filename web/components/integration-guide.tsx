"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { AlertCircle, ArrowUpRight, BookOpen } from "lucide-react"
import { api } from "@/lib/api"
import { type OAuthMetadata, managementError } from "@/lib/applications"
import { CopyField } from "@/components/copy-field"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { FieldGroup } from "@/components/ui/field"

export function IntegrationGuide() {
  const [metadata, setMetadata] = useState<OAuthMetadata | null>(null)
  const [error, setError] = useState("")
  useEffect(() => { api<OAuthMetadata>("/.well-known/oauth-authorization-server").then(setMetadata).catch(reason => setError(managementError(reason))) }, [])
  return <div className="flex flex-col gap-6"><div className="center-page-heading"><div><p className="center-eyebrow">DEVELOPER GUIDE</p><h1>接入指南</h1><p>通过 OAuth 2.0 授权码流程，让子系统使用统一账户登录。</p></div><Button variant="outline" nativeButton={false} role="link" render={<Link href="/admin/applications" />}>管理应用<ArrowUpRight data-icon="inline-end" /></Button></div>
    <Card><CardHeader><CardTitle className="flex items-center gap-2"><BookOpen size={18} />从接入到登录</CardTitle><CardDescription>服务端应用和公开客户端都可以使用授权码 + S256 PKCE。</CardDescription></CardHeader><CardContent><ol className="integration-steps">{[{ title: "创建应用", text: "填写应用主页与回调地址。服务端应用保存 Client ID 和 Client Secret；公开客户端仅使用 Client ID。" },{ title: "发起授权", text: "子系统生成随机 state 和 code_verifier，将 code_verifier 的 SHA-256 结果进行 Base64URL 编码作为 code_challenge，然后跳转到授权接口。" },{ title: "验证回调", text: "用户在统一登录页登录后，授权接口将 code 和 state 带回你的回调地址。子系统必须验证 state 与发起时一致。" },{ title: "交换令牌", text: "服务端向 Token Endpoint 提交 grant_type=authorization_code、code、redirect_uri、client_id 和 code_verifier。机密客户端额外通过 HTTP Basic 或 client_secret 参数认证。" },{ title: "获取用户并创建子系统会话", text: "使用 Authorization: Bearer ACCESS_TOKEN 请求 User Endpoint，以返回的 id 作为稳定用户标识，随后建立子系统自己的会话。" }].map((step,index) => <li key={step.title}><span>{index+1}</span><div><h2>{step.title}</h2><p>{step.text}</p></div></li>)}</ol></CardContent></Card>
    {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
    {metadata && <Card><CardHeader><CardTitle>服务端点</CardTitle><CardDescription>下列地址来自当前服务的授权服务器元数据。</CardDescription></CardHeader><CardContent><FieldGroup><CopyField label="Issuer" value={metadata.issuer} /><CopyField label="Authorization Endpoint" value={metadata.authorization_endpoint} /><CopyField label="Token Endpoint" value={metadata.token_endpoint} /><CopyField label="User Endpoint" value={metadata.userinfo_endpoint} /><CopyField label="Revocation Endpoint" value={metadata.revocation_endpoint} /></FieldGroup></CardContent></Card>}
    <Card><CardHeader><CardTitle>接入约定</CardTitle></CardHeader><CardContent><ul className="flex list-disc flex-col gap-3 pl-5 text-sm leading-relaxed text-muted-foreground"><li>当前提供 OAuth 2.0，不签发 OpenID Connect 的 ID Token；请使用通用 OAuth 客户端，并通过用户接口获取身份。</li><li>默认权限为 profile 和 email。令牌使用 Bearer 方式传递，不能当作 JWT 在子系统内自行解码。</li><li>回调地址必须精确匹配。生产环境使用 HTTPS；Client Secret 不应出现在浏览器代码中。</li><li>启停应用、修改回调地址或访问角色、重置密钥后，旧授权会被撤销。单个子系统退出登录应清理自己的会话。</li></ul></CardContent></Card>
  </div>
}
