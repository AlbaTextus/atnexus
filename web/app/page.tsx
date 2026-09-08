import type { Metadata } from "next"
import { HomePage } from "@/components/home-page"

export const metadata: Metadata = {
  title: "ALBATEXTUS · 一个账户，连接每个应用",
  description: "你的 ALBATEXTUS 统一账户入口。创建账户、连接应用，在同一处管理个人资料与账户安全。",
}

export default function Home() { return <HomePage /> }
