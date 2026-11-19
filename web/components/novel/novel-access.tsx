"use client"

import { Fragment, createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { UserRound } from "lucide-react"
import { Button } from "@/components/novel/ui/button"
import { api, ApiError } from "@/lib/api"
import { ContentApiError, fetchContent } from "@/lib/athub/athub-client"
import { ContentCache, clearStoredContent } from "@/lib/content-cache"

type Session = { expires_at: string; cache_scope: string }
type AccessContext = {
  ready: boolean
  authenticated: boolean
  error: string
  scope: string
  refresh: () => Promise<void>
  request: <T>(path: string, signal?: AbortSignal) => Promise<T>
}
const Context = createContext<AccessContext | null>(null)

export function NovelAccessProvider({ children }: { children: ReactNode }) {
  const session = useRef<Session | null>(null)
  const pending = useRef<Promise<Session> | null>(null)
  const contentCache = useRef<{ scope: string; cache: ContentCache } | null>(null)
  const lastCheck = useRef(0)
  const [ready, setReady] = useState(false)
  const [authenticated, setAuthenticated] = useState(false)
  const [error, setError] = useState("")
  const [scope, setScope] = useState("")
  const [cacheRevision, setCacheRevision] = useState(0)

  const obtain = useCallback(async () => {
    if (!pending.current) {
      pending.current = api<Session>("/api/v1/session")
        .then(value => {
          // During a rolling deployment an older backend may omit the scope.
          // Use an ephemeral scope rather than sharing a persisted cache.
          if (!value.cache_scope) value = { ...value, cache_scope: crypto.randomUUID() }
          if (contentCache.current?.scope !== value.cache_scope) {
            contentCache.current?.cache.clear()
            contentCache.current = { scope: value.cache_scope, cache: new ContentCache(value.cache_scope) }
          }
          session.current = value
          setScope(value.cache_scope)
          lastCheck.current = Date.now()
          setAuthenticated(true); setError("")
          return value
        })
        .catch(reason => {
          if (reason instanceof ApiError && (reason.status === 401 || reason.status === 403)) {
            session.current = null
            contentCache.current?.cache.clear()
            contentCache.current = null
            clearStoredContent()
            setAuthenticated(false)
            setError(reason.status === 401 ? "" : "当前账户无权访问小说库。")
          } else if (!session.current || Date.parse(session.current.expires_at) <= Date.now()) {
            setError("暂时无法读取登录状态，请稍后重试。")
          }
          throw reason
        }).finally(() => { pending.current = null; setReady(true) })
    }
    return pending.current
  }, [])
  const refresh = useCallback(async () => {
    // A background session check must not unmount the page or discard content.
    try { await obtain(); setCacheRevision(value => value + 1) } catch { /* The gate shows the account state. */ }
  }, [obtain])

  useEffect(() => {
    // Remove credentials saved by the previous manual-key implementation.
    try { sessionStorage.removeItem("atnexus:content-api-key") } catch { /* Storage may be disabled. */ }
    void refresh()
    const focus = () => { if (Date.now() - lastCheck.current >= 30_000) void refresh() }
    const changed = () => {
      contentCache.current?.cache.clear(false)
      setCacheRevision(value => value + 1)
    }
    window.addEventListener("focus", focus)
    window.addEventListener("atnexus:content-changed", changed)
    return () => {
      window.removeEventListener("focus", focus)
      window.removeEventListener("atnexus:content-changed", changed)
    }
  }, [refresh])

  const request = useCallback(async <T,>(path: string, signal?: AbortSignal): Promise<T> => {
    let current = session.current
    if (!current || Date.parse(current.expires_at) <= Date.now() + 15000) current = await obtain()
    const credential = current
    const load = async (sharedSignal: AbortSignal) => {
      try { return await fetchContent<T>(path, sharedSignal) }
      catch (reason) {
        if (!(reason instanceof ContentApiError) || reason.status !== 401 || sharedSignal.aborted) throw reason
        contentCache.current?.cache.clear(false)
        const renewed = await obtain()
        if (renewed.cache_scope !== credential.cache_scope) throw new DOMException("Account session changed", "AbortError")
        return fetchContent<T>(path, sharedSignal)
      }
    }
    return contentCache.current!.cache.read(path, load, signal)
  }, [obtain, cacheRevision])

  return <Context.Provider value={{ ready, authenticated, error, scope, refresh, request }}>{children}</Context.Provider>
}

export function useNovelAccess() {
  const context = useContext(Context)
  if (!context) throw new Error("Novel access provider is required")
  return context
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
  const { authenticated, ready, scope } = useNovelAccess()
  if (!ready) return <div className="novel-loading" role="status">正在加载…</div>
  return authenticated ? <Fragment key={scope}>{children}</Fragment> : <NovelConnection />
}

export function ContentFailure({ message, retry }: { message: string; retry: () => void }) {
  return <div className="panel novel-failure" role="alert"><p>{message}</p><Button variant="outline" onClick={retry}>重试</Button></div>
}
