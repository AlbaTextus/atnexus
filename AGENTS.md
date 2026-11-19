# atnexus 仓库约定

## 说明文档

- 文档站 <https://docs.albatextus.com> 的正文写在本仓库的 `docs/`，按模块分目录（`athub`、`oauth`、`image`、`creatplat`、`deploy`），每个页面一个 Markdown 文件。站点代码在另一个仓库 [AlbaTextus/docs](https://github.com/AlbaTextus/docs)。
- 改动行为、环境变量、数据库迁移或部署方式时，在同一个分支里更新 `docs/` 的对应说明，和代码一起提交；不要写回本仓库的 README。
- 文档之间的链接写站内绝对路径，例如 `[数据库](/deploy/database)`，不要写成仓库内的相对路径。
- 发布是自动的：`main` 推送到 GitHub 且改动包含 `docs/` 时，[同步工作流](.github/workflows/sync-docs.yml) 把 `docs/` 镜像到文档站仓库的 `content/` 并推送，文档站随即更新。功能分支上的文档在合并前不会发布。
- 不要直接改文档站仓库的 `content/`，下一次同步会用本仓库的 `docs/` 覆盖它。
- 新增页面除了在 `docs/<模块>/` 写 Markdown，还要在文档站仓库的 `lib/navigation.ts` 加一项，否则页面不会出现在站点上。只改已有页面不需要动文档站仓库。
- 同步工作流依赖仓库密钥 `DOCS_DEPLOY_KEY`（文档站仓库带写权限的部署密钥的私钥）。

## Go 测试

- 纳入版本控制的 Go 测试（`*_test.go`）统一放在 `test/_go/`，按仓库根目录下原 Go 包路径组织。例如，`internal/security` 的测试放到 `test/_go/internal/security/`，保留原来的 `package` 声明。新增或合入测试时，不要在业务源码目录保留副本。
- 在仓库根目录运行 `pnpm test:go` 执行测试；也可指定原包路径和测试名，例如 `pnpm test:go ./internal/security -run TestRandomTokenAndHash`。测试入口会用 Go overlay 按原包编译测试；直接运行 `go test ./...` 不会发现集中存放的测试。
- 临时验证脚本仍放在 `temp/`。见 [Go 测试说明](test/README.md)。
