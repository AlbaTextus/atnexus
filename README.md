# atnexus

atnexus 是 ALBATEXTUS 的统一服务：v1 是账户中心与 OAuth 2.0 授权，v2 是 ATHub 内容存储。
两套功能编译成一个程序，由 `cmd/server` 一起初始化，OAuth 监听 `8001`，ATHub 监听 `8003`，分别连接各自数据库。
Go module 路径为 `github.com/AlbaTextus/atnexus`，根工作区包为 `@albatextus/atnexus`，
账户前端包为 `@albatextus/atnexus-web`，统一程序名为 `atnexus`。

后端使用 Go，前端使用 Next.js、React 和 shadcn/ui，数据存储于 PostgreSQL。Prisma 负责数据库结构与迁移，Go 运行时通过 pgx 访问数据库。

[快速开始](#快速开始) · [登录与账户](#登录与账户) · [应用接入](#应用接入) · [配置与部署](#配置与部署) · [API](#api) · [开发](#开发)

## 小说库界面框架

首页 `/` 保留原有展示页面。小说库工作区使用独立入口 `/library`：
复用用户中心的 `SiteHeader` 顶部导航，包含小说库、分类、数据来源和设置，
最右侧显示当前登录账号头像并进入现有用户中心，内容区域居中铺开。
导航已预留分类、阅读记录、书架、收藏、数据来源、下载和设置，
对应 `/library/{categories,history,bookshelf,favorites,sources,downloads,settings}`。
书库列表、标签分类、数据来源、作品详情及章节目录通过浏览器
直接请求 `NEXT_PUBLIC_ATHUB_API_URL` 下的 `/api/v2/*`，Next.js 不转发小说接口。
书库与详情直接复制 `demo-2` 原组件及其 Button、Badge、Tabs、ToggleGroup 等控件到
`web/components/novel-demo/`，原样复用 DOM、类名和 CSS 声明；样式仅加
`.novel-demo` 作用域，避免影响主页和用户中心。与后端的连接由
`web/lib/novel-demo-data.ts` 适配，共用站点顶部导航；不改演示仓库，不引入其 SQLite、
固定书名、示例封面或跨项目源码引用。
作品详情沿用原图标页签，增加章节目录入口。目录仅使用 `/api/v2/toc` 返回的标题，
按“作品 → 卷 → 章节”展开和收起，卷与章节分别按 `order` 排序。
未分卷章节不会丢失：有卷时归入“未分卷”，整本无卷时直接列在作品下。
章节标题为普通文字，不提供阅读跳转，不请求 `/api/v2/content` 或显示正文。
小说库自动联动用户中心登录，无需输入 API Key。账户接口
`POST /api/v1/account/athub/session` 使用现有会话签发最长 5 分钟的只读凭证，
浏览器仅在内存保存，自动续期并直连内容后端；不生成、读取或重置个人 Key。
内容后端每次读取都在账户库检查会话有效期、撤销状态、账户封禁与管理员限制。
退出登录或管理员禁用后，已签发凭证也无法继续从服务端读取。会话调用保留审计记录，`key_id` 为空，
不计入个人 API Key 的总数、趋势、调用明细或管理员调用排行。
外部调用继续使用个人 API Key；个人 Key 的停用和重置不影响网站登录会话。
书库使用 `view=page` 每次读取 24 本，搜索书名/作者、排序、筛选及总数统计均由后端完成，
仅批量查询当前页的作者。页码和搜索条件保存在 URL，返回页面时可恢复。
资源统计使用 `view=count`，不下载完整列表。详情先显示基础资料，其余资料和目录独立加载。
前台按账户会话隔离内容缓存，并合并同路径并发请求；普通数据缓存 30 秒，标签和平台缓存 5 分钟。
F5 后先验证登录态，再复用当前会话在 `sessionStorage` 中仍有效的缓存；不持久化凭证、章节正文和修订历史。
窗口获得焦点时后台检查会话并按缓存有效期更新内容，保留已挂载页面；退出登录清理持久缓存。
内容模块另有每进程最多 128 项、16 MiB 的读缓存，单项超过 1 MiB 不缓存。
缓存位于鉴权、权限、额度与审计之后，实际 API Key 请求即使命中缓存仍然记账；
写入与审核通过后清空本进程缓存。多实例和直接 SQL 修改由缓存有效期兜底，不提供跨进程即时失效。
作者 API 提供可空的 `name`，书库列表和详情使用真实姓名；未补录姓名时保留作者编号。
没有接口字段的发行日期、年龄限制、总字数和资源链接不使用示例值，相关区域保留占位，不新增数据业务。
下载资源、阅读记录、书架和收藏仍是待接入区域。
8003 允许 `AUTH_UI_URL` 与 `WEBAUTHN_RP_ORIGINS` 对应来源进行带 Authorization 的 GET 跨域读取，
预检不消耗调用统计；实际 GET 仍执行鉴权与记账，不跨域传递账户 Cookie。
`NEXT_PUBLIC_ATHUB_API_URL` 是构建时配置，部署后端后需重新构建前端；反代须保留 Origin、
Authorization、OPTIONS 及后端的跨域响应头。
用户中心与管理后台继续使用原页面和鉴权，小说库侧栏不包裹这些页面。

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
| 内容存储（v2） | 平台、作者、小说、卷、章节、全书目录与批量上传 |
| 贡献历史（v3） | 贡献者、按请求合并的变更记录、字段前后对比 |

## 模块维护与路由

OAuth 业务继续使用 `/api/v1/*`，OAuth 协议端点 `/oauth/*` 和发现端点保持原路径。
ATHub 代码合入 [`modules/athub/`](modules/athub/README.md)，直接使用 `/api/v2/content`、
`/api/v2/upload` 等路径，不加 `/athub`。同一进程开两个监听器，模块之间没有内部 HTTP 转发。
v1 与 v2 在这里代表不同功能模块。

`8003/api/v3/history?novelId=2&limit=20&offset=0` 返回分页变更历史，
`8003/api/v3/contributors?novelId=2` 返回真实创建者与后续贡献者。
两个接口使用现有网站只读凭证或个人 Key 鉴权，浏览器直连后端；Next.js 不代理 v3。
作品详情的贡献者卡片链接到 `/library/books/2/history`，沿用现有组件、字体和面板。

历史表 `novel_revisions` 位于服务账户库 `DATABASE_URL`，迁移为根目录
`20261110050000_novel_history`；ATHub 小说库不新增历史表或账号表。
修改者由宿主根据已认证的账号或 Key 所属账号确定，客户端不能指定贡献者。
小说元数据、卷、章节、批量上传以及已有作者/标签名称的修改都记录变更；
同一次请求按小说合并为一条记录，无实际变化不新增记录。
章节正文更新时，在服务历史中保存修改片段及少量上下文，按字高亮新增与删除，并标注所在行；
不保存整章初始快照。单章保存与批量上传均记录差异，小说库仍只存当前内容。
旧历史只有摘要哈希、字数及变动标记，无法追溯补出当时的正文差异。
旧数据不补造创建者，后续编辑者只列为贡献者。删除条目后服务库仍保留历史。

跨库写入先在服务库保存 pending 记录，再提交小说事务，最后确认历史为 completed；
准备记录失败会回滚小说写入。只有 completed 记录用于历史页与贡献统计，
内容提交与历史确认之间发生崩溃或服务库中断时，pending 记录保留供核对，不冒充已完成记录。

ATHub 保留独立 `go.mod`、存储实现、测试和 Prisma 迁移；根模块通过本地 `replace` 引用。
维护内容功能时修改该目录，再重新构建统一程序。普通 Git 克隆即可取得全部源码，
不需要相邻 ATHub 目录或初始化子模块。原 ATHub 工作目录保留。

v2 的外部调用使用登录用户在 `8001/api/v1/account/key` 创建的个人 API Key，不接受原全局密钥、
OAuth access token 或账户 Cookie。每个 Key 归属一个用户，创建时仅返回一次明文，账户库只保存哈希。
账户维护模式沿用 v1 范围；v2 内容请求不经过账户维护拦截。
`8001/healthz` 检查账户库，账户维护状态仍返回 `433`；`8003/healthz` 检查内容库，
数据库不可用返回 `503`。任一端口绑定失败会释放已绑定端口并退出整个程序。

本项目提供 OAuth 2.0 服务，**不签发 OpenID Connect ID Token**。访问令牌是不透明字符串；应用通过 `/api/v1/user` 获取用户身份，不应将令牌当作 JWT 解码。

## ATHub 用户密钥与调用记录

登录前端后，点击顶栏的钥匙图标，或访问 `/account/key`，首次进入时自动生成唯一的永久 Key，
可复制、重置和停用。调用统计提供近 24 小时、近 7 天、近 30 天的平滑趋势图、
接口次数和最近调用分页记录，页面不展示端口号或有效期字段。完整 Key 仅在自动生成或重置后显示一次，丢失后需重置。

以下账户接口在 OAuth `8001` 端口，要求有效登录会话；创建和撤销还需
`X-AlbaTextus-Client: account-center`，不接受跨站请求。只能管理自己的 Key 和查看自己的记录。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/v1/account/key` | 提交 `{}` 自动取得自己的唯一 Key；首次生成返回 201 和一次性 `api_key`，已存在时返回 200，仅返回 `key` 元数据。可选 `name`，不接受有效期。 |
| GET | `/api/v1/account/key` | `keys` 数组只包含自己的当前 Key（含停用状态）；不返回密钥明文或哈希。 |
| POST | `/api/v1/account/key/reset` | 提交 `{}`，原子停用旧 Key 并生成永久有效的新 Key，201 返回一次性明文；保留旧 Key 与调用历史。 |
| DELETE | `/api/v1/account/key/{id}` | 停用自己的 Key，保留调用历史。 |
| GET | `/api/v1/account/athub/usage` | 总次数、成功/失败/待完成次数，以及方法、接口、端口和状态码分组统计；可选 `?days=1\|7\|30` 获取时间范围内的数据及按端口聚合的趋势。 |
| GET | `/api/v1/account/athub/calls?limit=50&offset=0` | 分页调用记录；limit 1–100，offset 0–1000000。 |

Key 为 `ath_` 加 32 个随机字节的 Base64URL 编码，账户库仅保存 SHA-256 哈希和显示前缀。
每个用户只有一个当前 Key，默认永久有效。停用后拒绝后续调用，刷新或重复自动申请不会恢复启用；
用户可主动重置以生成新 Key。重置时旧 Key 立即失效，旧 Key 仅用于保留调用归属，无法再使用。
管理员可在用户详情的「API 密钥」区域禁用 Key，用户无法通过重置或自动申请绕过限制。
管理员禁用会设置账户级 `users.athub_disabled`，并停用该用户已有 Key；仅获授权的管理员能解除限制。
封禁账户时，在同一事务中同步设置此限制、停用 Key 并撤销登录。解封账户不会自动解除 Key 限制，
须由管理员单独解除，之后用户可重置生成新 Key；旧 Key 不会自动恢复。
鉴权每次查询账户启用状态、管理员禁用状态及 Key 状态，并在开始记录调用时重新校验，
避免鉴权查询与禁用操作之间的并发请求绕过限制。已经授权并开始执行的请求可以完成。
调用 ATHub 时使用 `Authorization: Bearer <api_key>`；这是调用方的个人凭据，
不再需要配置宿主全局 `ATHUB_API_TOKEN`。

根目录 `prisma/` 定义 `athub_api_keys` 和 `athub_api_calls`，均存入账户 `DATABASE_URL`。
内容库 `ATHUB_DATABASE_URL` 只保存平台、作者、小说、卷和章节。
内容请求先在账户库写入待完成记录，再执行内容读写，最后补齐状态码、耗时和响应字节数。
统计按调用次数计数，不把上传的章节数量当成调用次数；可归属 Key 的失败调用也计数。
缺失/未知 Key 的失败调用以匿名记录保存，不出现在任何用户的个人统计里。
不保存 Authorization、查询字符串、请求体和小说正文；未知路由归类为 `/api/v2/{unknown}`。
健康检查和账户管理请求不计入内容调用次数。账户库无法校验或无法开始记录时，
返回 `503` 并停止内容业务；完成记录失败或进程中断时，保留 `status=null` 的待完成记录。
Key 目前拥有共享内容库读写权限，用户归属用于鉴权与统计，不会自动按用户隔离小说。

不传 `days` 时保持全量统计；传入时，响应增加 `from`、`until`（左闭右开，UTC）、
`bucket_seconds` 和 `trend`（`started_at`、`port`、`calls`）。
`days=1` 使用最近 24 个小时桶，`days=7/30` 使用最近 7/30 个 UTC 日桶，均包含当前桶。
没有调用的桶由前端补零，趋势从全部匹配记录聚合，不受最近调用分页限制。
端口取自服务端监听配置，目前内容调用记录来自 `8003`，账户 `8001` 的管理操作不计入。

部署先在账户库应用 `20261110000000_athub_user_keys` 迁移，再更新统一程序。
迁移只新增两张表与索引，不修改内容库和用户数据。服务启动会检查这些表是否就绪。
单 Key 模式还需应用 `20261110010000_athub_single_key`：新增 `is_current` 与每用户当前 Key 的部分唯一索引。
已有多 Key 时保留最近一个可用 Key（没有可用 Key 时保留最近一个停用 Key），其余停用并归档，
当前 Key 清除有效期，原已过期 Key 保持停用，所有历史调用记录保留。
管理员禁用功能还需应用 `20261110020000_athub_admin_disable`，为已有封禁账户同步禁用 Key。

管理员接口沿用账户会话、来源校验、角色层级与 `updated_at` 版本检查：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/v1/admin/users/{id}/athub-key` | 查询指定用户的 Key 元数据、`admin_disabled`、`account_active` 和 `updated_at`，不返回明文或哈希。 |
| PUT | `/api/v1/admin/users/{id}/athub-key` | 提交 `{"disabled":true,"updated_at":"..."}` 禁用，`false` 解除限制。封禁账户须先解封；不可操作自己的 Key 管理权限。 |

普通管理员仅管理普通用户，超级管理员可管理其他角色。用户端重置被管理员禁用的 Key 返回
`403 athub_admin_disabled`，封禁账户的内容调用返回 `401 invalid_api_key`。

## 服务拆分

本仓库曾使用 `AlbaTextus/connect`、`AlbaTextus/oauth` 名称，现统一命名为
`AlbaTextus/atnexus`，负责账户、认证、OAuth 授权与 ATHub 内容存储。
积分、支付、管理调整与 24 小时结算已迁移至独立仓库 [AlbaTextus/credit](https://github.com/AlbaTextus/credit)，使用独立数据库。
账户中心的积分入口和旧支付页面跳转到 `NEXT_PUBLIC_CREDIT_URL`（本地默认 `http://localhost:3300`）。
钱包及支付 API 不再由本服务提供；调用方应切换到 Credit。

Credit 在应用管理中注册为机密 OAuth 客户端：回调地址 `http://localhost:3300/api/v1/auth/callback`，
授权类型 `authorization_code`，scope `profile`，生产环境将地址换为 Credit 的 HTTPS 域名。
`profile` 用户信息包含稳定 UUID、数字用户编号、昵称、头像及角色；Credit 不接收账户密码或登录 Cookie。

历史迁移文件保持原样，升级使用 `pnpm db:deploy`。旧钱包表保留为只读迁移来源，
不会自动删除、清空或在本服务继续结算。迁移步骤见 Credit 的 `MIGRATION.md`。
旧数据库的 `prisma migrate dev` 可能提出删除已移出的表，完成迁移及核对前不要接受这种删除。

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

以下命令在仓库根目录执行。需要 Go 1.26.1+、Node.js 24+、pnpm 和 PostgreSQL；Redis 为可选的会话缓存。仓库通过 `packageManager` 固定 pnpm 版本；Docker Compose 使用 PostgreSQL 16 和可选的 Redis 7。

### 1. 准备环境

```bash
cp .env.example .env
pnpm install --frozen-lockfile
npm run db:athub:install
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
ATHUB_DATABASE_URL=postgresql://athub:athub@localhost:5433/athub?sslmode=disable
HTTP_ADDR=:8001
ATHUB_HTTP_ADDR=:8003
ISSUER_URL=http://localhost:3288
AUTH_UI_URL=http://localhost:3288
COOKIE_SECURE=false
WEBAUTHN_RP_ID=localhost
WEBAUTHN_RP_ORIGINS=http://localhost:3288
```

### 2. 初始化数据库

已有 PostgreSQL 时，分别配置账户库 `DATABASE_URL` 和内容库 `ATHUB_DATABASE_URL`。
接入已有 ATHub 数据时直接使用原内容库连接地址，不需要复制小说数据；个人 Key 在账户库重新创建。
也可以使用仓库提供的两个数据库容器：

```bash
docker compose up -d postgres athub-postgres
```

如需启用 Redis 会话缓存，在 `.env` 设置 `REDIS_ENABLED=true`，再启动可选容器；不开启时 Go 服务不会连接 Redis：

```bash
docker compose --profile redis up -d redis
```

数据库就绪后执行：

```bash
pnpm db:deploy
npm run db:athub:deploy
pnpm db:seed
```

`db:deploy` 应用全部已提交的迁移。`db:seed` 是开发初始化命令，会创建或更新以下账户和示例客户端：

`db:athub:deploy` 只迁移内容库。迁移前备份数据库；其中保留的
`20261103000000_remove_user_api` 会删除旧 `ApiUser`、`ApiKey` 和 `ApiUsage` 表及数据，
保留内容数据。统一服务启动只检查两套数据库，任何一套初始化失败都不会开始监听。

| 项目 | 默认值 |
| --- | --- |
| 管理员邮箱 | `admin@example.com` |
| 管理员密码 | `Admin123!ChangeMe` |
| 管理员角色 | `ADMIN` |
| 示例 Client ID | `albatextus-web` |
| 示例 Client Secret | `development-secret-change-me` |
| 示例回调 | `http://localhost:3288/auth/callback` |

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

访问 [首页](http://localhost:3288/)、[登录页](http://localhost:3288/login) 或 [用户中心](http://localhost:3288/account)。OAuth 默认监听 `8001`，ATHub 默认监听 `8003`，两个端口均提供 `/healthz`。内容接口例如 `http://localhost:8003/api/v2/content?novelId=1&order=1`。

账户前端通过 `localhost:3288` 访问：Next.js 仅将 `/api/v1/*`、`/oauth/*` 和 `/.well-known/*` 转发至 `OAUTH_BACKEND_URL`（默认 `http://localhost:8001`），使账户页面与登录会话保持同源。Key 管理和调用统计属于账户接口，继续由该入口提供。不要混用 `localhost` 和 `127.0.0.1`，否则 Cookie 和 Passkey 的来源校验可能不一致。

小说内容 `/api/v2/*` 不在前端域名下提供，也不经过 Next.js 转发。调用方直接访问 `https://api.albatextus.com/api/v2/*`，由外部反向代理转发到后端 `8003`；本地可直接请求 `http://localhost:8003/api/v2/*`。前端不需要配置或连接 ATHub 内容端口。

`pnpm web:dev` 默认使用 Turbopack。中文字体的 `regular.css` 和 `medium.css` 在 `web/app/layout.tsx` 中作为独立 CSS 入口加载，不要再通过 `web/styles/fonts.css` 的 `@import` 合并。Windows 本地环境中，两套分片字体及其 source map 合并为大型 PostCSS IPC 结果时会卡住首次编译；分离入口保留相同的字体、字重和样式，并缩小单次转换结果。

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
| GitHub OAuth App | `GITHUB_CLIENT_ID`、`GITHUB_CLIENT_SECRET` | `http://localhost:3288/api/v1/auth/providers/github/callback` |
| [LINUX DO Connect](https://connect.linux.do/) | `LINUXDO_CLIENT_ID`、`LINUXDO_CLIENT_SECRET` | `http://localhost:3288/api/v1/auth/providers/linuxdo/callback` |
| 自定义 OAuth | `CUSTOM_OAUTH_CLIENT_ID`、`CUSTOM_OAUTH_CLIENT_SECRET` 及端点 URL | `http://localhost:3288/api/v1/auth/providers/custom/callback` |

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
| `/admin/integration`、`/account/integration`、`/wiki/*` | 跳转到独立的 [ALBATEXTUS Wiki](https://github.com/AlbaTextus/wiki) 接入指南与对应文档 |
| `/admin/legal` | 跳转到 Wiki |
| `/privacy`、`/terms` | 跳转到 Wiki 上的公开协议正文；协议内容仍由本服务的 `/api/v1/legal/{slug}` 提供 |
| `/consent` | OAuth 应用授权确认 |

账户采用四级等级：1 级普通用户（`USER`）、2 级用户（`LEVEL_2`）、3 级管理员（`ADMIN`）、4 级超级管理员（`SUPER_ADMIN`）。用户接口保留 `role`，同时返回由角色确定的数字 `level`（1、2、3、4）。2 级可分配、筛选、设置为应用访问门槛，并单独配置 Key 累计总调用上限，默认不限次；不具备管理后台权限。1、2 级用户管理自己的账户；3 级管理员可管理应用、协议及 1、2 级用户；4 级超级管理员可管理全部角色。管理操作禁止修改自己的角色、停用自己，或通过后台重置自己的密码和撤销自己的登录。开放 2 级需执行 `pnpm db:deploy` 应用数据库迁移；迁移保留已有账户角色及各等级的 Key 限额配置。

修改密码、变更角色、停用账户或撤销登录会使相关中心会话与 OAuth 凭据失效。子系统自己建立的本地会话需要由子系统处理失效与退出。

## 应用接入

### 登记客户端

在 `/admin/applications/new` 创建应用，填写名称、主页、回调地址、描述和最低访问角色。回调地址精确匹配，不支持通配符；生产使用 HTTPS，本地回环地址允许 HTTP。

`/admin/settings` 分别提供 Key 总调用限制与接口权限配置。默认 LT1 仅可 GET 查看、读取资源；LT2 可上传、新建和修改，但提交需审核后生效；LT3 可直接上传、修改并审核，不能删除；LT4 拥有全部操作。接口权限只配置允许／禁止及审核要求，具体接口按方法与路径覆盖分类开关，不再设置或执行分类、具体接口的次数限制，旧配置中的次数上限也不再生效。网页登录的接口权限同样生效。Key 累计总调用限制保留，默认不限次。接口权限由超级管理员保存，管理员可查看。

作品详情「贡献者」标题右侧的红色铅笔进入原位编辑模式，点击可编辑内容即可修改，也可在浏览状态双击内容直接编辑。作品名称、来源平台、关联作者、连载状态、封面链接、简介和标签可编辑；字数、章节数、更新时间和贡献记录保持只读。顶部操作栏统一保存或取消资料修改，需审核时提交后继续展示正式资料。`/admin/settings` 的「接口权限 → 作品资料编辑」可按等级选择继承、允许或禁止，审核要求沿用该等级的「内容写入（含作品资料编辑）需审核后生效」。浏览器通过 `PUT /api/v1/account/athub/novels/{id}` 提交，服务端绑定登录会话及作品编号，复用 `PUT /api/v2/novelinfo` 的权限、审核和变更历史；只读内容凭据不能用于写入。

编辑模式下，插图面板上方为展示画布，下方为当前作品的图片资源。将资源拖入画布后，拖动图片块内部调整位置，拖动四边或四角调整大小；靠近其他块的边缘时吸附并显示对齐线。移出画布保留图片资源，删除资源则同时清理画布引用。布局可在插图面板单独保存，也随顶部「保存修改」一起提交，取消修改还原未保存布局。浏览状态按已保存布局展示；未设置布局的作品沿用图片列表。

插图资源上传使用 `POST /api/v1/account/athub/novels/{id}/illustrations`，删除使用 `DELETE /api/v1/account/athub/novels/{id}/illustrations/{illustrationId}`；资源操作单独提交，资料编辑的取消不会撤销已经提交的上传或删除。布局通过 `GET /api/v2/illustration-layout?novelId=…` 读取，浏览器以 `PUT /api/v1/account/athub/novels/{id}/illustration-layout` 保存，内部复用 `PUT /api/v2/illustration-layout?novelId=…` 的权限、审核和贡献历史。保存校验作品资源归属与布局版本；旧版本返回 409，避免覆盖他人的调整。待审核资源或布局继续展示正式数据。上线前应用模块迁移 `20261115010000_illustration_layout`。

需审核的内容写入返回 HTTP `202` 以及 `review_id`、`status: "pending"`，此时正式书库尚未变化。书库头像菜单内的审核入口位于 `/library/reviews`，LT3、LT4 可查看提交并通过或驳回；不能审核自己的提交，也不能借审核执行自己无权进行的操作。通过后应用原提交，保留提交者作为历史贡献者；驳回原因与审核结果会保存。重复或并发审核不会重复写入内容。提交者可通过 `/api/v1/account/athub/submissions` 及 `/{id}` 查询自己的状态和结果，审核接口为 `/api/v1/admin/athub/reviews`。

权限配置接口为 `GET/PUT /api/v1/admin/athub/permissions`，当前账户能力接口为 `GET /api/v1/account/athub/permissions`。启用此功能需分别运行 `pnpm db:deploy` 和 `pnpm db:athub:deploy`，应用账户库权限／审核迁移及内容库幂等回执迁移，然后更新前后端；迁移不改变现有账户等级和 Key 总调用限额。

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
http://localhost:3288/oauth/authorize
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
curl http://localhost:3288/oauth/token \
  --user 'YOUR_CLIENT_ID:YOUR_CLIENT_SECRET' \
  --data-urlencode 'grant_type=authorization_code' \
  --data-urlencode 'code=AUTHORIZATION_CODE' \
  --data-urlencode 'redirect_uri=YOUR_REGISTERED_CALLBACK' \
  --data-urlencode 'code_verifier=YOUR_ORIGINAL_VERIFIER'

curl http://localhost:3288/api/v1/user \
  --header 'Authorization: Bearer YOUR_ACCESS_TOKEN'
```

公开客户端省略 `--user`，改为提交 `client_id`，并保留 `code_verifier`。授权确认请求约 10 分钟有效，绑定当前用户与登录会话，只能处理一次；每次应用授权都需要用户确认。

用户信息始终包含 `id` / `sub`。申请 `profile` 才返回昵称、头像、角色等资料，申请 `email` 才返回邮箱及登录标识说明。请按需申请权限。

### 刷新、撤销与自省

```bash
curl http://localhost:3288/oauth/token \
  --user 'YOUR_CLIENT_ID:YOUR_CLIENT_SECRET' \
  --data-urlencode 'grant_type=refresh_token' \
  --data-urlencode 'refresh_token=YOUR_REFRESH_TOKEN'
```

刷新令牌会轮换，成功后需同时保存新的访问令牌和刷新令牌。只有允许 `refresh_token` 的应用才会获得刷新令牌。撤销使用 `POST /oauth/revoke`，查询令牌状态使用 `POST /oauth/introspect`；两者均提交 `token` 并进行客户端认证。

服务元数据位于 `/.well-known/oauth-authorization-server`。客户端密钥重置、回调地址或最低角色变更、应用启停等操作会撤销相关已有令牌与未使用的授权码。

## 配置与部署

### 关键环境变量

Go 服务从仓库根目录 `.env` 加载配置，已有进程环境变量优先。前端的 `OAUTH_BACKEND_URL`、`NEXT_PUBLIC_WIKI_URL` 和 `NEXT_OUTPUT_DIR` 需单独传给 Next.js 进程。

| 变量 | 用途 |
| --- | --- |
| `DATABASE_URL` | PostgreSQL 连接地址，必填 |
| `ATHUB_DATABASE_URL` | 内容库 PostgreSQL 连接地址，必填，与账户库分开 |
| `REDIS_ENABLED` | 是否启用 Redis 会话缓存，默认 `false`；关闭时不建立 Redis 连接 |
| `REDIS_URL` | Redis 连接地址，默认 `redis://localhost:6379/0`；支持 `redis://` 与 `rediss://` |
| `REDIS_KEY_PREFIX` | Redis 键前缀，默认 `albatextus:connect` |
| `REDIS_SESSION_TTL` | 会话缓存时长，默认 `30s`，且不会超过会话剩余有效期 |
| `CLIENT_SECRET_ENCRYPTION_KEY` | 客户端密钥与 PKCE verifier 加密密钥，必填并保持稳定 |
| `APP_ENV` | 本地为 `development`，生产为 `production` |
| `HTTP_ADDR` | OAuth 监听地址，默认 `:8001` |
| `ATHUB_HTTP_ADDR` | ATHub 监听地址，默认 `:8003` |
| `ISSUER_URL` | 对外 OAuth 地址；本地应为 `http://localhost:3288` |
| `AUTH_UI_URL` | 认证前端对外地址 |
| `COOKIE_SECURE` | HTTPS 部署设为 `true` |
| `SESSION_TTL` | 中心会话有效期，默认 `24h` |
| `AUTHORIZATION_CODE_TTL` | 授权码有效期，默认 `5m` |
| `WEBAUTHN_RP_ID` | Passkey 所属域名，不带协议和端口 |
| `WEBAUTHN_RP_ORIGINS` | 允许的完整前端 Origin，逗号分隔 |
| `PASSKEY_CHALLENGE_TTL` | Passkey 挑战有效期，默认 `5m` |
| `OAUTH_BACKEND_URL` | Next.js 转发到账户功能的地址，默认 `http://localhost:8001` |
| `NEXT_PUBLIC_WIKI_URL` | 独立 Wiki 服务的公开地址，用于文档、接入指南与协议链接及 `/wiki/*` 跳转，默认 `http://localhost:3200`；构建时内联，修改后需重新构建 |
| `NEXT_OUTPUT_DIR` | Next.js 构建目录，相对于 `web/`，默认 `.next` |

其他配置见 [.env.example](.env.example) 和 [配置实现](internal/config/config.go)。修改代理后端地址时，生产前端需要重新构建，以更新 rewrites。

### Docker Compose

完成快速开始中的环境配置、数据库迁移和初始化后，可以由 Compose 启动完整服务：

```bash
docker compose up --build -d
```

当前 [docker-compose.yml](docker-compose.yml) 是本地开发配置，固定使用本地地址和开发环境，并不自动执行迁移。Go 容器只接收 `environment` 中列出的变量：**目前未透传 `TURNSTILE_*` 和 `CUSTOM_OAUTH_AVATAR_FIELD`**，使用这些功能时需在 Compose 配置或覆盖文件中补充。只修改根目录 `.env` 不会把未列出的变量自动注入容器。

Compose 项目与后端服务均名为 `atnexus`。数据库名称、账号及已有连接配置继续沿用；
默认复用改名前的 `oauth_oauth-postgres`、`oauth_athub-postgres` 数据卷。
如果旧部署使用了自定义 Compose 项目名或卷名，启动前将 `ACCOUNT_POSTGRES_VOLUME`、
`ATHUB_POSTGRES_VOLUME` 设为实际已有卷名。升级时先停止旧 Compose 服务，再启动新服务，
避免端口冲突；停止旧服务时保留数据卷，不使用 `down -v`。

Redis 服务位于 Compose 的 `redis` profile 中，默认不会启动。启用缓存时使用 `docker compose --profile redis up --build -d`；禁用时继续使用普通的 `docker compose up --build -d`。Compose 内部默认连接 `redis://redis:6379/0`，如需覆盖可设置 `REDIS_COMPOSE_URL`。

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

Go 服务可用 `go build -o ../release/atnexus ./cmd/server` 构建，Windows 可将输出命名为 `../release/atnexus.exe`。运行时在仓库目录提供 `.env` 或进程环境变量。已有实例升级只需应用新迁移并更新服务，不需要重复 seed。

正式部署应使用同源 HTTPS 入口，正确设置 `ISSUER_URL`、`AUTH_UI_URL`、`WEBAUTHN_RP_ID`、`WEBAUTHN_RP_ORIGINS` 和 `COOKIE_SECURE=true`。反向代理保留 `/api/`、`/oauth/`、`/.well-known/` 的转发；健康检查直接请求 Go 的 `/healthz`。启用 CSP 时，为 Turnstile 放行 `https://challenges.cloudflare.com` 的脚本与 iframe。

数据库迁移应作为独立发布步骤执行。加密密钥须与数据库一起备份、多实例保持一致；已有客户端只有哈希而没有加密副本时，需重置后才能在后台查看明文。不要在每次部署时重新生成加密密钥，也不要在生产使用 seed 的默认凭据。

## API

账户与 OAuth 路由以 [internal/httpapi/api.go](internal/httpapi/api.go) 为准；
内容路由见 [ATHub 模块接口](modules/athub/README.md#http-接口)。
以下路径中的 `{id}`、`{provider}` 为参数；协议 `{slug}` 仅支持 `privacy`、`terms`。

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

## 消息功能下线迁移

后台邮件推送、全站公告和消息中心已移除，包括管理页面、公开公告接口及后台发信队列。注册、验证和绑定邮箱仍使用原有 SMTP 配置；验证码邮件保留品牌 HTML 模板和纯文本备份。

部署时先停止旧版 Go 服务，备份需要保留的历史消息，再执行 `pnpm db:deploy`，最后启动新版 Go 服务并更新前端。新增的 `20261101000000_remove_communications` 迁移会永久删除 `mail_deliveries` 和 `communications` 表及其中的消息、发送记录；历史迁移保持不变，不影响邮箱验证数据。外部服务若仍使用旧公告接口，需要同步移除入口和轮询。

## Linux AMD64 构建

支持 Windows 原生构建，无需 WSL 或 Docker。构建机安装 Go（版本见 `go.mod`）、
Node.js 20.9 或更新版本、Corepack 和 `tar`（Windows 自带），在项目根目录执行：

```bash
node scripts/build-linux-amd64.mjs
# 可指定发布包名称；同名产物已存在时会退出，避免覆盖。
node scripts/build-linux-amd64.mjs atnexus-linux-amd64-v1
# 也可通过项目命令执行。
pnpm build:linux-amd64
```

Windows 也可使用 PowerShell 入口：

```powershell
.\scripts\build-linux-amd64.ps1
.\scripts\build-linux-amd64.ps1 -Name atnexus-linux-amd64-v1
```

Linux 可运行 `bash scripts/build-linux-amd64.sh`。Corepack 未安装时先运行
`npm install --global corepack`。构建前可设置 `OAUTH_BACKEND_URL`、
`NEXT_PUBLIC_ATHUB_API_URL`、`NEXT_PUBLIC_WIKI_URL` 和 `NEXT_PUBLIC_CREDIT_URL`。
默认后端地址为 `http://127.0.0.1:8001`，内容 API 为 `https://api.albatextus.com`，
Wiki 和 Credit 分别为 `http://localhost:3200`、`http://localhost:3300`。
本地 `.env` 文件不参与构建，公开地址和 Next.js 转发目标修改后需要重新构建。

脚本复制当前工作目录源码（包括未提交的修改）到 `temp/`，安装锁定依赖并构建，
退出时清理临时目录。开发依赖与前端构建目录不受影响。产物位于项目的 `release/`：

- `<名称>/`：静态 Go 后端、Next.js 前端、Linux AMD64 生产依赖、启动脚本、
  两套数据库迁移与 `.env.example`。
- `<名称>.tar.gz` 和 `<名称>.tar.gz.sha256`：完整发布包与校验文件。

目标服务器需要 Linux AMD64、glibc 和 Node.js 20.9 或更新版本（推荐 Node.js 24），
不适用于 Alpine/musl。构建包不附带 Node.js。前端先在构建机编译，再独立安装 Linux 的
生产依赖，不打包 Windows 开发依赖。部署后保留已有 `.env`，
执行 `chmod +x atnexus start-backend.sh start-frontend.sh`，
分别运行 `./start-backend.sh`、`./start-frontend.sh`；
前端默认监听 `3288`，可通过 `HOSTNAME_OVERRIDE` 和 `PORT` 调整。
脚本不自动执行数据库迁移或 seed；迁移仍按现有部署流程执行。

## 开发

```text
cmd/                  服务入口、开发 seed 与本地图床接入工具
internal/             配置、HTTP API、OAuth、Passkey、第三方登录、存储
modules/athub/        独立维护的内容模块、测试及内容库迁移
prisma/               数据库 schema 与迁移
web/app/              Next.js 页面与全局样式
web/components/       账户、管理后台与基础 UI 组件
web/lib/              前端 API 与共享逻辑
temps/                本地临时脚本、检查与日志
release/              本地构建与发布产物
```

| 命令 | 用途 |
| --- | --- |
| `npm run server:dev` | 一个进程启动 OAuth 8001 与 ATHub 8003 |
| `npm run test:go` | 运行宿主与 ATHub 两个 Go 模块的测试 |
| `npm run db:athub:deploy` | 应用内容库迁移 |
| `pnpm web:dev` | 前端开发服务 |
| `pnpm web:typecheck` | TypeScript 类型检查 |
| `pnpm db:validate` | 校验 Prisma schema |
| `pnpm db:format` | 格式化 Prisma schema |
| `pnpm db:migrate` | 在开发数据库创建、应用迁移 |
| `pnpm db:deploy` | 应用已提交的迁移 |
| `pnpm db:studio` | 打开数据库管理界面 |
| `go test ./...` | 仅运行宿主 Go 模块测试；不遍历独立 ATHub 模块 |

Prisma schema 与迁移是数据库结构的来源；Go 服务没有 Prisma Client，也不会在启动时自动建表。首次检查前端或更换构建目录时，先完成一次前端构建，生成 Next.js 路由类型。

相邻的 ALBATEXTUS 图床可通过 `go run ./cmd/register-imgbed` 联调注册。该工具会登记客户端并写入相邻图床的本地配置，运行前请核对 [工具源码](cmd/register-imgbed/main.go) 中的目录、端口与回调约定。

相邻的 Status 后台可通过 `go run ./cmd/register-status` 接入当前 OAuth。工具创建独立机密应用 `albatextus-status`，最低角色为 `ADMIN`，仅申请 `profile`；默认回调为 `http://127.0.0.1:3100/api/auth/callback`，凭据写入 `../status/.dev.vars`。应用所有者使用 `SEED_ADMIN_EMAIL` 对应的现有管理员，也可在首次登记时用 `-admin-id <UUID>` 指定。Status 默认按服务端用户资料中的 `role` 允许 `ADMIN`、`SUPER_ADMIN`，符合条件的其他账号也能登录；OAuth 应用更严格的 `SUPER_ADMIN` 最低角色会保留。支持 `-app-url`、`-issuer-url`、`-output`；重复运行保留已有密钥、回调、Status 管理密钥和自定义允许名单。旧版仅允许所有者 UUID 的配置，可运行 `go run ./cmd/register-status -migrate-owner-allowlist` 切换到角色授权；该选项拒绝自动清理自定义账号名单。登记后在 Status 执行 `npm run db:migrate:local` 并重启开发服务。工具不会重置用户、运行 seed 或修改其他应用。

目前未提供邮箱验证码绑定、OIDC ID Token 或跨子系统统一登出。功能问题和接入反馈可提交到 [GitHub Issues](https://github.com/AlbaTextus/atnexus/issues)。

## UI 与字体维护

Web 的组件、主题、字体与页面展示样式在 atnexus 项目内独立维护，不再依赖 `@albatextus/ui` 包或相邻的 ATUI 仓库。迁移使用此前实际安装的组件版本，保留现有外观与交互；独立 ATUI 项目不受影响。基础组件实现位于 `web/components/ui/`，其他展示组件位于 `web/components/`，带业务反馈或路由适配的组件继续包装 `web/components/presentation/` 中的本地实现。

应用与用户管理使用共享 `StatCard`；列表使用 `EmptyState`；删除通行密钥、解绑账号、删除应用及重置密钥使用 `ConfirmDialog`。这些组合归在原有 card / empty / dialog 组件组中，不再在页面内重复维护骨架。请求、统计和名称二次确认仍在账户模块，密钥按需读取与隐藏清除的逻辑保持不变。

PasswordInput 与 CopyField 的静态密钥显隐共用本地 InputGroupPassword；ClientSecretField 继续独立负责按需读取及隐藏时清除服务端密钥，不将其改为预加载静态字段。底层 Base UI、动画和 Markdown 等依赖直接在 `web/package.json` 中声明。

首页授权示例使用 `web/components/code-highlighted/` 中的 `CodeHighlighted`，保留深色终端外观，只为该示例加载 HTTP 语法，不在全局初始化高亮引擎。个人简介及编辑预览中的 Markdown 代码块继续按需高亮，沿用全站明暗主题、中文复制反馈和纯文本回退。

`web/app/globals.css` 引入本地 `web/styles/oauth.css`，主题 token、页面样式与字体声明均保留在 `web/styles/`。字体文件和 OFL 许可证位于 `web/fonts/`，构建时自动生成静态资源地址，不依赖外部字体服务。修改后运行 `pnpm web:typecheck` 和 `pnpm web:build`；依赖更新时同步锁文件，部署使用 `pnpm install --frozen-lockfile`。Docker 构建只需 atnexus 自身目录，不再需要额外 UI 构建上下文。
