import { AuthLayout } from "@/components/auth-layout"
import { RegisterForm } from "@/components/register-form"

export default async function RegisterPage({ searchParams }: { searchParams: Promise<{ return_to?: string }> }) {
  const params = await searchParams
  return <AuthLayout background="identity"><RegisterForm returnTo={params.return_to || null} /></AuthLayout>
}
