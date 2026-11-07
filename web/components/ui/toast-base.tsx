"use client"

import type { ReactNode } from "react"
import { Toast } from "@base-ui/react/toast"
import { CircleCheck, CircleX, X } from "lucide-react"

export const createToastManager = Toast.createToastManager
export type ToastManager = ReturnType<typeof createToastManager>

export function ToastProvider({ children, toastManager, limit = 3 }: {
  children: ReactNode; toastManager: ToastManager; limit?: number
}) {
  return <Toast.Provider toastManager={toastManager} limit={limit}>
    {children}
    <Toast.Portal><Toast.Viewport className="operation-toast-viewport" aria-label="操作通知" aria-live="off"><ToastList /></Toast.Viewport></Toast.Portal>
  </Toast.Provider>
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
