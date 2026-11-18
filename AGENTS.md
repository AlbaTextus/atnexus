# atnexus 仓库约定

## 说明文档

- 说明文档在文档站仓库 [AlbaTextus/docs](https://github.com/AlbaTextus/docs)，以 Git 子模块挂在 `docs/`，只检出 `docs/content/` 下的 Markdown（按模块分目录：`athub`、`oauth`、`image`、`creatplat`、`deploy`）。本仓库只记录子模块的提交号，不保存文档副本。
- 改动行为、环境变量、数据库迁移或部署方式时，同一次工作里更新 `docs/content/` 的对应说明，不要写回本仓库的 README。
- 新的检出或 worktree 里 `docs/` 是空的，运行一次 `pnpm docs:sync` 即可初始化。不带参数时它只拉取文档仓库的新提交，并在本仓库提交子模块指针。
- 子模块里的提交必须位于 `main` 分支上，不要在游离 HEAD 上提交。

### 文档改动的流程

文档跟着功能分支走：在分支里改，分支合并到 `main` 之后才推送到文档仓库。

1. 在功能分支（worktree）里改代码时，同一个分支里改 `docs/content/` 的对应说明。
2. 在仓库根目录运行 `pnpm docs:sync -m "文档提交说明"`。它在子模块里提交文档，合入文档仓库远端 `main` 的新提交，再在当前分支提交子模块指针。这一步只产生本地提交。
3. 分支合并前不要推送文档仓库。推送文档仓库的 `main` 会发布文档站，文档会先于功能上线。
4. 分支合并到本仓库的 `main` 之后，在改文档的那个 worktree 里运行 `git -C docs push origin main`，把文档推送到文档仓库。
5. 文档仓库推送成功后再推送本仓库的 `main`。顺序不能反，否则别人拿到的指针指向远端不存在的提交。
6. 在主检出里运行 `pnpm docs:sync`，让那里的 `docs/` 跟上。

注意：

- 文档提交在推送前只存在于改文档的那个 worktree 的子模块里，其他检出拿不到。推送完成前不要删除该 worktree。
- 推送被拒绝说明远端有别的分支先推送了文档。重新运行 `pnpm docs:sync` 合入后再推送；不要对已经记录进本仓库的文档提交做 rebase 或 amend，否则指针会指向不存在的提交。

访问 GitHub 失败时，先按本机环境设置 `HTTPS_PROXY` 再运行；不要把代理地址写进仓库。

## Go 测试

- 纳入版本控制的 Go 测试（`*_test.go`）统一放在 `test/_go/`，按仓库根目录下原 Go 包路径组织。例如，`internal/security` 的测试放到 `test/_go/internal/security/`，保留原来的 `package` 声明。新增或合入测试时，不要在业务源码目录保留副本。
- 在仓库根目录运行 `pnpm test:go` 执行测试；也可指定原包路径和测试名，例如 `pnpm test:go ./internal/security -run TestRandomTokenAndHash`。测试入口会用 Go overlay 按原包编译测试；直接运行 `go test ./...` 不会发现集中存放的测试。
- 临时验证脚本仍放在 `temp/`。见 [Go 测试说明](test/README.md)。
