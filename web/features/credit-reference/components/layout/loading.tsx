// Copied from linux-do/credit, Apache-2.0. Only imports adapted for this preview.
import * as React from "react"
import { cn } from "@/features/credit-reference/lib/utils"
import { FileTextIcon } from "lucide-react"

interface LoadingStateProps extends React.ComponentProps<"div"> {
  title?: string
  description?: string
  icon?: React.ComponentType<{ className?: string }>
  iconSize?: "sm" | "md" | "lg"
}

/**
 * 加载状态展示组件
 * 用于统一显示加载中的状态
 */
export function LoadingState({
  title = "加载中",
  description = "正在获取活动数据...",
  icon: Icon = FileTextIcon,
  className,
  iconSize = "md",
}: LoadingStateProps) {
  const iconSizes = { sm: "size-8", md: "size-10", lg: "size-14" }
  const iconInnerSizes = { sm: "size-4", md: "size-5", lg: "size-7" }

  return (
    <div className={cn("flex flex-col items-center justify-center py-12 text-center", className)}>
      <div className={cn(
        "rounded-full bg-muted flex items-center justify-center mb-2 animate-pulse",
        iconSizes[iconSize]
      )}>
        <Icon className={cn("text-muted-foreground", iconInnerSizes[iconSize])} />
      </div>

      {title && (
        <h3 className="text-sm font-medium mb-1 animate-pulse">
          {title}
        </h3>
      )}

      {description && (
        <p className="text-xs text-muted-foreground max-w-md animate-pulse">
          {description}
        </p>
      )}
    </div>
  )
}

/**
 * 带边框的加载状态组件
 */
export function LoadingStateWithBorder(props: LoadingStateProps) {
  return (
    <div className="border border-dashed rounded-lg">
      <LoadingState {...props} />
    </div>
  )
}
