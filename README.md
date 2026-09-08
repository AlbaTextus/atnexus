# ALBATEXTUS

**一个账户，让连接更简单。**

AlbaTextus 是面向社区应用的账户中心与 OAuth 2.0 授权服务。用户可以管理个人资料、登录方式和应用授权；管理员可以管理用户、接入应用及公开协议。

后端使用 Go，前端使用 Next.js、React 和 shadcn/ui，数据存储于 PostgreSQL。Prisma 负责数据库结构与迁移，Go 运行时通过 pgx 访问数据库。

[快速开始](#快速开始) · [登录与账户](#登录与账户) · [应用接入](#应用接入) · [配置与部署](#配置与部署) · [API](#api) · [开发](#开发)

## 功能

| 模块 | 当前能力 |
| --- | --- |
| 账户认证 | 邮箱或登录标识与密码登录、用户注册、会话管理、记住邮箱 |
| 第三方登录 | GitHub、LINUX DO Connect、自定义 OAuth 平台，支持绑定与解绑 |
| 通行密钥 | WebAuthn / Passkey 注册、无用户名登录、凭据管理 |
| 人机验证 | Cloudflare Turnstile，服务端验证令牌 |
| 个人中心 | 个人资料、昵称、平台头像来源、密码及登录方式管理 |
| OAuth 服务 | 授权码、S256 PKCE、刷新令牌轮换、客户端凭证、令牌撤销与自省 |
| 授权确认 | 展示应用与申请权限，用户明确允许后才签发授权码 |
| 管理后台 | 应用接入、客户端密钥、最低访问角色、用户管理、协议编辑 |

本项目提供 OAuth 2.0 服务，**不签发 OpenID Connect ID Token**。访问令牌是不透明字符串；应用通过 `/api/v1/me` 获取用户身份，不应将令牌当作 JWT 解码。

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

登录页采用纯白卡片，顶部 `ALBATEXTUS` 字标在完成验证与协议确认后收拢为 `AT`。GitHub、LINUX DO 和通行密钥入口与密码登录共用验证状态。

启用 Turnstile 时，流程为：

1. 完成 Cloudflare 人机验证，保留片刻成功反馈。
2. 原验证区域显示「同意并继续」，下方提供隐私政策与用户协议链接。
3. 点击同意后收起验证区域，允许密码、第三方或通行密钥登录。

未启用 Turnstile 时，登录页仍要求点击协议确认。验证过期或登录尝试失败后，需要重新完成对应步骤。协议链接在新窗口打开，避免丢失正在填写的表单。

这里的协议确认是登录页的交互门槛，当前没有独立的服务端协议签署记录；它与 OAuth 应用授权确认是两个不同流程。

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

在「个人资料 → 我的头像」选择「自定义上传」，选图预览后点击「保存头像」。支持 PNG、JPEG、WebP、GIF，最大 5 MB、1600 万像素；静态图片生成最大 512 × 512 的 PNG，GIF 逐帧裁剪并保留动画、帧间隔和循环次数。GIF 最多 200 帧，各帧完整画面合计不超过 4000 万像素，裁剪结果也不超过 5 MB。裁剪预览接口只返回图片，点击保存才上传图床并更新账户。头像保存在图床的 `user/avatars/<用户 ID>` 分类下，用户中心、授权确认页和 OAuth 用户资料同步使用保存后的地址。切换到平台或默认头像会保留已上传的自定义头像，方便再次选择。

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

curl http://localhost:3000/api/v1/me \
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

Go 服务可用 `go build -o release/oauth ./cmd/server` 构建，Windows 可将输出命名为 `release/oauth.exe`。运行时提供 `.env` 或进程环境变量。已有实例升级只需应用新迁移并更新服务，不需要重复 seed。

正式部署应使用同源 HTTPS 入口，正确设置 `ISSUER_URL`、`AUTH_UI_URL`、`WEBAUTHN_RP_ID`、`WEBAUTHN_RP_ORIGINS` 和 `COOKIE_SECURE=true`。反向代理保留 `/api/`、`/oauth/`、`/.well-known/` 的转发；健康检查直接请求 Go 的 `/healthz`。启用 CSP 时，为 Turnstile 放行 `https://challenges.cloudflare.com` 的脚本与 iframe。

数据库迁移应作为独立发布步骤执行。加密密钥须与数据库一起备份、多实例保持一致；已有客户端只有哈希而没有加密副本时，需重置后才能在后台查看明文。不要在每次部署时重新生成加密密钥，也不要在生产使用 seed 的默认凭据。

## API

完整路由以 [internal/httpapi/api.go](internal/httpapi/api.go) 为准。以下路径中的 `{id}`、`{provider}` 为参数；协议 `{slug}` 仅支持 `privacy`、`terms`。

### OAuth 与公开接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/healthz` | Go 与数据库健康检查 |
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
| GET | `/api/v1/me` | 当前账户或 Bearer 用户身份 |
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

用户列表支持 `q`、`role`、`status`、`page`、`page_size`；更新用户需提交读到的 `updated_at`。协议以 Markdown 保存，更新需提交 `revision`。版本冲突会返回 `409`，应重新读取后编辑。

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

相邻的 AlbaTextus 图床可通过 `go run ./cmd/register-imgbed` 联调注册。该工具会登记客户端并写入相邻图床的本地配置，运行前请核对 [工具源码](cmd/register-imgbed/main.go) 中的目录、端口与回调约定。

目前未提供邮箱验证码绑定、OIDC ID Token 或跨子系统统一登出。功能问题和接入反馈可提交到 [GitHub Issues](https://github.com/AlbaTextus/oauth/issues)。
