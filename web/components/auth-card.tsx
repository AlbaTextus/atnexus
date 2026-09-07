import Link from "next/link"
import type { ReactNode } from "react"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"

type AuthCardProps = {
  title: string
  titleId: string
  description?: string
  children: ReactNode
  footer: ReactNode
  brand?: ReactNode
}

export function AuthCard({ title, titleId, description, children, footer, brand }: AuthCardProps) {
  return (
    <Card variant="auth" className="auth-card w-full max-w-md" aria-labelledby={titleId}>
      <CardHeader>
        {brand ?? <Link href="/" className="auth-brand-mark" aria-label="AlbaTextus 首页">A</Link>}
        <CardTitle><h1 id={titleId}>{title}</h1></CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent>{children}</CardContent>
      <CardFooter>{footer}</CardFooter>
    </Card>
  )
}
