import { redirect } from "next/navigation"
import { wikiUrl } from "@/lib/wiki"

export default function IntegrationPage() { redirect(wikiUrl("/integration")) }
