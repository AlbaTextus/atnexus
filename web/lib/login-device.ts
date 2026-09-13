// A versioned browser-environment fingerprint; it is not proof of device identity.
export async function collectLoginDevice() {
  if (typeof window === "undefined") return
  try {
    const suffix = `; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`
    document.cookie = `albatextus_screen=${screen.width}x${screen.height}${suffix}`
    document.cookie = `albatextus_timezone=${encodeURIComponent(Intl.DateTimeFormat().resolvedOptions().timeZone)}${suffix}`
    const attributes = JSON.stringify([
      "v1", navigator.userAgent, navigator.language, navigator.languages,
      navigator.platform, navigator.hardwareConcurrency, navigator.maxTouchPoints,
      screen.width, screen.height, screen.colorDepth, window.devicePixelRatio,
      Intl.DateTimeFormat().resolvedOptions().timeZone,
    ])
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(attributes))
    const fingerprint = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, "0")).join("")
    document.cookie = `albatextus_device=${fingerprint}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`
  } catch {
    // Restricted browsers must still be able to sign in, without a stale fingerprint.
    document.cookie = "albatextus_device=; Path=/; Max-Age=0; SameSite=Lax"
  }
}
