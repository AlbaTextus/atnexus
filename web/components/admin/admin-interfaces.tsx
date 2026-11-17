"use client"

import { useEffect, useState } from "react"
import { Select } from "@base-ui/react/select"
import { AlertCircle, Check, ChevronDown, RefreshCw } from "lucide-react"
import { api } from "@/lib/api"
import { feedback } from "@/lib/feedback"
import { type AccessRule, type Endpoint, type Operation, type RolePolicy, endpointNames, operationNames, permissionLevels, publicEndpointPath } from "@/lib/athub/athub-permissions"
import { useAccountUser } from "@/components/account/account-shell"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Skeleton } from "@/components/ui/skeleton"

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
    <Select.Trigger aria-label={label} render={<Button variant="outline" className="h-8 w-full max-w-44 justify-between gap-2 px-2.5 font-normal" />}>
      <Select.Value className="min-w-0 flex-1 text-left">{content(value)}</Select.Value>
      <Select.Icon><ChevronDown className="size-3.5 text-muted-foreground transition-transform in-aria-expanded:rotate-180" /></Select.Icon>
    </Select.Trigger>
    <Select.Portal className="pointer-events-none fixed inset-0 z-50" style={{ zoom: "calc(1 / var(--site-root-scale, var(--site-scale, 1)))" }}>
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
  const changedLevels = permissionLevels.filter(level => drafts[level.role] && JSON.stringify(drafts[level.role]) !== JSON.stringify(saved[level.role]))
  const dirty = changedLevels.length > 0
  const disabled = !editable || busy
  function changeRule(role: string, rule: DraftRule) {
    setDrafts(current => ({ ...current, [role]: { ...current[role], rules: [...current[role].rules.filter(item => ruleKey(item) !== ruleKey(rule)), rule].sort((a, b) => ruleKey(a).localeCompare(ruleKey(b))) } }))
  }
  function inherit(role: string, endpoint: Endpoint) {
    setDrafts(current => ({ ...current, [role]: { ...current[role], rules: current[role].rules.filter(item => ruleKey(item) !== ruleKey(endpoint)) } }))
  }
  async function save() {
    if (!dirty || disabled) return
    setBusy(true); setError("")
    try {
      const results = await Promise.allSettled(changedLevels.map(async level => {
        const updated = await api<RolePolicy>("/api/v1/admin/athub/permissions", { method: "PUT", body: JSON.stringify(drafts[level.role]) })
        const value = toDraft(updated)
        setDrafts(current => ({ ...current, [level.role]: value }))
        setSaved(current => ({ ...current, [level.role]: value }))
        return level.label
      }))
      const failures = results.flatMap((result, index) => result.status === "rejected"
        ? [`${changedLevels[index].label}：${result.reason instanceof Error ? result.reason.message : "保存失败"}`] : [])
      const completed = results.flatMap(result => result.status === "fulfilled" ? [result.value] : [])
      if (completed.length) feedback.success(`${completed.join("、")} 接口权限已保存`)
      if (failures.length) setError(failures.join("；"))
    } catch (reason) { setError(reason instanceof Error ? reason.message : "保存失败") }
    finally { setBusy(false) }
  }
  function permissionSelect(role: string, endpoint: Endpoint) {
    const draft = drafts[role]
    if (!draft) return <span className="text-muted-foreground">—</span>
    const override = draft.rules.find(rule => ruleKey(rule) === ruleKey(endpoint))
    const inheritedAllowed = Boolean(draft.rules.find(rule => rule.operation === endpoint.operation && rule.method === "")?.allowed)
    const level = permissionLevels.find(item => item.role === role)?.label ?? role
    return <EndpointPermissionSelect label={`${level} ${endpoint.method} ${publicEndpointPath(endpoint.path)}权限`}
      disabled={disabled || !privileged(role, endpoint.operation)} inheritedAllowed={inheritedAllowed}
      value={override ? override.allowed ? "allow" : "deny" : "inherit"}
      onValueChange={value => value === "inherit" ? inherit(role, endpoint) : changeRule(role, { ...endpoint, allowed: value === "allow" })} />
  }
  function columns() {
    return <colgroup><col style={{ width: "28%" }} />{permissionLevels.map(level => <col key={level.role} style={{ width: "18%" }} />)}</colgroup>
  }
  function restoreDefaults() {
    setDrafts(current => Object.fromEntries(Object.entries(current).map(([role, draft]) => [role, {
      ...draft,
      review_required: role === "LEVEL_2",
      rules: operations.map(operation => ({ operation, method: "", path: "", allowed: defaultAllowed(role, operation) })),
    }])))
  }
  return <Card data-interface-permissions="">
    <CardHeader>
      <div className="flex items-center justify-between gap-3"><CardTitle><h2>接口权限</h2></CardTitle><Button variant="outline" size="icon" aria-label="重新加载接口权限" disabled={loading || busy} onClick={() => setRevision(value => value + 1)}><RefreshCw /></Button></div>
    </CardHeader>
    <CardContent className="space-y-5">
      {error && <Alert variant="destructive"><AlertCircle /><AlertDescription>{error}</AlertDescription></Alert>}
      {loading ? <Skeleton className="h-80 w-full" /> : Object.keys(drafts).length > 0 && <>
        <div className="overflow-x-auto rounded-xl border border-border">
          <table aria-label="等级权限对照" className="w-full table-fixed text-left text-sm">
            {columns()}
            <thead className="bg-muted/45"><tr className="border-b border-border">
              <th scope="col" className="px-4 py-3 font-medium">操作</th>
              {permissionLevels.map(level => <th key={level.role} scope="col" className="px-2 py-3 text-center font-medium">{level.label}</th>)}
            </tr></thead>
            <tbody>{operations.map(operation => <tr key={operation} className="border-b border-border/60 last:border-0 hover:bg-muted/20">
              <th scope="row" className="px-4 py-3.5 font-normal">{operationNames[operation]}</th>
              {permissionLevels.map(level => {
                const rule = drafts[level.role]?.rules.find(item => item.operation === operation && item.method === "")
                return <td key={level.role} className="px-2 py-3.5 text-center">{rule
                  ? <Checkbox className="mx-auto" aria-label={`${level.label} ${operationNames[operation]}权限`} checked={rule.allowed} disabled={disabled || !privileged(level.role, operation)} onCheckedChange={value => changeRule(level.role, { ...rule, allowed: Boolean(value) })} />
                  : <span className="text-muted-foreground">—</span>}</td>
              })}
            </tr>)}</tbody>
            <tbody className="border-t border-border">
              <tr className="border-b border-border/60"><th scope="row" className="px-4 py-4 font-normal">作品资料编辑</th>
                {permissionLevels.map(level => <td key={level.role} className="px-2 py-3 text-center">{permissionSelect(level.role, novelEditEndpoint)}</td>)}
              </tr>
              <tr><th scope="row" className="px-4 py-4 font-normal">内容写入需审核<span className="sr-only">（含作品资料编辑）</span></th>
                {permissionLevels.map(level => <td key={level.role} className="px-2 py-4 text-center">{drafts[level.role]
                  ? <Checkbox className="mx-auto" aria-label={`${level.label} 内容写入（含作品资料编辑）需审核后生效`} checked={drafts[level.role].review_required} disabled={disabled} onCheckedChange={value => setDrafts(current => ({ ...current, [level.role]: { ...current[level.role], review_required: Boolean(value) } }))} />
                  : <span className="text-muted-foreground">—</span>}</td>)}
              </tr>
            </tbody>
          </table>
        </div>
        <details className="group rounded-xl border border-border">
          <summary className="flex cursor-pointer list-none items-center justify-between px-4 py-3.5 text-sm font-medium [&::-webkit-details-marker]:hidden">具体接口规则<ChevronDown className="size-4 text-muted-foreground transition-transform group-open:rotate-180" /></summary>
          <div className="overflow-x-auto border-t border-border">
            <table aria-label="具体接口权限对照" className="w-full table-fixed text-left text-sm">
              {columns()}
              <thead className="bg-muted/45"><tr className="border-b border-border"><th scope="col" className="px-4 py-3 font-medium">接口</th>{permissionLevels.map(level => <th key={level.role} scope="col" className="px-2 py-3 text-center font-medium">{level.label}</th>)}</tr></thead>
              <tbody>{operations.flatMap(operation => endpoints.filter(endpoint => endpoint.operation === operation && ruleKey(endpoint) !== ruleKey(novelEditEndpoint)).map(endpoint => <tr key={ruleKey(endpoint)} className="border-b border-border/60 last:border-0 hover:bg-muted/20">
                <th scope="row" className="px-4 py-3 font-normal"><p>{operationNames[endpoint.operation]} · {endpointNames[endpoint.path] ?? publicEndpointPath(endpoint.path)}</p><code className="break-all text-xs text-muted-foreground">{endpoint.method} {publicEndpointPath(endpoint.path)}</code></th>
                {permissionLevels.map(level => <td key={level.role} className="px-2 py-3 text-center">{permissionSelect(level.role, endpoint)}</td>)}
              </tr>))}</tbody>
            </table>
          </div>
        </details>
        {editable ? <div className="flex items-center justify-end gap-3 pt-1">
          <Button variant="outline" disabled={busy} onClick={restoreDefaults}>恢复默认权限</Button>
          <Button disabled={busy || !dirty} onClick={() => void save()}>{busy ? "保存中…" : "保存权限"}</Button>
        </div> : <p className="text-sm text-muted-foreground">接口权限由超级管理员修改。</p>}
      </>}
    </CardContent>
  </Card>
}
