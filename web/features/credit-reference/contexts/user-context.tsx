"use client"
import { createContext, useContext, useEffect, useState, type ReactNode } from "react"
import { previewRequest } from "../lib/services"
import { Button } from "../components/ui/button"

export type PreviewUser = { id: string; username: string; nickname: string; avatar_url?: string; trust_level: number; is_admin: boolean; available_balance: string; pending_balance: string }
export type PreviewData = { user: PreviewUser; trust_levels: Record<string, string>; public_config: { settlement_delay_days_min: number; settlement_delay_days_max: number }; notification: { show_bell: boolean }; announcement: { title: string; link_label: string; link_url: string } }
const Context = createContext<{ data: PreviewData | null; loading: boolean } | null>(null)
export function UserProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<PreviewData | null>(null)
  const [error, setError] = useState(false)
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let active = true
    previewRequest<PreviewData>("user").then(value => { if (active) setData(value) }).catch(() => { if (active) setError(true) })
    return () => { active = false }
  }, [revision])
  if (error) return <div className="p-8" role="alert">钱包数据加载失败。<Button variant="outline" onClick={() => { setError(false); setRevision(v => v + 1) }}>重试</Button></div>
  return <Context.Provider value={{ data, loading: !data }}>{children}</Context.Provider>
}
export function usePreviewData() {
  const context = useContext(Context)
  if (!context) throw new Error("Reference preview provider is required")
  return context
}
export function useUser() {
  const { data, loading } = usePreviewData()
  return {
    user: data?.user ?? null, loading,
    getTrustLevelLabel: (level: number) => data?.trust_levels[String(level)] ?? "",
    logout: async () => { window.location.assign("/account") },
  }
}
