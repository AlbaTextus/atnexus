import type { Metadata } from "next"
import { WikiDocument } from "@/components/wiki-document"
import { IntegrationEndpoints } from "@/components/integration-endpoints"
import { integrationContent, integrationDescription, integrationTitle } from "@/lib/wiki-integration"

export const metadata: Metadata = { title: "ALBATEXTUS", description: integrationDescription }

export default function IntegrationPage() {
  return <WikiDocument title={integrationTitle} description={integrationDescription} content={integrationContent}>
    <IntegrationEndpoints />
  </WikiDocument>
}
