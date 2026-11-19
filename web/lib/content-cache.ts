type Entry = { value: unknown; expires: number }
type Pending = { promise: Promise<unknown>; controller: AbortController; users: number }
const prefix = "atnexus:content-cache:v1:"
const maxEntries = 48
const maxBytes = 1024 * 1024

// Session-scoped content only. Credentials are never persisted. A fresh account
// session must be obtained before selecting a cache, including after F5.
export class ContentCache {
  private entries = new Map<string, Entry>()
  private pending = new Map<string, Pending>()
  private generation = 0

  constructor(private scope: string) {
    try {
      const stored: [string, Entry][] = JSON.parse(sessionStorage.getItem(prefix + scope) || "[]")
      if (Array.isArray(stored)) {
        for (const [path, entry] of stored.slice(-maxEntries)) {
          if (typeof path === "string" && entry && typeof entry.expires === "number" && entry.expires > Date.now()) {
            this.entries.set(path, entry)
          }
        }
      }
    } catch { /* Storage is optional. */ }
  }

  clear(abortRequests = true) {
    this.generation++
    this.entries.clear()
    if (abortRequests) {
      for (const pending of this.pending.values()) pending.controller.abort()
    }
    // A post-mutation reader must not join a fetch started before invalidation.
    this.pending.clear()
    try { sessionStorage.removeItem(prefix + this.scope) } catch { /* Storage is optional. */ }
  }

  async read<T>(path: string, load: (signal: AbortSignal) => Promise<T>, signal?: AbortSignal): Promise<T> {
    signal?.throwIfAborted()
    const hit = this.entries.get(path)
    if (hit && hit.expires > Date.now()) return hit.value as T
    this.entries.delete(path)
    let pending = this.pending.get(path)
    if (!pending || pending.controller.signal.aborted) {
      const controller = new AbortController()
      const generation = this.generation
      const next: Pending = { controller, users: 0, promise: Promise.resolve() }
      next.promise = load(controller.signal).then(value => {
        if (!controller.signal.aborted && this.generation === generation) {
          const ttl = /^\/api\/v5\/(tags|platforms)(\?|$)/.test(path) ? 5 * 60_000 : 30_000
          this.entries.set(path, { value, expires: Date.now() + ttl })
          while (this.entries.size > maxEntries) this.entries.delete(this.entries.keys().next().value!)
          this.persist()
        }
        return value
      }).finally(() => {
        if (this.pending.get(path) === next) this.pending.delete(path)
      })
      this.pending.set(path, next)
      pending = next
    }
    const shared = pending
    shared.users++
    return new Promise<T>((resolve, reject) => {
      const abort = () => reject(signal?.reason || new DOMException("Aborted", "AbortError"))
      signal?.addEventListener("abort", abort, { once: true })
      shared.promise.then(value => resolve(value as T), reject).finally(() => {
        signal?.removeEventListener("abort", abort)
      })
      if (signal?.aborted) abort()
    }).finally(() => {
      shared.users--
      if (!shared.users && this.pending.get(path) === shared) shared.controller.abort()
    })
  }

  private persist() {
    try {
      let entries = [...this.entries].filter(([, entry]) => entry.expires > Date.now())
      let json = JSON.stringify(entries)
      while (json.length * 2 > maxBytes && entries.length) {
        this.entries.delete(entries[0][0])
        entries = entries.slice(1)
        json = JSON.stringify(entries)
      }
      // History need not survive reloads.
      entries = entries.filter(([path]) => !/^\/api\/v4\/history(\?|$)/.test(path))
      sessionStorage.setItem(prefix + this.scope, JSON.stringify(entries))
    } catch { /* Storage is optional. */ }
  }
}

export function clearStoredContent() {
  try {
    for (const key of Object.keys(sessionStorage)) if (key.startsWith(prefix)) sessionStorage.removeItem(key)
  } catch { /* Storage is optional. */ }
}
