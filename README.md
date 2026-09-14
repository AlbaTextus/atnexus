# ALBATEXTUS Connect

**一个账户，让连接更简单。**

ALBATEXTUS 是面向社区应用的账户中心与 OAuth 2.0 授权服务。用户可以管理个人资料、登录方式和应用授权；管理员可以管理用户、接入应用及公开协议。

后端使用 Go，前端使用 Next.js、React 和 shadcn/ui，数据存储于 PostgreSQL。Prisma 负责数据库结构与迁移，Go 运行时通过 pgx 访问数据库。

[快速开始](#快速开始) · [登录与账户](#登录与账户) · [应用接入](#应用接入) · [配置与部署](#配置与部署) · [API](#api) · [开发](#开发)

## 功能

| 模块 | 当前能力 |
| --- | --- |
| 账户认证 | 邮箱验证码注册、邮箱或登录标识与密码登录、会话管理、记住邮箱 |
| 邮箱管理 | SMTP 验证码、验证已有邮箱、绑定或更换邮箱 |
| 第三方登录 | GitHub、LINUX DO Connect、自定义 OAuth 平台，支持绑定与解绑 |
| 通行密钥 | WebAuthn / Passkey 注册、无用户名登录、凭据管理 |
| 人机验证 | Cloudflare Turnstile，服务端验证令牌 |
| 个人中心 | 个人资料、昵称、平台头像来源、密码及登录方式管理 |
| OAuth 服务 | 授权码、S256 PKCE、刷新令牌轮换、客户端凭证、令牌撤销与自省 |
| 授权确认 | 展示应用与申请权限，用户明确允许后才签发授权码 |
| 管理后台 | 应用接入、客户端密钥、最低访问角色、用户管理、协议编辑 |
| 钱包 | 平台积分余额、分页收支流水、订单确认支付 |
| 支付 | 应用创建订单、用户核对与密码确认、原子扣款及收款、幂等查询 |

本项目提供 OAuth 2.0 服务，**不签发 OpenID Connect ID Token**。访问令牌是不透明字符串；应用通过 `/api/v1/user` 获取用户身份，不应将令牌当作 JWT 解码。

## 钱包与支付

钱包直接集成在本项目，共用用户、会话、应用和 PostgreSQL。用户入口为 `/account/wallet`，超级管理员入口为 `/admin/wallet`。余额默认零，单位为平台积分；超级管理员可按用户编号查询流水、增加或扣除余额，必须填写原因，操作带幂等键。没有引入参考项目的红包、排行榜、社区等级或争议系统，也没有法币充值、提现和自动退款。

**启用方式：** 执行 `pnpm db:deploy` 应用钱包、整数积分和 `20260910000000_wallet_settlements` 迁移，重启 Go 服务并重新构建前端。已有用户首次读取或使用钱包时创建零余额钱包。数据库约束阻止负余额，并保留订单与流水；已有订单的应用只能停用，不能删除。

钱包页面现已通过同源服务端接口 `/account/wallet/data` 读取 Go 后端与 PostgreSQL，不再读取本地模拟文件，也不需要 `BALANCE_PREVIEW_ENABLED`。页面样式与支付页面保持不变。总积分为可用积分与未来积分之和，支付只能使用可用积分。

**未来积分：** 新支付成功后，付款方立即扣款，收款方先计入未来积分，从 `paid_at` 起满 **24 小时** 后转入可用积分。Go 服务每 15 秒扫描到期记录；钱包读取、统计查询和后续记账也会先结算已到期积分。服务重启后会补处理，多个实例或重复请求不会重复入账。到账时间由服务器确定，页面可查看确切时间。升级前已到账的余额及旧订单不重新冻结；超级管理员的积分调整仍即时生效。可用与未来积分之和不能超过余额上限。

7 天统计按上海时区统计今日及此前六天，以实际可用余额流水为依据：支出计入付款日，收入计入预定到账日；尚未到账的未来积分不计入收入，空白日期补零。管理调整也纳入对应日期的收入或支出。统计独立于活动列表的筛选和分页。

用户登录后在订单页核对商品与金额，点击确认支付即可扣除积分。钱包页展示余额、流水和订单；第三方登录用户也可以直接确认支付。付款接口验证当前账户会话、请求来源和订单归属。

### 应用支付接口

机密应用使用现有 Client ID / Client Secret 进行 HTTP Basic 认证，服务端创建订单，收款人为应用所有者。公开客户端、未指定所有者的应用、已停用应用和自付订单无法创建。付款用户必须满足应用最低角色。钱包 API 使用账户会话与账户中心请求头，应用密钥或 OAuth 令牌不能代替用户确认支付。

`POST /api/v1/payments/orders`，`Content-Type: application/json`：

```json
{
  "external_order_id": "purchase-20260909-001",
  "payer_id": "用户接口返回的 UUID",
  "title": "服务积分消费",
  "amount": "12"
}
```

服务端响应 `{ "order": { ... }, "checkout_url": "https://账户中心/pay/订单UUID" }`。新订单返回 201，相同订单重试返回 200。金额必须是整数字符串，正数上限为 `90000000000`；小数输入直接拒绝。数据库保留原有 100 倍存储单位以兼容已有余额，并通过约束确保积分为整数。

将用户引导到 `checkout_url`，用户核对应用、商品和金额后点击确认支付。订单 15 分钟有效。支付事务同时完成付款方扣款及流水、应用所有者未来积分增加、24 小时到账记录及订单状态更新，使用行锁防止并发透支。到期时原子减少未来积分、增加可用积分并写入唯一收款流水。重复确认不会重复扣款；金额、付款人或商品不同的同订单号请求返回 409。

应用服务端用相同 Basic 认证查询 `GET /api/v1/payments/orders/{id}`，只在 `status=paid` 时按订单 UUID 幂等发货。其余状态为 `pending`、`cancelled`、`expired`。查询仅限本应用订单；本次不提供异步回调。不要将浏览器跳转或客户端提示视为支付凭据。服务端下单金额必须从可信商品配置计算。

| 账户接口 | 用途 |
| --- | --- |
| `GET /api/v1/account/wallet` | 可用余额 `balance`、未来余额 `pending_balance` |
| `GET /api/v1/account/wallet/view?resource=user` | 当前登录用户的积分卡片数据和一天到账配置 |
| `GET /api/v1/account/wallet/view?resource=daily-stats` | 7 天收入、支出及总计，积分以整数字符串返回 |
| `GET /api/v1/account/wallet/view?resource=transactions` | 兼容活动列表及小票的数据，分页与类型、状态、时间筛选 |
| `GET /api/v1/account/wallet/entries?offset=0` | 流水，每页 20 条，返回 `has_more` |
| `GET /api/v1/account/wallet/orders?offset=0` | 付款订单，每页 20 条 |
| `GET /api/v1/account/wallet/orders/{id}` | 本人付款订单详情 |
| `POST /api/v1/account/wallet/orders/{id}/pay` | 无需请求体，使用当前账户会话和 `X-AlbaTextus-Client: account-center` 请求头确认扣款 |
| `POST /api/v1/account/wallet/orders/{id}/cancel` | 取消尚未支付且未过期的订单 |
| `GET /api/v1/admin/wallet/users/{numericId}?offset=0` | 超级管理员查看指定用户钱包和流水 |
| `POST /api/v1/admin/wallet/adjustments` | 超级管理员提交 `user_numeric_id`、字符串 `amount`（正负）、`reason`、`idempotency_key` |

主要错误：余额不足 `insufficient_balance`（409）、订单关闭 `order_closed`（409）、幂等键冲突 `idempotency_conflict`（409）。超时或网络失败后，创建订单和调整余额必须沿用原幂等键重试，先核对结果再发起新操作。

活动查询支持 `page`（从 1 开始）、`page_size`（1–100，默认 20）、逗号分隔的 `types` / `statuses`、RFC3339 格式的 `startTime` / `endTime`。默认查询近 30 天，最大偏移 100000；返回 `{orders,total,page,page_size}`。按创建时间和 UUID 倒序稳定排序，空页返回空数组及完整总数。付款订单映射为 `payment`，已确认的收款为 `receive`，平台管理调整为 `community`；未付款订单不向收款方展示。取消订单在兼容视图中显示 `failed`，小票备注为“订单已取消”，原支付 API 仍返回 `cancelled`。编号使用现有订单或流水 UUID。接口只读取当前会话所属钱包，不接受用户 ID 指定其他账户，也不输出邮箱、密码摘要或客户端密钥。

## 邮箱认证（SMTP）

注册账户必须验证邮箱；账号中心的「账户安全」支持验证、绑定和更换邮箱。邮件仅通过 SMTP 发送，在 Go 服务的 `.env` 中配置：

```dotenv
SMTP_HOST=smtp.qq.com
SMTP_PORT=587
SMTP_USER=your-email@qq.com
SMTP_PASS=your-smtp-auth-code
# SMTP_FROM=noreply@example.com
```

`SMTP_FROM` 留空默认使用 `SMTP_USER`。QQ 邮箱需先开启 SMTP，并填写授权码。端口 587 强制 STARTTLS，465 默认使用 TLS；也可以用 `SMTP_SECURE` 显式指定直接 TLS，服务端始终验证证书。发送超时为 10 秒。配置不完整会阻止启动；全部留空时暂停邮箱注册和验证，已有账户登录及第三方登录不受影响。

上线前执行 `pnpm db:deploy` 应用邮箱验证迁移，再重启后端。存量账户默认标记为未验证，可在账户安全页面补验；已有密码登录继续可用。第三方新建账户只有上游明确证明邮箱已验证时才继承验证状态，LINUX DO 的专属登录标识不视为邮箱。

验证码为 6 位随机数字，5 分钟有效，最多尝试 5 次，数据库仅保存带服务端密钥的摘要。每个邮箱 60 秒最多发一次、每小时最多 10 次；账号中心同一账户也受此限制，整个服务每小时最多发 300 次。注册发信与注册提交均经过已配置的 Turnstile，生产环境建议启用。发送失败使本次验证码失效，冷却仍保留；再次发送会替换同场景、同会话、同邮箱的旧验证码。验证码不能跨注册/绑定场景、邮箱或登录会话使用。

更换邮箱需要目标邮箱验证码；有密码的账户还需确认当前密码。成功后保留当前会话，撤销其他设备会话及既有应用令牌。验证当前邮箱不会撤销会话。邮箱验证和账号变更在同一事务内完成。

| 接口 | 请求字段 |
| --- | --- |
| `POST /api/v1/auth/email/code` | `email`，返回 `challenge_id`、`expires_in`、`retry_after` |
| `POST /api/v1/users` | `email`、`password`、`challenge_id`、`email_code` |
| `POST /api/v1/account/setup` | 注册后用当前会话提交 `username`，完成首次用户名设置 |
| `POST /api/v1/account/email/code` | `email`，要求登录及账号中心请求头 |
| `POST /api/v1/account/email` | `email`、`challenge_id`、`code`、`current_password`（有密码时必填） |

`/api/v1/user` 返回 `email_verified`；OAuth 应用仅在获准 `email` scope 后收到该字段。

## 快速开始

以下命令在仓库根目录执行。需要 Go 1.25+、Node.js（建议 22+）、pnpm 和 PostgreSQL。仓库通过 `packageManager` 固定 pnpm 版本；Docker Compose 使用 PostgreSQL 16。

### 1. 准备环境

```bash
cp .env.example .env
pnpm install --frozen-lockfile
```

PowerShell 可用 `Copy-Item .env.example .env` 复制配置。已有 `.env` 时直接编辑，不要覆盖现有凭据。

生成一次客户端密钥加密用的随机密钥：

```bash
node -e "console.log(require('crypto').randomBytes(32).toString('hex'))"
```

将输出填入根目录 `.env` 的 `CLIENT_SECRET_ENCRYPTION_KEY`。它必须是 32 字节密钥的 64 位十六进制编码，后续启动应复用同一个值。

本地关键配置如下，其余配置见 [.env.example](.env.example)：

```dotenv
DATABASE_URL=postgresql://oauth:oauth@localhost:5432/oauth?sslmode=disable
ISSUER_URL=http://localhost:3000
AUTH_UI_URL=http://localhost:3000
COOKIE_SECURE=false
WEBAUTHN_RP_ID=localhost
WEBAUTHN_RP_ORIGINS=http://localhost:3000
```

### 2. 初始化数据库

已有 PostgreSQL 时，将 `DATABASE_URL` 改为实际地址。也可以使用仓库提供的数据库容器：

```bash
docker compose up -d postgres
```

数据库就绪后执行：

```bash
pnpm db:deploy
pnpm db:seed
```

`db:deploy` 应用全部已提交的迁移。`db:seed` 是开发初始化命令，会创建或更新以下账户和示例客户端：

| 项目 | 默认值 |
| --- | --- |
| 管理员邮箱 | `admin@example.com` |
| 管理员密码 | `Admin123!ChangeMe` |
| 管理员角色 | `ADMIN` |
| 示例 Client ID | `albatextus-web` |
| 示例 Client Secret | `development-secret-change-me` |
| 示例回调 | `http://localhost:3000/auth/callback` |

可用 `SEED_ADMIN_EMAIL`、`SEED_ADMIN_PASSWORD`、`SEED_ADMIN_NAME`、`SEED_CLIENT_ID`、`SEED_CLIENT_SECRET`、`SEED_CLIENT_REDIRECT_URIS` 覆盖默认值。Seed 会输出开发凭据，也会更新已有记录；不要把它作为每次部署必跑的迁移步骤。

示例回调只是登记值，本项目没有实现 `/auth/callback` 应用页面，实际接入时需替换。

### 3. 启动服务

终端一启动 Go API：

```bash
go run ./cmd/server
```

终端二启动前端：

```bash
pnpm web:dev
```

访问 [首页](http://localhost:3000/)、[登录页](http://localhost:3000/login) 或 [用户中心](http://localhost:3000/account)。Go API 默认监听 `8080`，健康检查为 [http://localhost:8080/healthz](http://localhost:8080/healthz)。

浏览器统一通过 `localhost:3000` 访问：Next.js 将 `/api/*`、`/oauth/*` 和 `/.well-known/*` 转发至 Go 服务，使页面与登录会话保持同源。不要混用 `localhost` 和 `127.0.0.1`，否则 Cookie 和 Passkey 的来源校验可能不一致。

## 登录与账户

### 登录流程

新账户创建后进入 `/welcome` 设置用户名，目前只有一个用户名输入框。保存成功后前往用户中心，或继续原先的应用授权流程。用户名作为 `display_name` 展示，登录仍使用邮箱或原有登录方式。未完成的设置会保留，刷新或重新登录后可以继续；第三方首次创建的账户也会进入此步骤并预填上游名称。已有账户不受影响，普通账户完成首次设置后仍遵循原有的昵称修改权限。

登录页采用纯白卡片，顶部 `ALBATEXTUS` 字标在完成 Cloudflare 安全验证后收拢为 `AT`。GitHub、LINUX DO 和通行密钥入口与密码登录共用验证状态，登录不要求点击协议确认。

创建账户页的验证流程为：

1. 配置了 Turnstile 时，先完成 Cloudflare 人机验证，保留片刻成功反馈。
2. 原验证区域显示「同意并继续」，下方提供隐私政策与用户协议链接。
3. 点击同意后收起验证区域，允许继续邮箱验证码注册或使用第三方账号。

未启用 Turnstile 时，登录可直接继续；创建账户页仍要求点击协议确认。验证过期或尝试失败后，需要重新完成对应步骤。协议链接在新窗口打开，避免丢失正在填写的表单。

这里的协议确认是创建账户页的交互门槛，当前没有独立的服务端协议签署记录；它与 OAuth 应用授权确认是两个不同流程。

“记住邮箱”只在登录成功后保存邮箱或登录标识，不保存密码；取消勾选会清除已保存的值。

### 第三方登录

在根目录 `.env` 配置凭据后重启 Go 服务。平台是否开放由后端返回；未配置的平台入口不可点击。

| 平台 | 必要配置 | 本地回调地址 |
| --- | --- | --- |
| GitHub OAuth App | `GITHUB_CLIENT_ID`、`GITHUB_CLIENT_SECRET` | `http://localhost:3000/api/v1/auth/providers/github/callback` |
| [LINUX DO Connect](https://connect.linux.do/) | `LINUXDO_CLIENT_ID`、`LINUXDO_CLIENT_SECRET` | `http://localhost:3000/api/v1/auth/providers/linuxdo/callback` |
| 自定义 OAuth | `CUSTOM_OAUTH_CLIENT_ID`、`CUSTOM_OAUTH_CLIENT_SECRET` 及端点 URL | `http://localhost:3000/api/v1/auth/providers/custom/callback` |

自定义平台需设置 `CUSTOM_OAUTH_AUTHORIZATION_URL`、`CUSTOM_OAUTH_TOKEN_URL`、`CUSTOM_OAUTH_USERINFO_URL`，支持授权码、S256 PKCE 和 `client_secret_post`。用户信息接口须接受 Bearer Token 并返回 JSON。默认字段为 `sub`、`email`、`email_verified`、`name`、`picture`，可通过 `CUSTOM_OAUTH_*_FIELD` 修改，支持 `data.id` 等点分路径。

平台稳定 ID 用于识别用户；相同邮箱不会自动合并账户或提升角色。已有账户应先登录，再到「账户安全」绑定第三方账号。

- GitHub 和自定义平台创建新账户时需要已验证邮箱。
- LINUX DO 使用稳定的用户 ID 创建独立账户，内部登录标识属于 `@accounts.albatextus.invalid` 保留域；`email_is_alias=true` 表示它不是收件邮箱。
- 第三方创建的账户可设置首个密码；解绑第三方账号前需要设置密码。
- 平台登录会同步头像，个人资料页可选择自动、默认或某个已绑定平台的头像来源。

第三方凭据仅供 Go 服务使用，不要放入 `NEXT_PUBLIC_*`。正式回调地址应使用 `AUTH_UI_URL` 对应的 HTTPS 域名。已有绑定后，不应把同一个自定义平台配置改指向另一套身份系统。

### 自定义头像与图床

选图后会弹出裁剪窗口，可拖动图片、缩放并查看圆形头像范围，支持触屏和方向键调整。确认裁剪后可重新裁剪；取消会保留已有预览，点击「保存头像」才会上传到图床。

在「个人资料 → 我的头像」选择「自定义上传」，选图预览后点击「保存头像」。支持 PNG、JPEG、WebP、GIF，最大 5 MB、1600 万像素；静态图片生成最大 512 × 512 的 PNG，GIF 逐帧裁剪并保留动画、帧间隔和循环次数。GIF 最多 200 帧，各帧完整画面合计不超过 4000 万像素，裁剪结果也不超过 5 MB。裁剪预览接口只返回图片，点击保存才上传图床并更新账户。头像通过 `POST /api/categories/user/avatars/upload?entityId=<用户 UUID>&returnFormat=full` 上传，保存在图床的 `user/<用户 UUID>/avatars/` 分类下；`entityId` 使用账户 UUID，而非页面地址中的数字 ID。用户中心、授权确认页和 OAuth 用户资料同步使用保存后的地址，旧头像链接继续有效。切换到平台或默认头像会保留已上传的自定义头像，方便再次选择。

Go 服务的 `.env` 需配置 `IMGBED_URL`（图床根地址）和 `IMGBED_API_TOKEN`（图床管理后台创建的、仅有 `upload` 权限的 API Token），配置后重启 API。令牌只供服务端使用，不要放入 `NEXT_PUBLIC_*`。生产图床地址须为 HTTPS，本地开发允许回环 HTTP。升级时先执行 `pnpm db:deploy` 应用头像字段迁移。

### Turnstile

设置 `TURNSTILE_SITE_KEY` 和 `TURNSTILE_SECRET_KEY` 启用验证；两项均为空时关闭，只填写一项会拒绝启动。`TURNSTILE_ALLOWED_HOSTNAMES` 为逗号分隔的精确域名，默认取 `AUTH_UI_URL` 的主机名，不含协议、端口或路径。

本地联调可使用 [Cloudflare 官方测试密钥](https://developers.cloudflare.com/turnstile/troubleshooting/testing/)：

```dotenv
TURNSTILE_SITE_KEY=1x00000000000000000000AA
TURNSTILE_SECRET_KEY=1x0000000000000000000000000000000AA
TURNSTILE_TEST_MODE=true
```

测试模式不提供真实机器人防护。生产环境需换成自己的密钥并关闭测试模式；`APP_ENV=production` 会拒绝测试模式和官方测试密钥。

Go 服务在注册、密码登录、第三方登录发起、通行密钥登录挑战创建前调用 Siteverify。受保护请求通过 `X-Turnstile-Token` 携带令牌；正式验证检查域名、`action=authenticate`、时效和重复使用。验证服务不可用时不会放行。已登录用户绑定账号及 OAuth 应用换取令牌不额外触发这一验证。

### 页面与权限

用户中心以数字用户 ID 作为路径：`/account/{numeric_id}`，个人资料为 `/account/{numeric_id}/profile`，账户安全为 `/account/{numeric_id}/security`。服务端按登录会话校验路径中的 ID，其他账户（包括管理员）访问这些私人页面返回 404。管理员通过独立的管理后台管理账户。

旧入口 `/account`、`/account/profile`、`/account/security` 会跳转到当前登录用户的数字 ID 路径；未登录先进入登录页。数字 ID 用于页面路由，OAuth 的稳定内部账户标识继续使用原 UUID。

| 页面 | 用途 |
| --- | --- |
| `/` | 公开主页、社区与接入资源入口 |
| `/login`、`/register` | 登录与注册 |
| `/account` | 个人资料概览 |
| `/account/profile` | 修改昵称、选择头像来源 |
| `/account/security` | 密码、通行密钥与第三方账号管理 |
| `/admin/applications` | 应用列表、创建、启停、配置与密钥管理 |
| `/admin/users` | 用户搜索、角色与状态管理、密码重置、撤销登录 |
| `/wiki/integration` | 公开应用接入指南、服务端点与接入约定 |
| `/admin/integration`、`/account/integration` | 跳转到 Wiki 接入指南 |
| `/admin/legal` | 跳转到 Wiki |
| `/wiki/privacy`、`/wiki/terms` | 公开协议正文；`/privacy`、`/terms` 跳转到对应 Wiki 页面 |
| `/consent` | OAuth 应用授权确认 |

账户采用四级等级：1 级普通用户（`USER`）、2 级预留、3 级管理员（`ADMIN`）、4 级超级管理员（`SUPER_ADMIN`）。2 级暂不对应角色，也不能分配或设置为应用访问门槛。用户接口保留 `role`，同时返回由角色确定的数字 `level`（1、3、4）；现有账户自动按角色对应等级，无需数据库迁移。普通用户管理自己的账户；3 级管理员可管理应用及协议，但用户管理仅限 1 级普通用户；4 级超级管理员可管理全部角色。管理操作禁止修改自己的角色、停用自己，或通过后台重置自己的密码和撤销自己的登录。

修改密码、变更角色、停用账户或撤销登录会使相关中心会话与 OAuth 凭据失效。子系统自己建立的本地会话需要由子系统处理失效与退出。

## 应用接入

### 登记客户端

在 `/admin/applications/new` 创建应用，填写名称、主页、回调地址、描述和最低访问角色。回调地址精确匹配，不支持通配符；生产使用 HTTPS，本地回环地址允许 HTTP。

- **服务端应用**使用 Client ID 和 Client Secret。密钥只应保存在应用服务端。
- **公开客户端**没有 Client Secret，授权请求必须使用 S256 PKCE。

新建应用默认允许 `authorization_code` 和 `refresh_token`，提供 `profile`、`email` 权限。客户端类型创建后不可修改。客户端凭证模式需应用自身已配置允许 `client_credentials`；该模式代表应用，不代表登录用户。

### 授权码流程

1. 应用生成随机 `state`、`code_verifier`，保存到自己的会话。
2. 计算 `code_challenge = BASE64URL(SHA256(code_verifier))`，去掉末尾的 `=`。
3. 浏览器跳转至授权端点；账户中心完成登录后展示应用授权确认页。
4. 用户允许后，中心将 `code` 与原始 `state` 返回应用登记的回调地址。拒绝则返回 `error=access_denied`。
5. 应用验证 `state`，用授权码和原始 `code_verifier` 换取令牌。
6. 应用携带访问令牌请求用户信息，以稳定的 `id` / `sub` 建立本地会话。

授权请求示意（各参数均需正确 URL 编码）：

```text
http://localhost:3000/oauth/authorize
  ?response_type=code
  &client_id=YOUR_CLIENT_ID
  &redirect_uri=YOUR_REGISTERED_CALLBACK
  &scope=profile%20email
  &state=YOUR_RANDOM_STATE
  &code_challenge=YOUR_S256_CHALLENGE
  &code_challenge_method=S256
```

以下是服务端客户端的 Bash / curl 示例，将占位值替换为应用实际值：

```bash
curl http://localhost:3000/oauth/token \
  --user 'YOUR_CLIENT_ID:YOUR_CLIENT_SECRET' \
  --data-urlencode 'grant_type=authorization_code' \
  --data-urlencode 'code=AUTHORIZATION_CODE' \
  --data-urlencode 'redirect_uri=YOUR_REGISTERED_CALLBACK' \
  --data-urlencode 'code_verifier=YOUR_ORIGINAL_VERIFIER'

curl http://localhost:3000/api/v1/user \
  --header 'Authorization: Bearer YOUR_ACCESS_TOKEN'
```

公开客户端省略 `--user`，改为提交 `client_id`，并保留 `code_verifier`。授权确认请求约 10 分钟有效，绑定当前用户与登录会话，只能处理一次；每次应用授权都需要用户确认。

用户信息始终包含 `id` / `sub`。申请 `profile` 才返回昵称、头像、角色等资料，申请 `email` 才返回邮箱及登录标识说明。请按需申请权限。

### 刷新、撤销与自省

```bash
curl http://localhost:3000/oauth/token \
  --user 'YOUR_CLIENT_ID:YOUR_CLIENT_SECRET' \
  --data-urlencode 'grant_type=refresh_token' \
  --data-urlencode 'refresh_token=YOUR_REFRESH_TOKEN'
```

刷新令牌会轮换，成功后需同时保存新的访问令牌和刷新令牌。只有允许 `refresh_token` 的应用才会获得刷新令牌。撤销使用 `POST /oauth/revoke`，查询令牌状态使用 `POST /oauth/introspect`；两者均提交 `token` 并进行客户端认证。

服务元数据位于 `/.well-known/oauth-authorization-server`。客户端密钥重置、回调地址或最低角色变更、应用启停等操作会撤销相关已有令牌与未使用的授权码。

## 配置与部署

### 关键环境变量

Go 服务从仓库根目录 `.env` 加载配置，已有进程环境变量优先。前端的 `OAUTH_BACKEND_URL` 和 `NEXT_OUTPUT_DIR` 需单独传给 Next.js 进程。

| 变量 | 用途 |
| --- | --- |
| `DATABASE_URL` | PostgreSQL 连接地址，必填 |
| `CLIENT_SECRET_ENCRYPTION_KEY` | 客户端密钥与 PKCE verifier 加密密钥，必填并保持稳定 |
| `APP_ENV` | 本地为 `development`，生产为 `production` |
| `HTTP_ADDR` | Go 监听地址，默认 `:8080` |
| `ISSUER_URL` | 对外 OAuth 地址；本地应为 `http://localhost:3000` |
| `AUTH_UI_URL` | 认证前端对外地址 |
| `COOKIE_SECURE` | HTTPS 部署设为 `true` |
| `SESSION_TTL` | 中心会话有效期，默认 `24h` |
| `AUTHORIZATION_CODE_TTL` | 授权码有效期，默认 `5m` |
| `WEBAUTHN_RP_ID` | Passkey 所属域名，不带协议和端口 |
| `WEBAUTHN_RP_ORIGINS` | 允许的完整前端 Origin，逗号分隔 |
| `PASSKEY_CHALLENGE_TTL` | Passkey 挑战有效期，默认 `5m` |
| `OAUTH_BACKEND_URL` | Next.js 转发到的 Go 地址，默认 `http://localhost:8080` |
| `NEXT_OUTPUT_DIR` | Next.js 构建目录，相对于 `web/`，默认 `.next` |

其他配置见 [.env.example](.env.example) 和 [配置实现](internal/config/config.go)。修改代理后端地址时，生产前端需要重新构建，以更新 rewrites。

### Docker Compose

完成快速开始中的环境配置、数据库迁移和初始化后，可以由 Compose 启动完整服务：

```bash
docker compose up --build -d
```

当前 [docker-compose.yml](docker-compose.yml) 是本地开发配置，固定使用本地地址和开发环境，并不自动执行迁移。Go 容器只接收 `environment` 中列出的变量：**目前未透传 `TURNSTILE_*` 和 `CUSTOM_OAUTH_AVATAR_FIELD`**，使用这些功能时需在 Compose 配置或覆盖文件中补充。只修改根目录 `.env` 不会把未列出的变量自动注入容器。

### 构建与运行

将前端构建放在仓库的 `release/` 下；构建和启动必须使用同一个 `NEXT_OUTPUT_DIR`。

Bash：

```bash
NEXT_OUTPUT_DIR=../release/web pnpm web:build
NEXT_OUTPUT_DIR=../release/web pnpm web:start
```

PowerShell：

```powershell
$env:NEXT_OUTPUT_DIR = '../release/web'
pnpm web:build
pnpm web:start
```

Go 服务可用 `go build -o release/connect ./cmd/server` 构建，Windows 可将输出命名为 `release/connect.exe`。运行时提供 `.env` 或进程环境变量。已有实例升级只需应用新迁移并更新服务，不需要重复 seed。

正式部署应使用同源 HTTPS 入口，正确设置 `ISSUER_URL`、`AUTH_UI_URL`、`WEBAUTHN_RP_ID`、`WEBAUTHN_RP_ORIGINS` 和 `COOKIE_SECURE=true`。反向代理保留 `/api/`、`/oauth/`、`/.well-known/` 的转发；健康检查直接请求 Go 的 `/healthz`。启用 CSP 时，为 Turnstile 放行 `https://challenges.cloudflare.com` 的脚本与 iframe。

数据库迁移应作为独立发布步骤执行。加密密钥须与数据库一起备份、多实例保持一致；已有客户端只有哈希而没有加密副本时，需重置后才能在后台查看明文。不要在每次部署时重新生成加密密钥，也不要在生产使用 seed 的默认凭据。

## API

完整路由以 [internal/httpapi/api.go](internal/httpapi/api.go) 为准。以下路径中的 `{id}`、`{provider}` 为参数；协议 `{slug}` 仅支持 `privacy`、`terms`。

### OAuth 与公开接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/healthz` | 正常返回 200；维护开启返回 433（status=maintenance）；数据库或维护状态读取失败返回 503 |
| GET | `/.well-known/oauth-authorization-server` | OAuth 元数据 |
| GET | `/oauth/authorize` | 发起授权码流程 |
| POST | `/oauth/token` | 换取、刷新令牌或使用客户端凭证 |
| POST | `/oauth/revoke` | 撤销令牌 |
| POST | `/oauth/introspect` | 查询令牌状态 |
| GET | `/api/v1/auth/providers` | 第三方平台开放状态 |
| GET | `/api/v1/auth/verification` | Turnstile 公开配置 |
| GET | `/api/v1/legal/{slug}` | 公开协议正文 |

### 认证与个人账户

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/v1/users` | 注册 |
| POST | `/api/v1/sessions` | 密码登录 |
| DELETE | `/api/v1/sessions/current` | 退出当前会话 |
| GET | `/api/v1/user` | 当前账户或 Bearer 用户身份 |
| POST | `/api/v1/auth/providers/{provider}/start` | 开始第三方登录或绑定 |
| GET | `/api/v1/auth/providers/{provider}/callback` | 第三方回调 |
| GET / DELETE | `/api/v1/account/identities` / `/api/v1/account/identities/{provider}` | 列出或解绑第三方账号 |
| GET | `/api/v1/account/applications` | 当前用户可访问的应用信息 |
| PATCH | `/api/v1/account/profile` | 更新昵称 |
| PATCH | `/api/v1/account/avatar` | 更改头像来源 |
| POST | `/api/v1/account/password` | 设置或修改密码 |
| POST | `/api/v1/passkeys/registration/options`、`/api/v1/passkeys/registration/finish` | 注册通行密钥 |
| POST | `/api/v1/passkeys/login/options`、`/api/v1/passkeys/login/finish` | 通行密钥登录 |
| GET / DELETE | `/api/v1/passkeys` / `/api/v1/passkeys/{id}` | 列出或删除通行密钥 |
| GET / POST | `/api/v1/oauth/consent` | 读取与提交应用授权确认 |

账户管理接口使用中心登录会话。第三方登录发起、账户修改、应用授权决定等请求需携带 `X-AlbaTextus-Client: account-center`；Passkey 完成接口使用 `X-WebAuthn-Ceremony`。开启 Turnstile 后，注册与登录发起还需有效的 `X-Turnstile-Token`。

### 管理接口

管理接口要求管理员会话，不能用应用的 Bearer Token 替代。写入及读取客户端密钥需要 `X-AlbaTextus-Client: account-center`，并接受来源校验。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET / POST | `/api/v1/applications` | 应用列表 / 创建 |
| GET / PUT / DELETE | `/api/v1/applications/{id}` | 应用详情 / 更新 / 删除 |
| GET / POST | `/api/v1/applications/{id}/secret` | 查看 / 重置客户端密钥 |
| GET / POST | `/api/v1/admin/users` | 用户列表 / 创建 |
| PATCH | `/api/v1/admin/users/{id}` | 修改昵称、角色与启停状态 |
| POST | `/api/v1/admin/users/{id}/password` | 重置用户密码 |
| POST | `/api/v1/admin/users/{id}/revoke-sessions` | 撤销用户登录与授权 |
| GET / PUT | `/api/v1/admin/legal/{slug}` | 读取 / 更新协议 |
| GET / PATCH | `/api/v1/admin/features` | 读取 / 更新站点功能开关 |

用户列表支持 `q`、`role`、`status`、`page`、`page_size`；更新用户需提交读到的 `updated_at`。协议以 Markdown 保存，更新需提交 `revision`。版本冲突会返回 `409`，应重新读取后编辑。

## 站点功能开关

管理员可在「应用接入」页面底部控制第三方登录、全站注册和 Cloudflare Turnstile 验证。第三方登录保留总开关，并支持单独启停 GitHub、LINUX DO 和已配置的自定义平台；平台名称和 Cloudflare 验证前显示品牌图标。设置保存在数据库 `site_features` 中并即时生效；部署前需执行 `pnpm db:deploy`。

关闭第三方登录总开关会停用全部平台的登录与账户绑定入口，并保留各平台的独立设置。关闭某个平台只阻止该平台登录与绑定；登录入口、回调及账户写入事务均检查设置，已经开始的流程也受限制。关闭全站注册会阻止邮箱验证码注册及第三方首次建号，已有账户仍可登录，已登录用户仍可绑定邮箱。关闭 Cloudflare 验证会跳过登录、注册和通行密钥操作的人机验证；重新开启后仍需有效的 `TURNSTILE_SITE_KEY` 与 `TURNSTILE_SECRET_KEY` 配置才能显示验证组件。

接口 `GET /api/v1/admin/features` 返回三个总开关、`providers` 平台列表（`id`、`name`、`enabled`、`configured`）和 Turnstile 的配置状态；`PATCH /api/v1/admin/features` 接受 `{"feature":"external_login_enabled|registration_enabled|turnstile_enabled","enabled":true|false}`，或 `{"provider":"github|linuxdo|custom","enabled":true|false}`，每次只指定一种目标。平台状态独立保存于 `provider_login_enabled`，更新一个平台不会覆盖其他平台；未单独设置的平台默认允许，总开关关闭时仍全部停用。读写均要求管理员会话及标准账户请求来源校验。

## 网站维护

超级管理员可通过账户页面右上角、退出登录按钮左侧的扳手按钮开启或结束维护。普通管理员没有该权限。确认开启后，访客、普通用户和普通管理员均进入 `/maintenance`；只有有效且启用中的 `SUPER_ADMIN` 会话可以继续访问。

维护状态保存在数据库 `site_maintenance` 中，默认关闭，重启服务后保留。部署此功能前运行 `pnpm db:deploy`，随后更新 Go 后端与 Next.js 前端。状态读取异常时暂停放行，避免误开放网站。

维护页不展示管理员登录入口。超级管理员可直接访问 `/maintenance/login`，使用密码、通行密钥或已绑定的第三方账号登录。普通账户即使密码正确也不会获得新会话；第三方登录会先验证已有绑定账户的超级管理员角色，拒绝其他账户，也不会自动注册新用户。已登录的超级管理员可以正常使用管理功能并结束维护。

页面访问由 Next.js 请求代理拦截并跳转至维护页，业务 API 和 OAuth 接口由 Go 中间件校验，维护期间受限请求返回 `433` 和 `Retry-After: 15`；无法读取维护状态时仍返回 `503`。已打开的页面每 15 秒检查一次状态，并在重新聚焦时检查；结束维护后，维护页自动恢复到首页。此功能不会撤销已有会话，恢复后用户可继续使用。

接口：`GET /api/v1/maintenance` 返回当前维护状态与当前会话是否可放行；`PUT /api/v1/admin/maintenance` 接受 `{"enabled": true|false}`，要求超级管理员会话及标准账户请求来源校验。健康检查、状态查询、身份验证所需接口以及退出登录保持可用。

### 已知限制：维护期间的 OAuth 授权

2026-09-09 隔离数据库联调确认：超级管理员可进入授权确认页并获取授权码，但接入应用通过服务器请求 `/oauth/token` 换取或刷新令牌时，没有浏览器会话 Cookie，仍被维护中间件拦截并返回 `503 maintenance`。仅携带 Bearer 令牌的 `/api/v1/user` 请求也会被拦截。因此，维护期间尚不能完成超级管理员的完整应用登录流程；关闭维护后流程正常。后续需按授权码或令牌所属用户校验维护访问权限，同时保留客户端认证与 PKCE 校验。本次提交记录该限制，尚未修复。

## 全站公告与邮箱推送

管理后台使用统一的 `/admin/notifications` 消息发布页面。同一份标题与正文可选择「站内公告」「邮箱推送」或两个渠道同时发布，共用一份草稿和发布记录；旧的 `/admin/announcements`、`/admin/emails` 地址均跳转到此页面。管理员可保存草稿、发布和撤下公告，以及向全部已验证用户、管理员或自己发送邮件。邮件范围仅包含启用且邮箱已验证的账户，排除 `.invalid` 占位邮箱；站内公告始终对全站公开，与邮件收件范围独立。后台记录与公开公告列表不设条数截断，每行只显示标题与时间，点击后查看详情或编辑。列表支持滚轮、触屏滑动、鼠标拖动滚动及右下角拖动调整高度；统计覆盖全部消息。

首页、Wiki、登录注册页与账户后台的右上角提供公告信封。未读公告显示红点，打开详情或点击「全部已读」后消除；发布内容更新后重新提醒。已读状态按账户保存在当前浏览器，访客单独记录，暂不跨设备同步。公告每分钟及窗口重新聚焦时刷新。后台采用上方编辑、下方完整预览、底部发布记录的布局。

点击发布记录会打开独立详情弹窗，不替换当前编辑内容与预览。已发布的公告可通过弹窗底部的「撤下公告」操作，长内容在弹窗内滚动，操作栏保持可见；草稿可通过「继续编辑」主动载入编辑器。仅邮件记录和已撤下公告不提供撤下按钮。

部署时先执行 `pnpm db:deploy` 应用 `20260910020000_communications` 迁移，再重启 Go 服务并更新前端。邮件复用现有 `SMTP_HOST`、`SMTP_PORT`、`SMTP_USER`、`SMTP_PASS`、`SMTP_FROM`、`SMTP_SECURE` 配置；未配置时仍可保存草稿。发件凭据仅在服务器环境变量中保存。

邮件模板基于项目外提供的 `mail/verification-code.html`，部署副本位于 `internal/email/templates/message.html`，通过 Go embed 随服务编译，无需部署机器保留原始绝对路径。验证码邮件动态填入验证码、用途和年份；消息邮件沿用相同品牌、渐变卡片与页脚，并用消息正文替代验证码区域。动态文本由 `html/template` 转义。实际发信使用 `multipart/alternative`，同时提供纯文本和 HTML，保留模板的 Outlook 布局回退。后台「邮件」预览调用管理员接口 `POST /api/v1/admin/communications/preview`，与发信使用同一个渲染函数，在禁用脚本的 iframe 中展示。

公告也使用同一品牌模板，标签和页脚切换为「社区公告」。管理员预览通过 `channel: "announcement"` 选择公告模板；用户通过 `GET /api/v1/announcements/{id}` 读取同一份渲染，草稿、已撤下内容和仅邮件消息均不可通过此接口访问。模板 iframe 允许同源尺寸测量，仍禁用脚本，并根据正文高度自适应。

双渠道发布时，在同一个事务中固定收件人、写入邮件队列并公开公告；入队失败则公告保持草稿，重复提交不会再次入队。服务后台逐封发送，每封邮件只包含自己的收件地址，页面轮询发送进度。「已发送」表示 SMTP 服务器已接收，不代表对方已读或最终投递成功。发送失败或进程中断后结果不明确的邮件记为失败，不自动重发；排查后可新建消息重新发送。包含已提交邮件的消息内容不可编辑，即使撤下其公告也不会修改邮件内容或取消队列。发布、撤下与发送均要求管理员会话和账户来源校验。

接口：公开公告 `GET /api/v1/announcements`；统一管理列表和新增 `GET/POST /api/v1/admin/communications`；更新 `PUT /api/v1/admin/communications/{id}`；邮件或双渠道发布 `POST /api/v1/admin/communications/{id}/send`；撤下公告 `POST /api/v1/admin/communications/{id}/archive`。消息的 `kind` 为 `announcement`、`email` 或 `both`。更新、发布和撤下需提交当前 `revision`，并发编辑冲突返回 409。公开接口仅返回公告内容和阅读版本，不暴露邮件范围或发送统计。

## 开发

```text
cmd/                  服务入口、开发 seed 与本地图床接入工具
internal/             配置、HTTP API、OAuth、Passkey、第三方登录、存储
prisma/               数据库 schema 与迁移
web/app/              Next.js 页面与全局样式
web/components/       账户、管理后台与基础 UI 组件
web/lib/              前端 API 与共享逻辑
temps/                本地临时脚本、检查与日志
release/              本地构建与发布产物
```

| 命令 | 用途 |
| --- | --- |
| `pnpm web:dev` | 前端开发服务 |
| `pnpm web:typecheck` | TypeScript 类型检查 |
| `pnpm db:validate` | 校验 Prisma schema |
| `pnpm db:format` | 格式化 Prisma schema |
| `pnpm db:migrate` | 在开发数据库创建、应用迁移 |
| `pnpm db:deploy` | 应用已提交的迁移 |
| `pnpm db:studio` | 打开数据库管理界面 |
| `go test ./...` | 运行 Go 测试 |

Prisma schema 与迁移是数据库结构的来源；Go 服务没有 Prisma Client，也不会在启动时自动建表。首次检查前端或更换构建目录时，先完成一次前端构建，生成 Next.js 路由类型。

相邻的 ALBATEXTUS 图床可通过 `go run ./cmd/register-imgbed` 联调注册。该工具会登记客户端并写入相邻图床的本地配置，运行前请核对 [工具源码](cmd/register-imgbed/main.go) 中的目录、端口与回调约定。

相邻的 Status 后台可通过 `go run ./cmd/register-status` 接入当前 Connect。工具创建独立机密应用 `albatextus-status`，最低角色为 `ADMIN`，仅申请 `profile`；默认回调为 `http://127.0.0.1:3100/api/auth/callback`，凭据写入 `../status/.dev.vars`。应用所有者使用 `SEED_ADMIN_EMAIL` 对应的现有管理员，也可在首次登记时用 `-admin-id <UUID>` 指定。Status 默认按服务端用户资料中的 `role` 允许 `ADMIN`、`SUPER_ADMIN`，符合条件的其他账号也能登录；Connect 应用更严格的 `SUPER_ADMIN` 最低角色会保留。支持 `-app-url`、`-issuer-url`、`-output`；重复运行保留已有密钥、回调、Status 管理密钥和自定义允许名单。旧版仅允许所有者 UUID 的配置，可运行 `go run ./cmd/register-status -migrate-owner-allowlist` 切换到角色授权；该选项拒绝自动清理自定义账号名单。登记后在 Status 执行 `npm run db:migrate:local` 并重启开发服务。工具不会重置用户、运行 seed 或修改其他应用。

目前未提供邮箱验证码绑定、OIDC ID Token 或跨子系统统一登出。功能问题和接入反馈可提交到 [GitHub Issues](https://github.com/AlbaTextus/connect/issues)。
