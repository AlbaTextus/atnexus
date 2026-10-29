"use client"

import { useEffect, type ReactNode } from "react"
import { ToastProvider as SharedToastProvider } from "@albatextus/ui/components/toast"
import { api } from "@/lib/api"
import { feedback, takeExternalLogin, takeRedirectFeedback, toastManager } from "@/lib/feedback"

export function ToastProvider({ children }: { children: ReactNode }) {
  return <SharedToastProvider toastManager={toastManager}>{children}<RedirectFeedback /></SharedToastProvider>
}

function RedirectFeedback() {
  useEffect(() => {
    let active = true
    const timer = window.setTimeout(() => {
      const message = takeRedirectFeedback()
      if (message) feedback.success(message)
      if (takeExternalLogin()) {
        // An OAuth round trip alone is not proof of a successful login.
        void api("/api/v1/user").then(() => { if (active) feedback.success("登录成功") }).catch(() => {})
      }
    }, 0)
    return () => { active = false; window.clearTimeout(timer) }
  }, [])
  return null
}
