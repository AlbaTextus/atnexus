export type TurnstileOptions = {
  sitekey: string
  action: string
  theme: "light" | "dark"
  language: "zh-cn"
  size: "flexible" | "compact"
  "response-field": false
  "refresh-expired": "auto"
  callback: (token: string) => void
  "expired-callback": () => void
  "timeout-callback": () => void
  "error-callback": () => boolean
}

type TurnstileClient = {
  render: (container: HTMLElement, options: TurnstileOptions) => string
  remove: (id: string) => void
}

declare global {
  interface Window {
    turnstile?: TurnstileClient
    albatextusTurnstileLoaded?: () => void
  }
}

let loader: Promise<TurnstileClient> | undefined

export function loadTurnstile(): Promise<TurnstileClient> {
  if (loader) return loader
  if (window.turnstile) return Promise.resolve(window.turnstile)
  loader = new Promise<TurnstileClient>((resolve, reject) => {
    const script = document.createElement("script")
    script.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit&onload=albatextusTurnstileLoaded"
    script.async = true
    script.defer = true
    const fail = () => {
      clearTimeout(timeout)
      script.remove()
      loader = undefined
      reject(new Error("安全验证加载失败，请检查网络后重试。"))
    }
    const timeout = setTimeout(fail, 15000)
    script.onerror = fail
    window.albatextusTurnstileLoaded = () => {
      if (!window.turnstile) { fail(); return }
      clearTimeout(timeout)
      resolve(window.turnstile)
    }
    document.head.appendChild(script)
  })
  return loader
}
