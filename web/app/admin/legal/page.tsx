import { redirect } from "next/navigation"
import { wikiUrl } from "@/lib/wiki"

export default function LegalManagementPage() {
  redirect(wikiUrl("/"))
}
