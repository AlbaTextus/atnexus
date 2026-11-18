# atnexus

atnexus 是 ALBATEXTUS 的统一服务：一个 Go 程序运行 OAuth、图床、创作者平台和 ATHub 四个模块，前端位于 `web/`。

Go 测试集中在 [`test/`](test/)，运行 `pnpm test:go` 执行测试套件。

其余说明统一写在文档站 <https://docs.albatextus.com>（源码在 [AlbaTextus/docs](https://github.com/AlbaTextus/docs) 的 `content/`）。文档仓库以子模块挂在 `docs/`，运行 `pnpm docs:sync` 检出或更新其中的 Markdown。

| 内容 | 文档 |
| --- | --- |
| 服务总览与部署顺序 | [部署 · 服务总览](https://docs.albatextus.com/deploy/overview) |
| 本地启动 | [部署 · 本地快速开始](https://docs.albatextus.com/deploy/local) |
| 环境变量 | [部署 · 环境变量](https://docs.albatextus.com/deploy/environment) |
| 数据库与迁移 | [部署 · 数据库与迁移](https://docs.albatextus.com/deploy/database) |
| Docker Compose | [部署 · Docker Compose](https://docs.albatextus.com/deploy/docker) |
| 构建与发布包 | [部署 · 构建与发布](https://docs.albatextus.com/deploy/build) |
| 生产部署与升级 | [部署 · 生产部署与升级](https://docs.albatextus.com/deploy/production) |
| 目录结构与开发约定 | [部署 · 开发与目录约定](https://docs.albatextus.com/deploy/development) |
| OAuth 与账户 | [OAuth](https://docs.albatextus.com/oauth/overview) |
| 图床 | [图床](https://docs.albatextus.com/image/overview) |
| 创作者平台 | [创作者平台](https://docs.albatextus.com/creatplat/overview) |
| ATHub 内容接口 | [ATHub](https://docs.albatextus.com/athub/overview) |
