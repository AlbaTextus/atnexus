import { AuthLayout } from "@/components/auth/auth-layout"
import { LoginForm } from "@/components/auth/login-form"

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ return_to?: string; oauth_error?: string }> }) {
  const params = await searchParams
  return <AuthLayout appearance="white" background="identity"><LoginForm returnTo={params.return_to || null} oauthError={params.oauth_error} /></AuthLayout>
}
