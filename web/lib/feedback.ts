"use client"

import { useCallback, useEffect, useState } from "react"
import { Toast } from "@base-ui/react/toast"

export const toastManager = Toast.createToastManager()
type FeedbackKind = "success" | "error"
const flashKey = "albatextus.operation-feedback"
const externalLoginKey = "albatextus.pending-login"

function show(kind: FeedbackKind, message: string) {
  if (!message.trim()) return
  toastManager.add({
    id: `${kind}:${message}`,
    type: kind,
    title: kind === "error" ? "操作失败" : message,
    description: kind === "error" ? message : undefined,
    priority: kind === "error" ? "high" : "low",
    timeout: kind === "error" ? 3000 : 2000,
  })
}

export const feedback = {
  success: (message: string) => show("success", message),
  error: (message: string) => show("error", message),
}

// Keep field-level messages while also announcing each completed operation.
// Calling the setter again repeats a failed attempt, even if its message is unchanged.
export function useFeedback(kind: FeedbackKind, initial = "") {
  const [message, setMessage] = useState(initial)
  useEffect(() => {
    if (!initial) return
    const timer = window.setTimeout(() => show(kind, initial), 0)
    return () => window.clearTimeout(timer)
  }, [kind, initial])
  const update = useCallback((value: string) => {
    setMessage(value)
    show(kind, value)
  }, [kind])
  return [message, update] as const
}

// Full-page authentication redirects remount the app. Consume the message once
// on the destination instead of losing it when the login form disappears.
export function redirectWithFeedback(destination: string, message: string) {
  try {
    sessionStorage.setItem(flashKey, JSON.stringify({ message, at: Date.now() }))
    window.location.assign(destination)
  } catch {
    feedback.success(message)
    window.setTimeout(() => window.location.assign(destination), 900)
  }
}

export function takeRedirectFeedback(): string | null {
  try {
    const raw = sessionStorage.getItem(flashKey)
    sessionStorage.removeItem(flashKey)
    if (!raw) return null
    const value = JSON.parse(raw)
    if (typeof value.message === "string" && value.message.length <= 500 && typeof value.at === "number" && Date.now() - value.at >= 0 && Date.now() - value.at < 60000) return value.message
  } catch { /* Storage must not prevent the destination from loading. */ }
  return null
}

export function markExternalLogin() {
  try { sessionStorage.setItem(externalLoginKey, String(Date.now())) } catch { /* Optional feedback. */ }
}

export function takeExternalLogin(): boolean {
  try {
    const raw = sessionStorage.getItem(externalLoginKey)
    sessionStorage.removeItem(externalLoginKey)
    if (!raw || new URLSearchParams(window.location.search).has("oauth_error")) return false
    const age = Date.now() - Number(raw)
    return age >= 0 && age < 10 * 60 * 1000
  } catch { return false }
}
