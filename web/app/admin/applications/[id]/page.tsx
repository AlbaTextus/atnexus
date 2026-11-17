import { ApplicationEditor } from "@/components/admin/application-editor"

export default async function ApplicationPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  return <ApplicationEditor key={id} id={id} />
}
