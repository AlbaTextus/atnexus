"use client"

import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { UserRound } from "lucide-react"
import { Button } from "@/components/novel-demo/ui/button"
import { api, ApiError } from "@/lib/api"
import { ContentApiError, fetchContent } from "@/lib/athub-client"

type Session = { access_token: string; expires_at: string }
type AccessContext = {
  ready: boolean
  authenticated: boolean
  error: string
  refresh: () => Promise<void>
  request: <T>(path: string, signal?: AbortSignal) => Promise<T>
}
const Context = createContext<AccessContext | null>(null)

export function NovelAccessProvider({ children }: { children: ReactNode }) {
  const session = useRef<Session | null>(null)
  const pending = useRef<Promise<Session> | null>(null)
  const [ready, setReady] = useState(false)
  const [authenticated, setAuthenticated] = useState(false)
  const [error, setError] = useState("")

  const obtain = useCallback(async () => {
    if (!pending.current) {
      pending.current = api<Session>("/api/v1/account/athub/session", { method: "POST", body: "{}" })
        .then(value => { session.current = value; setAuthenticated(true); setError(""); return value })
        .catch(reason => {
          session.current = null; setAuthenticated(false)
          setError(reason instanceof ApiError && reason.status === 401 ? "" : "暂时无法读取登录状态，请稍后重试。")
          throw reason
        }).finally(() => { pending.current = null; setReady(true) })
    }
    return pending.current
  }, [])
  const refresh = useCallback(async () => {
    session.current = null
    setReady(false); setAuthenticated(false)
    try { await obtain() } catch { /* The gate shows the account state. */ }
  }, [obtain])

  useEffect(() => {
    // Remove credentials saved by the previous manual-key implementation.
    try { sessionStorage.removeItem("atnexus:content-api-key") } catch { /* Storage may be disabled. */ }
    void refresh()
    const focus = () => { void refresh() }
    window.addEventListener("focus", focus)
    return () => window.removeEventListener("focus", focus)
  }, [refresh])

  const request = useCallback(async <T,>(path: string, signal?: AbortSignal): Promise<T> => {
    let current = session.current
    if (!current || Date.parse(current.expires_at) <= Date.now() + 15000) current = await obtain()
    try { return await fetchContent<T>(path, current.access_token, signal) }
    catch (reason) {
      if (!(reason instanceof ContentApiError) || reason.status !== 401 || signal?.aborted) throw reason
      current = await obtain()
      return fetchContent<T>(path, current.access_token, signal)
    }
  }, [obtain])

  return <Context.Provider value={{ ready, authenticated, error, refresh, request }}>{children}</Context.Provider>
}

export function useNovelAccess() {
  const context = useContext(Context)
  if (!context) throw new Error("Novel access provider is required")
  return context
}

export function useContentResource<T>(path: string | null) {
  const { authenticated, request } = useNovelAccess()
  const [result, setResult] = useState<{ path: string | null; data?: T; error?: string }>({ path: null })
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    if (!authenticated || !path) { setResult({ path: null }); return }
    const controller = new AbortController()
    setResult({ path })
    request<T>(path, controller.signal)
      .then(data => { if (!controller.signal.aborted) setResult({ path, data }) })
      .catch(reason => { if (!controller.signal.aborted) setResult({ path, error: reason instanceof Error ? reason.message : "加载失败。" }) })
    return () => controller.abort()
  }, [path, authenticated, request, revision])
  return {
    data: authenticated && result.path === path ? result.data : undefined,
    error: result.path === path ? result.error : undefined,
    retry: () => setRevision(value => value + 1),
  }
}

export function NovelConnection() {
  const { ready, authenticated, error, refresh } = useNovelAccess()
  const pathname = usePathname()
  return <section className="panel novel-connection" aria-labelledby="content-connection">
    <h2 id="content-connection"><UserRound aria-hidden="true" />账户</h2>
    {!ready ? <p role="status">正在读取登录状态…</p>
      : error ? <ContentFailure message={error} retry={() => { void refresh() }} />
      : authenticated ? <div className="novel-actions"><p>已使用用户中心登录</p><Link href="/account">进入用户中心</Link></div>
      : <div className="novel-actions"><p>登录后即可使用小说库。</p><Link href={`/login?return_to=${encodeURIComponent(pathname)}`}>登录</Link></div>}
  </section>
}

export function NovelAccessGate({ children }: { children: ReactNode }) {
  const { authenticated, ready } = useNovelAccess()
  if (!ready) return <div className="novel-loading" role="status">正在加载…</div>
  return authenticated ? children : <NovelConnection />
}

export function ContentFailure({ message, retry }: { message: string; retry: () => void }) {
  return <div className="panel novel-failure" role="alert"><p>{message}</p><Button variant="outline" onClick={retry}>重试</Button></div>
}
