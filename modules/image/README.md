# image：Go 图床后端

该模块由根 Go module 管理，通过 `cmd/server` 与 OAuth、creatplat、ATHub 在同一进程运行。业务 API 位于 `8002/api/v2`，文件仍以 `/file/<原文件 ID>` 访问，也支持 `/api/v2/file/<ID>`。原 imgbed 前端保持原样，本次没有迁移它的页面、构建或交互。

图片元数据使用独立 PostgreSQL 数据库；本地文件仍使用已有存储目录，S3 对象继续按渠道及原对象键读取。独立数据库可以建在已有 PostgreSQL 实例中，无需另开数据库服务。启动时检查实际数据库身份，拒绝将图床指向账户、内容或创作者库。

## 认证与接口

- 注入 OAuth 的 `auth.Verifier`，直接校验同一 Cookie session，不建立另一套图床登录会话。
- 普通已登录账户可上传；管理员可列表、管理、删除。修改请求携带 `X-AlbaTextus-Client: account-center`，来源受宿主 CORS/CSRF 约束。
- 外部 Key 在 OAuth 账户库按 SHA-256 存储，由 `GET/POST/PUT/DELETE /api/v1/admin/image/keys` 管理，创建时仅返回一次明文。`upload/list/delete/manage` 为独立权限。
- 支持 `Authorization: Bearer <Key>` 和旧版直接放置 Key 的格式。无效 Key 不回退到 Cookie；过期、撤销、权限不足均拒绝。
- `/file/` 读取公开，`Block` 文件禁止公开读取。`?from=admin` 预览要求管理员 session 或 `manage` Key。

| 能力 | 新接口（均加 `/api/v2`） |
| --- | --- |
| 普通/分块上传 | `POST /upload`，保留 `initChunked/chunked/merge/cleanup` 参数 |
| 分类 | `POST /categories/<分类>/upload`、`GET /categories/<分类>/list` |
| 目录、渠道、筛选 | `GET /directoryTree`、`/channels`、`/manage/list` |
| 删除 | `DELETE /manage/delete/<ID>`、`POST /manage/delete/batch` |
| 移动、重命名、封禁 | `/manage/move/<ID>`、`rename/<ID>`、`block/<ID>`、`unblock/<ID>`，使用 POST/PATCH |
| 元数据、标签 | `/manage/metadata/<ID>`、`tags/<ID>`、`tags/batch`、`tags/autocomplete` |
| 容量 | `/manage/quota`，从真实记录计算 |
| 存储配置 | `GET/POST /manage/sysConfig/upload`，读取隐藏 S3 凭据，修改时空凭据保留旧值 |
| IP 黑名单 | `GET/PUT /manage/cusConfig/blockipList` |
| 导出、记录恢复 | `/manage/batch/list`、`batch/settings`、`POST /manage/batch/restore/chunk` |

分类为 `user`、`user/avatars`、`user/backgrounds`、`book`、`book/covers`、`book/illustrations`、`forum`。新增文件使用 `user/<UUID>/avatars/...`、`book/<ID>/covers/...` 等结构，查询兼容旧分类在前的路径。上传返回数组中的 `src`；`returnFormat=full` 使用 `IMAGE_PUBLIC_URL`。

文件支持 GET、HEAD、Range，并直接流式响应。本地存储使用 `os.Root` 限制路径逃逸，独占保留和原子链接防止覆盖。分块所有者、渠道、有效期、每块摘要及合并回执在 PG 维护，重复合并返回同一结果；合并事务限制并发数量，为文件提交保留数据库连接。

## 配置

| 配置 | 用途 |
| --- | --- |
| `IMAGE_DATABASE_URL` | 必填，独立 PostgreSQL 数据库 |
| `IMAGE_HTTP_ADDR` | 默认 `:8002` |
| `IMAGE_PUBLIC_URL` | 公开根地址，本地默认 `http://localhost:8002` |
| `IMAGE_LOCAL_ROOT` | 必填，已有本地对象目录，不存在时拒绝启动 |
| `IMAGE_TEMP_ROOT` | 上传和分块临时文件，默认 `temp/image` |
| `IMAGE_LEGACY_HTTP_ADDR` | 可选，旧端口的文件兼容监听；本地当前为 `127.0.0.1:8082` |
| `IMAGE_LEGACY_SQLITE` | 迁移工具读取的原 SQLite |
| `IMAGE_MAX_UPLOAD_BYTES` | 默认 512 MiB |
| `IMAGE_MAX_CHUNK_BYTES` | 默认 20 MiB |
| `IMAGE_MAX_CHUNKS` | 默认 1000 |
| `IMAGE_UPLOAD_TTL` | 分块会话有效期，默认 24h |
| `IMAGE_HTTP_TIMEOUT` | 大文件读写期限，默认 10m |

OAuth 头像和创作者封面/插图的 Go 调用已改为 `/api/v2/categories/...`。`IMGBED_URL` 指向图床根地址，`IMGBED_API_TOKEN` 使用迁入 OAuth 的上传 Key。站点及原图床前端未在本轮修改。

## 迁移与升级

1. 在已有 PG 实例中新建空 image 库及专用账号，设置连接和原文件路径。备份并复用 `CLIENT_SECRET_ENCRYPTION_KEY`，不要生成新密钥覆盖。
2. 执行 `npm run db:deploy` 创建 OAuth 图床 Key 表，执行 `npm run db:image:deploy` 创建独立图床表。启动程序不会自动改表。
3. 停止原 Node 图床及其他 SQLite 写入方，完整备份 SQLite、WAL/SHM、文件目录，限制备份访问。
4. 执行 `npm run db:image:import -- -source <SQLite绝对路径>`，或 `atnexus-migrate-image -source <路径>`。
5. 工具使用只读 SQLite 事务逐行归档原五张表，以既有密钥加密。保留文件 ID、JSON 整数精度和 S3 键。旧 session、KV 索引与任务只归档，不生效；API Key 转为 OAuth 哈希。
6. 解密核对归档及写入的文件元数据，将本地字节 SHA-256 纳入源摘要。非空目标、变化的源快照、缺失文件、未知历史存储渠道均拒绝覆盖。
7. 重复执行验证同一摘要并返回 `alreadyImported=true`。两库非原子事务：OAuth 有独立导入标记，支持图床提交失败后重试，不重新启用后来被撤销的 Key。
8. 启动统一 Go 后端，检查旧图、新上传、各模块健康、session 撤销。旧域名继续反代 `/file/`；旧端口可设置兼容监听。

本地本轮导入：文件 15 条、设置归档 21 条、旧索引操作归档 10 条、Key 1 个；其他两张旧表为空。15 张旧图在新入口和原 8082 入口逐一核对 SHA-256，全部一致。原 `imgbed/data` 保留，停写备份位置记录于 `temp/image-cutover-backup.json`。真实数据和秘密不进入版本库。

## 兼容边界与失败恢复

- PG 是权威索引；旧 KV 索引重建/合并/任务机制不再运行，调用明确返回不适用。
- 原密码登录、图床 OAuth 回调和独立 session、UI 配置编辑、旧 Token 管理路径不恢复为新入口。原前端适配不属于本次任务。
- 旧上传/管理 API 调用方需切换 `/api/v2`；旧端口只提供文件读取。
- 文件记录恢复仅接受 `type=files`，校验已有对象后插入，禁止覆盖。S3 记录需包含已配置的 `ChannelName`。不会从备份恢复认证或存储密钥；完整原设置保留于加密归档。
- 对象存储与 PG 非同一事务。上传保留 `pending`，删除保留 `deleting`；远端超时、提交失败时需核对实际对象后恢复，不能直接批量删除状态记录。
- 移动先复制，确认 PG 提交后再删除原对象。复制成功而提交失败时可能有未索引目标对象；原子写入拒绝覆盖，需核对源/目标后处理。
- 到期分块不能合并，所有者或管理员仍可 cleanup。临时目录需定期检查遗留分块。
- 回退先停新 Go 写入。新系统有新增、移动或删除时必须先对账；仅在确认无新写入需要保留时才能恢复旧快照。新旧后端不能同时写同一目录。

验证脚本、fixture、日志位于 `temp/`。隔离 PG schema 验证覆盖迁移、Key/session、本地文件、分块及 S3 模拟协议；实际运行还验收真实旧图和 OAuth 退出失效。本轮没有对外部真实 S3 桶写入。
