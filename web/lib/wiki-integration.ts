export const integrationTitle = "接入指南"
export const integrationDescription = "通过 OAuth 2.0 授权码流程，让子系统使用统一账户登录。"
export const integrationContent = `## 从接入到登录

服务端应用和公开客户端都可以使用**授权码 + S256 PKCE**。

### 1. 创建应用

在 [管理应用](/admin/applications) 中填写应用主页与回调地址。服务端应用保存 **Client ID** 和 **Client Secret**；公开客户端仅使用 **Client ID**。创建和管理应用需要管理员权限。

### 2. 发起授权

子系统生成随机 \`state\` 和 \`code_verifier\`，将 \`code_verifier\` 的 **SHA-256** 结果进行 **Base64URL** 编码作为 \`code_challenge\`，然后跳转到授权接口。

### 3. 验证回调

用户在统一登录页登录后，授权接口将 \`code\` 和 \`state\` 带回你的回调地址。子系统**必须验证 \`state\` 与发起时一致**。

### 4. 交换令牌

服务端向 **Token Endpoint** 提交 \`grant_type=authorization_code\`、\`code\`、\`redirect_uri\`、\`client_id\` 和 \`code_verifier\`。机密客户端额外通过 **HTTP Basic** 或 \`client_secret\` 参数认证。

### 5. 获取用户并创建子系统会话

使用 \`Authorization: Bearer ACCESS_TOKEN\` 请求 **User Endpoint**，以返回的 \`id\` 作为稳定用户标识，随后建立子系统自己的会话。

## 接入约定

- 当前提供 **OAuth 2.0**，不签发 **OpenID Connect** 的 **ID Token**；请使用通用 OAuth 客户端，并通过用户接口获取身份。
- 默认权限为 \`profile\` 和 \`email\`。令牌使用 \`Bearer\` 方式传递，**不能当作 JWT 在子系统内自行解码**。
- 回调地址必须精确匹配。生产环境使用 **HTTPS**；**Client Secret 不应出现在浏览器代码中**。
- 启停应用、修改回调地址或访问角色、重置密钥后，旧授权会被撤销。单个子系统退出登录应清理自己的会话。

## 服务端点

下列地址来自当前服务的 [授权服务器元数据](/.well-known/oauth-authorization-server)，可直接复制使用。`
