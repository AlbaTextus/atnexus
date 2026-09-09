// Copied from linux-do/credit's /home OverviewCard (Apache-2.0).
// Adapted refresh accessibility, compact typography and footer-free layout.
import * as React from "react"
import { RefreshCw } from "lucide-react"
import { Button } from "@/features/credit-reference/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/features/credit-reference/components/ui/card"
import { cn } from "@/features/credit-reference/lib/utils"

interface OverviewCardProps {
  title: string
  titleExtra?: React.ReactNode
  headerExtra?: React.ReactNode
  action?: React.ReactNode
  loading?: boolean
  onRefresh?: () => void
  children: React.ReactNode
  className?: string
}

export function OverviewCard({
  title,
  titleExtra,
  headerExtra,
  action,
  loading,
  onRefresh,
  children,
  className
}: OverviewCardProps) {
  return (
    <Card className={cn("bg-background border-0 shadow-none rounded-xl min-h-[200px] flex flex-col h-full", className)}>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4 h-6">
            <CardTitle className="text-sm font-medium tracking-tight">{title}</CardTitle>
            {titleExtra && <div className="font-semibold">{titleExtra}</div>}
          </div>
          <div className="flex items-center gap-2">
            {action}
            {onRefresh && (
              <Button variant="ghost" size="icon" className="size-7 rounded-full text-muted-foreground" onClick={onRefresh} disabled={loading} aria-label="刷新收支统计">
                <RefreshCw className={cn("size-3.5", loading && "animate-spin motion-reduce:animate-none")} />
              </Button>
            )}
          </div>
        </div>
        {headerExtra && <div className="pt-1">{headerExtra}</div>}
      </CardHeader>
      <CardContent className="relative flex-1 -mt-4">
        {children}
      </CardContent>
    </Card>
  )
}
