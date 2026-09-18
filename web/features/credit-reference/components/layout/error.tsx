// Copied from linux-do/credit, Apache-2.0. Only imports adapted for this preview.
import * as React from "react"
import { AlertCircle } from "lucide-react"
import { Button } from "@/features/credit-reference/components/ui/button"

import { cn } from "@/features/credit-reference/lib/utils"


/**
 * 错误展示组件属性
 * 用于统一显示加载失败、请求失败等错误状态
 */
export interface ErrorDisplayProps {
  title?: string
  message?: string
  error?: Error | null
  onRetry?: () => void
  retryText?: string
  icon?: React.ComponentType<{ className?: string }>
  className?: string
  showStack?: boolean
}

/**
 * 区域级错误组件
 * 适合在较小的区域显示错误
 * 
 * @example
 * ```tsx
 * <ErrorInline 
 *   message="发生错误" 
 *   error={error} 
 *   onRetry={() => refetch()} 
 * />
 * ```
 */
export function ErrorInline({
  message,
  error,
  onRetry,
  className,
}: Omit<ErrorDisplayProps, 'title' | 'icon' | 'showStack'>) {
  const errorMessage = message || error?.message || "发生未知错误"

  return (
    <div className={cn("flex items-center gap-2 text-sm text-red-600 dark:text-red-400", className)}>
      <AlertCircle className="size-3 shrink-0 text-destructive" />
      <span className="flex-1 text-xs">{errorMessage}</span>
      {onRetry && (
        <Button
          onClick={onRetry}
          variant="secondary"
          size="sm"
          className="h-6 px-4 text-xs"
        >
          重试
        </Button>
      )}
    </div>
  )
}
