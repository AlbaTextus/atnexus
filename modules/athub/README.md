# ATHub 内容模块（v2）

本目录是 atnexus 统一服务内独立维护的内容模块。宿主通过 `module.go` 初始化数据库、取得
HTTP Handler 并负责关闭连接池；模块不启动 HTTP 服务。只依赖本目录源码，不引用相邻
ATHub 工作目录。单独修改、测试模块后，由宿主重新编译发布。

来源为 `D:\AlbaTextus\athub` 的当前工作树，包括当时未提交的内容 API 精简和迁移。
来源仓库基准提交为 `2712860b88270266ad4e241fc2a94bbee8ddc7e7`。
这是源码合入，未建立 submodule 或 subtree 同步关系；后续源码以本目录为维护入口。
原工作目录保留。

## HTTP 接口

宿主在同一进程中启动两个监听器：OAuth 使用 `8001`，本模块使用 `8003`
（可由 `ATHUB_HTTP_ADDR` 修改）。内容接口直接使用 `/api/v2/`：

| 路径 | 方法 |
| --- | --- |
| `/api/v2/platforms` | GET、POST |
| `/api/v2/authors` | GET、POST、PUT |
| `/api/v2/tags` | GET、POST、PUT、DELETE |
| `/api/v2/novelinfo` | GET、POST、PUT、DELETE |
| `/api/v2/volumes` | GET、PUT、DELETE |
| `/api/v2/toc` | GET |
| `/api/v2/content` | GET、PUT、DELETE |
| `/api/v2/upload` | POST |

请求需 `Authorization: Bearer <API_KEY>`。登录用户在宿主 `8001/api/v1/account/key`
自动生成唯一的永久 Key，支持重置和停用；宿主注入的鉴权与记账中间件校验归属、当前 Key、停用状态和用户启用状态。
Key 只在生成或重置时显示明文，账户库只保存 SHA-256 哈希。停用后不会自动恢复，重置保留历史调用记录。
原全局密钥、OAuth access token、会话 Cookie 和旧个人密钥不能替代新 Key。
内置小说库另由宿主账户会话签发短时只读凭证（`ats_`），仅允许 GET，
并在宿主账户库逐次核验会话和管理员限制；无需输入或生成个人 Key。
本模块仍只接收宿主鉴权后的请求，不处理用户或会话。
没有 `/athub` 中间路径。旧 `/api/content` 等路径不提供兼容。

作者支持可空的 `name`（最多 200 个字符），GET 单项和列表都会返回此字段。
`POST /api/v2/authors` 可提交 `{"platformId":1,"name":"作者笔名"}`；
省略姓名仍兼容原请求。`PUT /api/v2/authors?id=3` 提交 `{"name":"作者笔名"}`，
只修改姓名，不改作者 ID、所属平台或小说关联。名称首尾空白会清理；
`null`、空字符串或省略 `name` 清空姓名，包含 NUL 或超过 200 字符返回 400。
修改接口要求外部个人 API Key，网站自动登录的只读凭证不允许 PUT。
现有请求字段、分页、8 MiB JSON 上限、批量上传事务、分卷和章节排序规则保持不变。
创建资源返回的 `Location` 使用 `/api/v2/`。

## 数据库与迁移

`ATHUB_DATABASE_URL` 专用于内容库，与宿主 `DATABASE_URL` 分开配置。
本目录 `prisma/` 保留完整历史迁移，宿主账户库的迁移仍由根目录 `prisma/` 管理。
可连接原 ATHub 数据库，无需搬运内容数据；启动只检查连接与 schema，不自动迁移。
内容库保存平台、作者、小说、标签库、小说标签关系、卷和章节。`athub_api_keys`、`athub_api_calls` 两张表均由
宿主账户库维护；本模块只提供注入接口，不连接账户库、不定义用户或调用统计表。

在统一仓库根目录执行：

```powershell
npm run db:athub:install
# Prisma 7 在此目录执行，从统一仓库根目录读取 .env。
npm run db:athub:validate
npm run db:athub:deploy
```

迁移前备份数据库。其中既有 `20261103000000_remove_user_api` 迁移会删除
`ApiUser`、`ApiKey`、`ApiUsage`，保留平台、作者、小说、卷与章节。

`20261110030000_add_tags` 增量创建 `Tag` 与 `NovelTag`，已有小说默认无标签，原内容和时间不变。
标签名称唯一，小说元信息用 `tagIds` 引用独立标签库，不重复存储标签名称。
标签可以跨平台复用；小说列表支持 `tagId` 筛选。创建小说和批量上传的 `novel` 都可传入 `tagIds`。
覆盖小说元信息时省略 `tagIds`、传 `null` 或 `[]` 清空引用；任一标签不存在时整个事务回滚。
续传保留标签，标签改名保留 ID；删除仍被引用的标签返回 `409`。
删除小说只清理标签关系，保留标签库。新版本启动前先在统一仓库根目录执行 `npm run db:athub:deploy`。

## 验证

作者姓名功能需先应用 `20261110040000_add_author_name` 内容库迁移，仅新增
`Author.name VARCHAR(200)` 可空字段。已有作者和小说关联保留，历史姓名默认为 NULL，
不会推测或自动填入示例姓名。前端有姓名时显示笔名，无姓名时继续显示作者编号。

```powershell
# 在统一仓库根目录执行；根目录 go test ./... 不会遍历独立 Go 模块。
npm run test:go
go -C modules/athub vet ./...
```

设置 `ATHUB_TEST_DATABASE_URL` 后可运行真实 PostgreSQL 集成测试；测试创建并清理临时
schema，不在业务表中写入测试数据。未配置时这些测试跳过。
