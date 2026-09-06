# AlbaTextus OAuth

一个可运行的 OAuth 2.0 授权服务，使用 Go、PostgreSQL 和 Prisma schema。当前包含：

- 授权码模式（Authorization Code）与 S256 PKCE
- 刷新令牌轮换（Refresh Token Rotation）
- 客户端凭证模式（Client Credentials）
- 令牌撤销、令牌自省、Bearer 资源保护
- 用户注册、登录会话和 `/api/v1/me`
- 用户角色（`USER`、`ADMIN`、`SUPER_ADMIN`）及开发管理员初始化
- 独立 Next.js + shadcn/ui 认证前端：淡金色背景与居中白色卡片的邮箱登录、注册页面，支持密码显隐、中文错误提示及移动端适配
- 通行密钥（WebAuthn/Passkey）注册、无用户名登录和账户管理
- 用户中心：可用应用入口、个人资料、密码修改、通行密钥管理
- 第三方登录：GitHub、LINUX DO Connect、可配置 OAuth 服务；账号绑定/解绑与首次设置密码
- 管理员后台：OAuth 应用接入、最低访问角色、启停控制、客户端密钥生成与重置、接入指南
- Prisma migration/schema 作为数据库结构的唯一来源

## 为什么运行时使用 pgx

Prisma 官方没有 Go Client，社区的 Prisma Go Client 已停止维护。本服务使用 Prisma 管理 schema、迁移和 Studio，Go 运行时使用官方 PostgreSQL 驱动 pgx；这样不会将生产服务绑定到废弃的生成器，同时数据库模型仍由 Prisma 维护。

## 快速开始

需要 Go 1.25+、Node.js 20+ 和 PostgreSQL。复制环境文件后启动数据库并执行迁移：

```bash
cp .env.example .env
node -e "const fs=require('fs'); fs.writeFileSync('.env', fs.readFileSync('.env','utf8').replace(/^CLIENT_SECRET_ENCRYPTION_KEY=$/m, 'CLIENT_SECRET_ENCRYPTION_KEY=' + require('crypto').randomBytes(32).toString('hex')))"
pnpm install
pnpm db:deploy
go run ./cmd/seed
go run ./cmd/server
```

`pnpm db:seed` 会初始化开发管理员（可通过 `SEED_ADMIN_EMAIL`、`SEED_ADMIN_PASSWORD` 等环境变量覆盖）：

```text
email:    admin@example.com
password: Admin123!ChangeMe
role:     ADMIN
```

`ADMIN`、`SUPER_ADMIN` 可以进入管理后台，管理全部 OAuth 应用，包括历史 seed 应用。`USER` 只能使用个人用户中心；即使历史应用的 `owner_id` 指向该用户，也不能查看、创建、编辑、停用、删除应用或重置客户端密钥。

另开一个终端启动认证前端（默认 `http://localhost:3000`；根目录 `pnpm install` 已会安装 workspace 依赖）：

```bash
pnpm web:dev
```

浏览器访问 `http://localhost:3000/login`。前端通过 Next rewrites 将 `/api`、`/oauth` 和 `.well-known` 请求转发到 Go 服务的 `http://localhost:8080`；这样登录会话和 OAuth 授权可以在同一个浏览器域名下工作。

登录页以邮箱和密码为主要入口，保留通行密钥登录。“记住邮箱”只在登录成功后保存邮箱，不保存密码；取消勾选会立即清除已记住的邮箱。注册和登录页面切换时会保留 OAuth 回跳参数。

本地开发统一使用 `http://localhost:3000`，并将 `ISSUER_URL`、`AUTH_UI_URL`、`WEBAUTHN_RP_ORIGINS` 设置为该地址，`WEBAUTHN_RP_ID=localhost`。通行密钥的 RP ID 必须是域名；不要用 `127.0.0.1` 替代 `localhost`。应用管理页显示的接口地址来自 `ISSUER_URL`。

可设置 `NEXT_OUTPUT_DIR=release`，将前端构建产物放入 `web/release/`。使用该设置构建后，运行 `pnpm web:start` 时也需设置相同的环境变量。

没有本地 PostgreSQL 时可以使用 Docker Compose（本机需安装 Docker）。首次启动先迁移数据库，再启动 Go 与 Next.js 服务：

```bash
docker compose up -d postgres
pnpm db:deploy
pnpm db:seed
docker compose up --build
```

Compose 会启动 PostgreSQL、Go API（8080）和认证前端（3000）。生产环境应将 `pnpm db:deploy` 接入独立的 migration job。

## 用户中心与子系统接入

### 第三方账号登录

执行最新 `pnpm db:deploy` 后，在 `.env` 配置第三方凭据并重启 Go 服务。登录页、注册页和账户安全页共用服务端提供的平台状态；未配置的平台显示为“暂未开放”，不会发起无效跳转。凭据只在 Go 服务中使用，不要配置为 `NEXT_PUBLIC_*`。

GitHub：在 **Settings → Developer settings → OAuth Apps** 创建 OAuth App，填写：

- Homepage URL：`http://localhost:3000`
- Authorization callback URL：`http://localhost:3000/api/v1/auth/providers/github/callback`
- `.env`：`GITHUB_CLIENT_ID`、`GITHUB_CLIENT_SECRET`

LINUX DO：在 [Linux DO Connect](https://connect.linux.do/) 创建应用，把授权回调地址设为 `http://localhost:3000/api/v1/auth/providers/linuxdo/callback`，将 Client ID、Client Secret 填入 `.env` 的 `LINUXDO_CLIENT_ID`、`LINUXDO_CLIENT_SECRET`。授权、令牌和用户接口已固定为 `https://connect.linux.do/oauth2/authorize`、`https://connect.linux.do/oauth2/token`、`https://connect.linux.do/api/user`，与公开 Discovery 一致。使用 `openid profile email` 和 S256 PKCE。

其他 OAuth：设置 `CUSTOM_OAUTH_NAME`、`CUSTOM_OAUTH_CLIENT_ID`、`CUSTOM_OAUTH_CLIENT_SECRET`，以及授权、令牌、用户信息三个 URL。回调为 `http://localhost:3000/api/v1/auth/providers/custom/callback`。协议要求授权码模式、S256 PKCE、令牌请求使用 `client_secret_post`，用户信息接口接受 Bearer Token 并返回 JSON。字段映射默认 `sub`、`email`、`email_verified`（布尔值）、`name`，可在 `.env` 中修改，支持 `data.id` 等点分路径；`sub` 必须是平台稳定账号 ID，不能使用可修改的昵称或邮箱。生产环境将地址替换为 `AUTH_UI_URL` 的实际 HTTPS 域名。已有绑定后不要将同一 custom 配置切换到不同身份平台。

例如 Google 可使用授权地址 `https://accounts.google.com/o/oauth2/v2/auth`、令牌地址 `https://oauth2.googleapis.com/token`、用户信息地址 `https://openidconnect.googleapis.com/v1/userinfo`，scope 使用默认 `openid,profile,email`。此处通过 OAuth 访问令牌调用用户信息接口，不读取或信任未验证的 ID Token。

GitHub 和通用平台首次登录要求平台提供已验证的邮箱，并创建 `USER` 账户。相同邮箱不会自动合并或提升角色：先登录原账户，再到 **账户安全 → 第三方账号** 绑定。绑定要求授权回调仍在发起时的同一登录会话；同一平台账号只能绑定一个本地用户。没有已验证邮箱的通用平台账号可绑定已有用户，但不能直接注册。取消授权、状态过期、邮箱冲突均显示中文提示，子系统的授权回跳参数会保留。

LINUX DO 的 Discovery 没有声明 `email_verified`。因此新用户按稳定的 `id`（兼容 `sub`）创建独立账户，内部使用 `@accounts.albatextus.invalid` 保留域的专属登录标识，不把平台返回的邮箱当作已验证邮箱，也不会匹配已有邮箱用户。个人资料页将其标为“登录标识”，`/api/v1/me` 中 `email_is_alias=true` 表明它不是收件邮箱。设置密码后也可用该标识登录；应用应优先使用用户 `id` 识别身份。

第三方创建的账户最初没有密码，可在账户安全中设置首个密码；完成后撤销旧会话和授权并重新登录。解绑前必须设置密码。第三方访问令牌只在服务端取用户信息时短暂使用，不入库；授权状态绑定当前浏览器、10 分钟有效且只能消费一次，PKCE verifier 加密存储。

| 方法 | 接口 | 用途 |
| --- | --- | --- |
| GET | `/api/v1/auth/providers` | 平台名称与是否开放，不包含凭据 |
| POST | `/api/v1/auth/providers/:provider/start` | 开始登录；`link: true` 表示绑定当前登录账户 |
| GET | `/api/v1/auth/providers/:provider/callback` | 平台授权回调 |
| GET | `/api/v1/account/identities` | 当前用户绑定的第三方账号 |
| DELETE | `/api/v1/account/identities/:provider` | 解绑当前用户的账号 |
| PATCH | `/api/v1/account/avatar` | 选择当前用户的头像来源，提交 `source` |

第三方登录或绑定时会同步平台头像：GitHub 使用 `avatar_url`，LINUX DO 支持 `avatar_url`、`avatar_template`（替换尺寸及补全站点地址）和 `picture`。自定义 OAuth 默认读取 `picture`，可以用 `CUSTOM_OAUTH_AVATAR_FIELD` 指定嵌套字段，例如 `profile.photo`。头像地址仅允许公开 HTTPS 地址，平台令牌不会拼入图片链接；图片由浏览器加载，失败时显示默认头像。

个人资料页支持 `auto`（最先绑定且提供头像的平台）、`default`（昵称默认头像）或某个已绑定的平台 ID。每次登录会更新对应平台头像，但保留用户选定的来源。解绑选中的平台后恢复自动选择，无可用头像时回退到默认头像。`/api/v1/me` 返回 `avatar_source`、`avatar_url` 及同值的 `picture`，供子系统显示；绑定列表包含各平台的 `avatar_url`。升级执行 `pnpm db:deploy` 应用 `20260906020000_provider_avatars`，历史第三方账号下次通过该平台登录后同步头像。

邮箱绑定与验证码暂缓；自定义头像上传将在对接图床后提供。

修改请求及开始登录需要 `X-AlbaTextus-Client: account-center`，拒绝跨站请求。回调验证浏览器 Cookie、state 和 PKCE，并只返回受限的本地页面或子系统授权接口。所有响应禁止缓存。

登录后可访问：

- `/account`：个人概览和已开放的应用，支持搜索与访问应用主页。
- `/account/profile`：查看账户信息、修改昵称、选择头像来源；用户不能自行修改邮箱、身份或其他人的资料。
- `/account/security`：修改登录密码、添加或删除自己的通行密钥。
- `/admin/applications`：管理员应用列表，支持名称 / Client ID 搜索及启停状态筛选。
- `/admin/applications/new`：管理员创建应用，填写主页、描述、图标 URL 与回调地址。
- `/admin/applications/:id`：管理员查看接入信息、编辑配置、重置密钥、停用或删除应用。
- `/admin/integration`：管理员接入指南。

普通用户的导航不显示后台入口；直接访问后台也不会加载管理内容，管理 API 返回 `403 admin_required`。旧 `/account/applications`、`/account/integration` 及其子路径重定向到对应后台地址。

用户中心只列出已启用、支持授权码登录且满足当前用户最低角色要求的应用，不返回客户端密钥、回调配置等管理字段。点击“打开应用”访问子系统主页，OAuth 跳转与 PKCE 参数由子系统生成。未配置主页的历史应用显示待配置提示。

修改密码需要验证当前密码；成功后撤销该用户已有中心会话、OAuth 令牌和未使用的授权码，并要求重新登录。通行密钥保留。新增的用户中心功能沿用现有数据库字段，无需额外迁移。

新增数据库列通过 `20260905000000_application_management` 迁移提供。升级已有实例执行 `pnpm db:deploy` 即可；迁移保留原应用，未指定归属的历史应用仅管理员可管理，不需要重新 seed。

创建时选择服务端应用或公开客户端，创建后类型不可修改。服务端应用的 Client Secret 默认遮罩显示，管理员点击眼睛可查看、隐藏，也可直接复制；重新打开页面后仍可查看。数据库保留 bcrypt 哈希用于客户端认证，并通过 AES-256-GCM 加密保存可查看的副本；密文绑定应用 ID，明文只通过专用管理员接口按需读取，不在列表或详情接口中返回。公开客户端不使用 Secret，必须提供 S256 PKCE。新应用默认允许 `authorization_code`、`refresh_token`，权限为 `profile`、`email`。

升级此功能需执行 `pnpm db:deploy`，并为 Go 服务及 seed 配置 `CLIENT_SECRET_ENCRYPTION_KEY`（32 字节随机值的 64 位十六进制编码，可用 `.env.example` 中的命令生成）。该配置应保持稳定，与数据库一起妥善备份；所有实例使用同一密钥，不要在每次启动时重新生成。已有应用仅保存哈希时无法还原旧密钥，页面会提示重置后查看；升级不会自动重置或使旧凭据失效。

最低访问角色按 `USER < ADMIN < SUPER_ADMIN` 校验。修改回调地址、最低角色或启停状态，以及重置密钥，会撤销该应用已有令牌和未使用的授权码；修改名称、描述、图标不会撤销授权。删除应用同时删除相关授权数据。

子系统先生成随机 `state` 与 PKCE 参数，跳转授权接口；回调验证 `state`，用授权码交换令牌，再携带 Bearer Token 请求 `/api/v1/me`，以返回的 `id` 建立自己的用户会话。用户已登录中心时，再进入其他接入应用可以复用中心登录会话。

当前实现是 OAuth 2.0，不签发 OpenID Connect ID Token；请使用通用 OAuth 客户端，通过用户接口读取身份，不能把访问令牌当作 JWT 解码。中心退出或令牌撤销不会自动清除子系统自己的会话；子系统应在退出时清理自己的会话，并处理令牌失效。

## API

### 应用管理

以下接口要求有效的管理员登录会话；普通用户（包括历史应用所有者）统一返回 403。修改请求还需携带 `X-AlbaTextus-Client: account-center`。Bearer Token 不能用于管理应用。响应均为 `Cache-Control: no-store`。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET / POST | `/api/v1/applications` | 列表 / 创建 |
| GET / PUT / DELETE | `/api/v1/applications/:id` | 详情 / 完整更新 / 删除 |
| POST | `/api/v1/applications/:id/secret` | 重置机密客户端密钥 |
| GET | `/api/v1/applications/:id/secret` | 管理员查看密钥；同样要求 `X-AlbaTextus-Client` 请求头，拒绝跨站请求 |

创建和更新使用 JSON 字段 `name`、`homepage_url`、`description`、`icon_url`、`redirect_uris`（1–10 个）、`minimum_role`、`is_active`、`is_public`。主页与图标、回调需使用 HTTPS，本地回环地址允许 HTTP；不接受通配符、用户凭据或锚点。列表与详情不会返回客户端密钥。

### 个人账户

以下接口只操作当前登录用户，使用中心会话，响应为 `Cache-Control: no-store`。修改请求同样要求 `X-AlbaTextus-Client: account-center`。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/v1/account/applications` | 返回满足访问条件的应用展示信息 |
| PATCH | `/api/v1/account/profile` | 更新自己的 `display_name`（1–100 个字符） |
| POST | `/api/v1/account/password` | 用 `current_password` 验证后设置 `new_password`（12–72 UTF-8 字节） |

### 用户会话

```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H 'content-type: application/json' \
  -d '{"email":"alice@example.com","password":"correct horse battery staple"}'

curl -c cookies.txt -X POST http://localhost:8080/api/v1/sessions \
  -H 'content-type: application/json' \
  -d '{"email":"alice@example.com","password":"correct horse battery staple"}'
```

### 授权码 + PKCE

先登录并保存 cookie。`code_challenge` 是 `base64url(sha256(code_verifier))`，不带 `=`：

```text
GET /oauth/authorize?response_type=code&client_id=albatextus-web&redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fauth%2Fcallback&scope=profile%20email&state=xyz&code_challenge=...&code_challenge_method=S256
```

服务会把 `code` 和 `state` 302 回调到已注册的 redirect URI，然后交换：

```bash
curl -X POST http://localhost:8080/oauth/token \
  -u albatextus-web:development-secret-change-me \
  -d grant_type=authorization_code -d code=... \
  -d redirect_uri=http://localhost:3000/auth/callback -d code_verifier=...
```

刷新令牌：

```bash
curl -X POST http://localhost:8080/oauth/token \
  -u albatextus-web:development-secret-change-me \
  -d grant_type=refresh_token -d refresh_token=...
```

资源请求：

```bash
curl http://localhost:8080/api/v1/me -H 'Authorization: Bearer ACCESS_TOKEN'
```

元数据位于 `/.well-known/oauth-authorization-server`。撤销和自省分别使用 `/oauth/revoke`、`/oauth/introspect`，均要求客户端认证。

### 通行密钥

登录后打开 `/account/security`，点击“添加通行密钥”，浏览器会调用系统的指纹、Face ID、Windows Hello 或设备 PIN。登录页的“使用通行密钥登录”使用 discoverable credential，不需要先输入邮箱。

对应 API：

- `POST /api/v1/passkeys/registration/options`（登录会话）
- `POST /api/v1/passkeys/registration/finish`（`X-WebAuthn-Ceremony` 请求头）
- `POST /api/v1/passkeys/login/options`
- `POST /api/v1/passkeys/login/finish`（`X-WebAuthn-Ceremony` 请求头）
- `GET/DELETE /api/v1/passkeys`、`/api/v1/passkeys/:id`（登录会话）

## 生产注意事项

- 生产环境必须使用 HTTPS、`COOKIE_SECURE=true`，并将 `ISSUER_URL` 设置为外部 HTTPS 地址。
- `WEBAUTHN_RP_ID` 必须是前端域名（不带协议和端口），`WEBAUTHN_RP_ORIGINS` 填写允许的完整前端 Origin；本地开发默认分别为 `localhost` 和 `http://localhost:3000`。
- 不要使用示例客户端 secret；通过 secret manager 注入 `SEED_CLIENT_SECRET`，并限制 redirect URI 为精确匹配。
- 默认密码哈希为 bcrypt，密码最少 12 个字符；访问令牌和授权码只以 SHA-256 摘要存储。
- 应将 `pnpm db:deploy` 放入发布流水线，并定期清理过期的 authorization codes、sessions 和 tokens。
