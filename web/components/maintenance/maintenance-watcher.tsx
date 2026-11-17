"use client"

import { useEffect } from "react"
import { usePathname } from "next/navigation"
import { maintenanceDestination, maintenancePath } from "@/lib/maintenance/maintenance-navigation"

export function MaintenanceWatcher() {
  const pathname = usePathname()
  useEffect(() => {
    let cancelled = false
    let running = false
    const controller = new AbortController()
    async function check() {
      if (running || document.visibilityState === "hidden") return
      running = true
      try {
        const response = await fetch("/api/v1/maintenance", { cache: "no-store", signal: AbortSignal.any([controller.signal, AbortSignal.timeout(5000)]) })
        const state = response.ok ? await response.json() : null
        if (cancelled) return
        // A failed status probe is not evidence that maintenance was enabled.
        if (!state || typeof state.enabled !== "boolean" || typeof state.bypass !== "boolean") return
        const onMaintenance = pathname === "/maintenance"
        const onAdminLogin = pathname === "/maintenance/login"
        if (state.enabled && !state.bypass && !onMaintenance && !onAdminLogin) {
          window.location.replace(maintenancePath(window.location.pathname + window.location.search + window.location.hash))
        } else if ((!state.enabled || state.bypass) && onMaintenance) {
          window.location.replace(maintenanceDestination(new URLSearchParams(window.location.search).get("return_to"), state.bypass))
        }
      } catch { /* A transient network outage is retried on the next check. */ }
      finally { running = false }
    }
    void check()
    const interval = window.setInterval(check, 15000)
    window.addEventListener("focus", check)
    window.addEventListener("pageshow", check)
    document.addEventListener("visibilitychange", check)
    return () => {
      cancelled = true
      controller.abort()
      window.clearInterval(interval)
      window.removeEventListener("focus", check)
      window.removeEventListener("pageshow", check)
      document.removeEventListener("visibilitychange", check)
    }
  }, [pathname])
  return null
}
