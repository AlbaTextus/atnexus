import type { ElementType, ReactNode } from "react"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { BrandWordmark } from "@/components/brand-wordmark"

export type AuthCardProps = {
  title: string
  titleId: string
  description?: string
  children: ReactNode
  footer: ReactNode
  brand?: ReactNode
  brandHref?: string
  LinkComponent?: ElementType
}

export function AuthCard({ title, titleId, description, children, footer, brand, brandHref = "/", LinkComponent }: AuthCardProps) {
  return <Card variant="auth" className="auth-card w-full max-w-md" aria-labelledby={titleId}>
    <CardHeader>
      {brand ?? <BrandWordmark variant="auth" href={brandHref} LinkComponent={LinkComponent} />}
      <CardTitle><h1 id={titleId}>{title}</h1></CardTitle>
      {description && <CardDescription>{description}</CardDescription>}
    </CardHeader>
    <CardContent>{children}</CardContent>
    <CardFooter>{footer}</CardFooter>
  </Card>
}
