# atnexus 仓库约定

## Go 测试

- 纳入版本控制的 Go 测试（`*_test.go`）统一放在 `test/_go/`，按仓库根目录下原 Go 包路径组织。例如，`internal/security` 的测试放到 `test/_go/internal/security/`，保留原来的 `package` 声明。新增或合入测试时，不要在业务源码目录保留副本。
- 在仓库根目录运行 `pnpm test:go` 执行测试；也可指定原包路径和测试名，例如 `pnpm test:go ./internal/security -run TestRandomTokenAndHash`。测试入口会用 Go overlay 按原包编译测试；直接运行 `go test ./...` 不会发现集中存放的测试。
- 临时验证脚本仍放在 `temp/`。见 [Go 测试说明](test/README.md)。
