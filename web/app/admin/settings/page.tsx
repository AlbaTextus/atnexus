import { AdminKeyLimits } from "@/components/admin-key-limits"
import { MaintenanceControl } from "@/components/maintenance-control"
import { AdminInterfaces } from "@/components/admin-interfaces"

export default function AdminSettingsPage() {
  return <section className="space-y-7 [&>[data-slot=card]]:border [&>[data-slot=card]]:border-border [&>[data-slot=card]]:shadow-none [&>[data-slot=card]]:ring-0" aria-labelledby="admin-settings-title">
    <h1 id="admin-settings-title" className="text-2xl font-semibold tracking-tight">配置</h1>
    <MaintenanceControl />
    <AdminInterfaces />
    <AdminKeyLimits />
  </section>
}
