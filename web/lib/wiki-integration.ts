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

## 钱包支付接入

平台钱包使用积分。机密应用复用现有 **Client ID / Client Secret**，收款钱包属于应用所有者；没有所有者的历史应用和公开客户端不能发起支付。用户不能向自己的应用付款。

1. 子系统先通过用户接口获取付款人的 UUID（\`id\`），在自己的服务端确定商品和金额。
2. 使用 HTTP Basic 认证调用 \`POST /api/v1/payments/orders\`，以 JSON 提交 \`external_order_id\`、\`payer_id\`、\`title\`、\`amount\`。金额必须为字符串，例如 \`"12"\`，仅支持整数，小数会被拒绝。
3. 将用户引导至响应中的 \`checkout_url\`。用户登录后核对订单，点击确认支付，完成余额扣款。
4. 服务端使用相同的 HTTP Basic 认证调用 \`GET /api/v1/payments/orders/{id}\` 查询结果，只有 \`status=paid\` 才可以发货。浏览器跳转或页面提示不能作为发货依据。

同一应用的 \`external_order_id\` 是幂等键：相同内容重试返回同一订单，内容不同返回 409。订单有效期 15 分钟，状态为 \`pending\`、\`paid\`、\`cancelled\` 或 \`expired\`；过期后重新发起付款需使用新订单号。重复确认已支付订单不会再次扣款。订单状态查询应设置合理间隔，并在完成、取消或过期后停止。

用户在 [我的钱包](/account/wallet) 查看余额和流水，登录后可在订单页直接确认支付。超级管理员可在 [用户管理](/admin/users) 打开用户详情，查看积分与流水、调整积分并填写原因。

本次提供余额记账和订单查询，不接入法币充值、提现、自动回调、红包或排行榜。发生订单争议时可由超级管理员按原因调整余额；这不会更改原订单状态，也不构成自动退款接口。

## 服务端点

下列地址来自当前服务的 [授权服务器元数据](/.well-known/oauth-authorization-server)，可直接复制使用。`
