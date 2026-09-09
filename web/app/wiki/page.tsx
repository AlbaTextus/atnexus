import type { Metadata } from "next"
import { WikiDocument } from "@/components/wiki-document"

export const metadata: Metadata = { title: "ALBATEXTUS" }
const content = `## 欢迎 👋

这里是 **ALBATEXTUS Wiki**，记录账户使用方式、应用接入说明，以及与你相关的政策和协议。

## 社区概览

- [社区介绍](/wiki/community)：认识 ALBATEXTUS，了解社区定位、交流方式与参与入口。
- [等级介绍](/wiki/community/levels)：查看账户等级、管理权限和应用访问门槛。

## 开始使用

通过 [用户中心](/account) 查看身份信息、编辑个人简介、管理头像和登录方式。一个 ALBATEXTUS 账户，让不同应用之间的连接更简单。

### 个人资料

自定义头像支持上传、裁剪和 GIF 动画。个人简介支持 Markdown 排版，可在编辑页面预览后保存。

### 账户安全

在账户安全页查看邮箱、修改密码、管理第三方登录和通行密钥。

## 应用接入

通过 [接入指南](/wiki/integration) 了解 OAuth 2.0 授权码流程、S256 PKCE、服务端点与回调处理，让子系统使用统一账户登录。

## 政策与协议

- [隐私政策](/wiki/privacy)：了解我们如何处理和保护账户信息。
- [用户协议](/wiki/terms)：了解使用服务时的规则与约定。

## 关于此 Wiki

文档界面采用与 [LINUX DO WIKI](https://github.com/chenyme/LINUX-DO-WIKI) 相同的 Nextra 文档主题，感谢开源社区。`
export default function WikiPage() { return <WikiDocument title="欢迎来到 ALBATEXTUS" description="账户、连接与社区的使用指南。" content={content} /> }
