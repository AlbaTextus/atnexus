"use client"

import { useEffect, useState } from "react"
import { Select } from "@base-ui/react/select"
import { AlertCircle, Check, ChevronDown, RefreshCw } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import { type AccessRule, type Endpoint, type Operation, type RolePolicy, endpointNames, operationNames, permissionLevels } from "@/lib/athub-permissions"
import { useAccountUser } from "@/components/account-shell"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Skeleton } from "@/components/ui/skeleton"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

type DraftRule = AccessRule
type Draft = Omit<RolePolicy, "rules"> & { rules: DraftRule[] }
const ruleKey = (rule: { operation: string; method: string; path: string }) => `${rule.operation}:${rule.method}:${rule.path}`
const toDraft = (policy: RolePolicy): Draft => ({ ...policy, rules: policy.rules.map(({ operation, method, path, allowed }) => ({ operation, method, path, allowed })) })
const operations: Operation[] = ["read", "upload", "edit", "review", "delete"]
const novelEditEndpoint: Endpoint = { operation: "edit", method: "PUT", path: "/api/v2/novelinfo" }
function privileged(role: string, operation: Operation) {
  return operation === "delete" ? role === "SUPER_ADMIN" : operation === "review" ? role === "ADMIN" || role === "SUPER_ADMIN" : true
}
function defaultAllowed(role: string, operation: Operation) {
  return operation === "read" || (operation === "upload" || operation === "edit" ? role !== "USER" : privileged(role, operation))
}

type PermissionValue = "inherit" | "allow" | "deny"

function EndpointPermissionSelect({ value, inheritedAllowed, disabled, label, onValueChange }: {
  value: PermissionValue
  inheritedAllowed: boolean
  disabled: boolean
  label: string
  onValueChange: (value: PermissionValue) => void
}) {
  const inheritedLabel = inheritedAllowed ? "允许" : "禁止"
  const items = [
    { value: "inherit", label: `沿用分类：${inheritedLabel}` },
    { value: "allow", label: "允许" },
    { value: "deny", label: "禁止" },
  ] as const
  const statusClass = (allowed: boolean) => allowed ? "text-emerald-700 dark:text-emerald-400" : "text-rose-700 dark:text-rose-400"
  const content = (option: PermissionValue) => option === "inherit"
    ? <span className="flex flex-1 items-center justify-between gap-3"><span className="text-muted-foreground">沿用分类</span><span className={statusClass(inheritedAllowed)}>{inheritedLabel}</span></span>
    : <span className={statusClass(option === "allow")}>{option === "allow" ? "允许" : "禁止"}</span>

  return <Select.Root value={value} items={items} disabled={disabled}
    onValueChange={next => { if (next) onValueChange(next) }}>
    <Select.Trigger aria-label={label} render={<Button variant="outline" className="h-8 w-44 justify-between gap-3 px-3 font-normal" />}>
      <Select.Value className="min-w-0 flex-1 text-left">{content(value)}</Select.Value>
      <Select.Icon><ChevronDown className="size-3.5 text-muted-foreground transition-transform in-aria-expanded:rotate-180" /></Select.Icon>
    </Select.Trigger>
    <Select.Portal className="pointer-events-none fixed inset-0 z-50" style={{ zoom: "calc(1 / var(--site-scale, 1))" }}>
      <Select.Positioner sideOffset={6} align="end" alignItemWithTrigger={false} className="pointer-events-auto isolate z-50">
        <Select.Popup data-slot="permission-select-popup" className="w-44 origin-(--transform-origin) rounded-xl border border-border bg-popover p-1 text-sm text-popover-foreground shadow-lg outline-none"
          style={{ zoom: "var(--site-scale, 1)" }}>
          <Select.List>
            {items.map(item => <Select.Item key={item.value} value={item.value}
              className="relative flex min-h-9 cursor-default select-none items-center gap-3 rounded-lg py-2 pr-3 pl-8 outline-none data-highlighted:bg-accent data-highlighted:text-accent-foreground">
              <Select.ItemIndicator className="absolute left-2.5 flex items-center"><Check className="size-3.5" /></Select.ItemIndicator>
              <Select.ItemText className="flex min-w-0 flex-1">{content(item.value)}</Select.ItemText>
            </Select.Item>)}
          </Select.List>
        </Select.Popup>
      </Select.Positioner>
    </Select.Portal>
  </Select.Root>
}

export function AdminInterfaces() {
  const editable = useAccountUser().role === "SUPER_ADMIN"
  const [saved, setSaved] = useState<Record<string, Draft>>({})
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [endpoints, setEndpoints] = useState<Endpoint[]>([])
  const [role, setRole] = useState("USER")
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError("")
    api<{ policies: RolePolicy[]; endpoints: Endpoint[] }>("/api/v1/admin/athub/permissions", { signal: controller.signal })
      .then(result => {
        if (controller.signal.aborted) return
        const values = Object.fromEntries(result.policies.map(policy => [policy.role, toDraft(policy)]))
        setSaved(values); setDrafts(values); setEndpoints(result.endpoints)
      }).catch(reason => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "加载失败") })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [revision])
  const draft = drafts[role]
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved[role])
  const disabled = !editable || busy
  const novelEditRule = draft?.rules.find(rule => ruleKey(rule) === ruleKey(novelEditEndpoint))
  const novelEditInherited = draft?.rules.find(rule => rule.operation === "edit" && rule.method === "")?.allowed ?? false
  function changeRule(rule: DraftRule) {
    setDrafts(current => ({ ...current, [role]: { ...current[role], rules: [...current[role].rules.filter(item => ruleKey(item) !== ruleKey(rule)), rule].sort((a, b) => ruleKey(a).localeCompare(ruleKey(b))) } }))
  }
  function inherit(endpoint: Endpoint) {
    setDrafts(current => ({ ...current, [role]: { ...current[role], rules: current[role].rules.filter(item => ruleKey(item) !== ruleKey(endpoint)) } }))
  }
  async function save() {
    if (!draft || disabled) return
    setBusy(true); setError("")
    try {
      const updated = await api<RolePolicy>("/api/v1/admin/athub/permissions", { method: "PUT", body: JSON.stringify(draft) })
      const value = toDraft(updated)
      setDrafts(current => ({ ...current, [role]: value })); setSaved(current => ({ ...current, [role]: value }))
      feedback.success(`${permissionLevels.find(item => item.role === role)?.label} 接口权限已保存`)
    } catch (reason) { setError(reason instanceof Error ? reason.message : "保存失败") }
    finally { setBusy(false) }
  }
  return <Card>
    <CardHeader>
      <div className="flex items-center justify-between gap-3"><CardTitle><h2>接口权限</h2></CardTitle><Button variant="outline" size="icon" aria-label="重新加载接口权限" disabled={loading || busy} onClick={() => setRevision(value => value + 1)}><RefreshCw /></Button></div>
      <CardDescription>LT1 查看、下载；LT2 上传、修改并提交审核；LT3 增加审核；LT4 包含删除。</CardDescription>
    </CardHeader>
    <CardContent className="space-y-5">
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
      <ToggleGroup value={[role]} onValueChange={values => { if (values[0]) setRole(values[0]) }} disabled={busy} variant="pill" size="lg" spacing={1} aria-label="接口权限等级">
        {permissionLevels.map(level => <ToggleGroupItem key={level.role} value={level.role}>{level.label}</ToggleGroupItem>)}
      </ToggleGroup>
      {loading ? <Skeleton className="h-56 w-full" /> : draft && <>
        <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b"><th className="pb-3 font-medium">操作</th><th className="pb-3 font-medium">允许</th></tr></thead>
          <tbody>{operations.map(operation => {
            const rule = draft.rules.find(item => item.operation === operation && item.method === "")
            if (!rule) return null
            return <tr key={operation} className="border-b last:border-0"><th scope="row" className="py-3 pr-4 font-normal whitespace-nowrap">{operationNames[operation]}</th>
              <td className="pr-5"><Checkbox aria-label={`${role} ${operationNames[operation]}权限`} checked={rule.allowed} disabled={disabled || !privileged(role, operation)} onCheckedChange={value => changeRule({ ...rule, allowed: Boolean(value) })} /></td>
            </tr>
          })}</tbody></table></div>
        <div className="flex items-center justify-between gap-4 rounded-xl border p-4">
          <span className="text-sm font-medium">作品资料编辑</span>
          <EndpointPermissionSelect value={novelEditRule ? novelEditRule.allowed ? "allow" : "deny" : "inherit"} inheritedAllowed={novelEditInherited}
            disabled={disabled} label={`${role} 作品资料编辑权限`} onValueChange={value => value === "inherit" ? inherit(novelEditEndpoint) : changeRule({ ...novelEditEndpoint, allowed: value === "allow" })} />
        </div>
        <label className="flex items-center gap-3 text-sm"><Checkbox checked={draft.review_required} disabled={disabled} onCheckedChange={value => setDrafts(current => ({ ...current, [role]: { ...current[role], review_required: Boolean(value) } }))} />内容写入（含作品资料编辑）需审核后生效</label>
        <details className="rounded-xl border p-4"><summary className="cursor-pointer text-sm font-medium">具体接口规则</summary><div className="mt-4 space-y-4">{endpoints.filter(endpoint => ruleKey(endpoint) !== ruleKey(novelEditEndpoint)).map(endpoint => {
          const override = draft.rules.find(rule => ruleKey(rule) === ruleKey(endpoint))
          const base = draft.rules.find(rule => rule.operation === endpoint.operation && rule.method === "")
          return <div key={ruleKey(endpoint)} className="flex flex-col gap-2 border-b pb-4 last:border-0 sm:flex-row sm:items-center">
            <div className="min-w-0 flex-1 text-sm"><p>{operationNames[endpoint.operation]} · {endpointNames[endpoint.path]}</p><code className="break-all text-xs text-muted-foreground">{endpoint.method} {endpoint.path}</code></div>
            <EndpointPermissionSelect label={`${endpoint.method} ${endpoint.path}权限`} disabled={disabled || !privileged(role, endpoint.operation)}
              inheritedAllowed={Boolean(base?.allowed)} value={override ? override.allowed ? "allow" : "deny" : "inherit"}
              onValueChange={value => value === "inherit" ? inherit(endpoint) : changeRule({ ...endpoint, allowed: value === "allow" })} />
          </div>
        })}</div></details>
        {editable ? <div className="flex flex-wrap justify-end gap-3">
          <Button variant="outline" disabled={busy} onClick={() => setDrafts(current => ({ ...current, [role]: { ...current[role], review_required: role === "LEVEL_2", rules: operations.map(operation => ({ operation, method: "", path: "", allowed: defaultAllowed(role, operation) })) } }))}>恢复默认权限</Button>
          <Button disabled={busy || !dirty} onClick={() => void save()}>{busy ? "保存中…" : "保存权限"}</Button>
        </div> : <p className="text-sm text-muted-foreground">接口权限由超级管理员修改。</p>}
      </>}
    </CardContent>
  </Card>
}
