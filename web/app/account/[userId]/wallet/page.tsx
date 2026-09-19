import { redirect } from "next/navigation"
export default function Page() { redirect((process.env.NEXT_PUBLIC_CREDIT_URL || "http://localhost:3300").replace(/\/$/, "") + "/account/wallet") }
