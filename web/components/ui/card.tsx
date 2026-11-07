import * as React from "react"
import { cn } from "cn"

function Card({
  className,
  size = "default",
  variant = "default",
  interactive = false,
  ...props
}: React.ComponentProps<"div"> & { size?: "default" | "sm"; variant?: "default" | "auth" | "payment"; interactive?: boolean }) {
  return (
    <div
      data-slot="card"
      data-size={size}
      data-variant={variant}
      data-interactive={interactive || undefined}
      className={cn(
        "group/card flex flex-col gap-(--card-spacing) overflow-hidden rounded-xl bg-card py-(--card-spacing) text-sm text-card-foreground ring-1 ring-border shadow-(--surface-shadow) transition-[box-shadow,translate] duration-300 [--card-spacing:--spacing(4)] has-data-[slot=card-footer]:pb-0 has-[>img:first-child]:pt-0 data-[size=sm]:[--card-spacing:--spacing(3)] data-[size=sm]:has-data-[slot=card-footer]:pb-0 *:[img:first-child]:rounded-t-xl *:[img:last-child]:rounded-b-xl",
        variant === "auth" && "gap-7 rounded-2xl p-8 shadow-(--auth-card-shadow) ring-border has-data-[slot=card-footer]:pb-8 max-sm:p-6 max-sm:has-data-[slot=card-footer]:pb-6",
        variant === "payment" && "grid gap-0 rounded-3xl border border-border/50 bg-card/70 p-0 shadow-xl ring-0 backdrop-blur-2xl md:grid-cols-[45%_55%]",
        className
      )}
      {...props}
    />
  )
}

function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-header"
      className={cn(
        "group/card-header @container/card-header grid auto-rows-min items-start gap-1 rounded-t-xl px-(--card-spacing) has-data-[slot=card-action]:grid-cols-[1fr_auto] has-data-[slot=card-description]:grid-rows-[auto_auto] [.border-b]:pb-(--card-spacing)",
        "group-data-[variant=auth]/card:justify-items-center group-data-[variant=auth]/card:gap-2 group-data-[variant=auth]/card:px-0 group-data-[variant=auth]/card:text-center",
        className
      )}
      {...props}
    />
  )
}

function CardTitle({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-title"
      className={cn(
        "font-heading text-base leading-snug font-medium group-data-[size=sm]/card:text-sm",
        "group-data-[variant=auth]/card:text-[26px] group-data-[variant=auth]/card:font-semibold group-data-[variant=auth]/card:tracking-tight",
        className
      )}
      {...props}
    />
  )
}

function CardDescription({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-description"
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

function CardAction({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-action"
      className={cn(
        "col-start-2 row-span-2 row-start-1 self-start justify-self-end",
        className
      )}
      {...props}
    />
  )
}

function CardContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-content"
      className={cn("px-(--card-spacing) group-data-[variant=auth]/card:px-0", className)}
      {...props}
    />
  )
}

function CardFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-footer"
      className={cn(
        "flex items-center rounded-b-xl border-t bg-muted/50 p-(--card-spacing)",
        "group-data-[variant=auth]/card:justify-center group-data-[variant=auth]/card:border-0 group-data-[variant=auth]/card:bg-transparent group-data-[variant=auth]/card:p-0",
        className
      )}
      {...props}
    />
  )
}

export type StatCardProps = Omit<React.ComponentProps<typeof Card>, "children"> & {
  label: React.ReactNode
  value?: React.ReactNode
  icon?: React.ReactNode
  note?: React.ReactNode
  tone?: "blue" | "green" | "orange" | "muted"
}

function StatCard({ label, value, icon, note, tone = "muted", className, ...props }: StatCardProps) {
  return <Card className={cn("gap-2 sm:gap-4", className)} {...props}>
    <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 px-3 sm:px-4">
      <CardDescription>{label}</CardDescription>
      {icon != null && <span data-slot="stat-icon" data-tone={tone} aria-hidden="true">{icon}</span>}
    </CardHeader>
    <CardContent className="px-3 sm:px-4">
      <div data-slot="stat-value">{value ?? "—"}</div>
      {note != null && <p className="mt-1 hidden text-xs text-muted-foreground sm:block">{note}</p>}
    </CardContent>
  </Card>
}

export {
  Card,
  StatCard,
  CardHeader,
  CardFooter,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
}
