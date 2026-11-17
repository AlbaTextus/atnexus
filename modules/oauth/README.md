# OAuth 与账户模块

本模块负责账户认证、OAuth 授权、第三方登录、Passkey、邮箱、人机验证，以及外部 API Key 的管理与凭据校验。

统一服务在本地默认 `8001` 挂载 `/api/v1/*`，协议端点 `/oauth/*` 和发现端点保持原路径。`OAUTH_HTTP_ADDR` 可覆盖监听地址，兼容原 `HTTP_ADDR`。

- `api/`：账户及外部 Key 管理 HTTP API。
- `protocol/`：OAuth 协议核心。
- `session.go`：其他模块共用的 session 校验权威；每次请求直接核验数据库，撤销立即生效。
- `apikey.go`：外部 API Key 校验权威，保留既有哈希、用户归属与状态规则。
- `repository/apikey.go`：外部 Key 生命周期及校验查询；原账户存储层委托此实现。
- 账户及 Key 数据继续使用原账户库和迁移链，本轮不搬迁物理数据。

`GET /api/v1/session` 返回 session 到期时间与缓存隔离标识，不返回凭证。浏览器站内请求使用 HttpOnly session Cookie；Key 用于外部调用，两者独立。

内容权限、额度、调用统计等旧账户入口暂保留过渡实现，其进一步迁移需明确归属。
