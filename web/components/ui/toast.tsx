"use client"

import { useEffect, type ReactNode } from "react"
import { Toast } from "@base-ui/react/toast"
import { CircleCheck, CircleX, X } from "lucide-react"
import { api } from "@/lib/api"
import { feedback, takeExternalLogin, takeRedirectFeedback, toastManager } from "@/lib/feedback"

export function ToastProvider({ children }: { children: ReactNode }) {
  return <Toast.Provider toastManager={toastManager} limit={3}>
    {children}
    <Toast.Portal><Toast.Viewport className="operation-toast-viewport" aria-label="操作通知" aria-live="off"><ToastList /></Toast.Viewport></Toast.Portal>
    <RedirectFeedback />
  </Toast.Provider>
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

function ToastList() {
  const { toasts } = Toast.useToastManager()
  return toasts.map(toast => {
    const Icon = toast.type === "error" ? CircleX : CircleCheck
    return <Toast.Root key={toast.id} toast={toast} className="operation-toast" data-kind={toast.type} swipeDirection={["up", "right"]}>
      <Toast.Content className="operation-toast-content">
        <Icon className="operation-toast-icon" aria-hidden="true" />
        <div className="operation-toast-text"><Toast.Title className="operation-toast-title" />{toast.description && <Toast.Description className="operation-toast-description" />}</div>
        <Toast.Close className="operation-toast-close" aria-label="关闭通知"><X aria-hidden="true" /></Toast.Close>
      </Toast.Content>
    </Toast.Root>
  })
}
