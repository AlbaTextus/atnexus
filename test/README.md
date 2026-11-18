# Go 测试

`_go/` 按仓库根目录下的 Go 包路径组织测试。例如，`internal/security` 的测试放在 `_go/internal/security/`，保留原来的 `package` 声明。

```sh
pnpm test:go                                     # 运行所有 Go 测试
pnpm test:go ./internal/security -run TestRandomTokenAndHash
pnpm test:go ./... -count=1                       # 禁用测试结果缓存
```

新增 `*_test.go` 时也按此目录结构放置，无需登记文件。`pnpm test:go` 会生成临时 Go overlay，让测试仍在原包内编译和运行，保留对包内未导出标识符的访问和原有的工作目录。直接运行 `go test ./...` 不会发现 `_go/` 下的测试。

数据库集成测试沿用现有的跳过条件；按需设置 `OAUTH_TEST_DATABASE_URL`、`ATHUB_TEST_DATABASE_URL` 或 `CREATPLAT_TEST_DATABASE_URL`。
