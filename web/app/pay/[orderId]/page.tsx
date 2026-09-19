import { redirect } from "next/navigation"
export default async function Page({ params }: { params: Promise<{ orderId: string }> }) { const { orderId } = await params; redirect((process.env.NEXT_PUBLIC_CREDIT_URL || "http://localhost:3300").replace(/\/$/, "") + "/pay/" + encodeURIComponent(orderId)) }
