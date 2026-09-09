"use client"

import { useEffect, useState, type FormEvent } from "react"
import { AlertCircle, Check } from "lucide-react"
import { api, ApiError } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { creditAmount, type Wallet, type WalletEntry } from "@/lib/wallet"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { UserCreditHistory } from "@/components/user-credit-history"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { BalanceCard } from "@/features/credit-reference/components/common/balance/balance-summary"
import { WalletAppearance } from "@/features/credit-reference/preview"

type ManagedCredits = { numeric_id: number; is_active: boolean; wallet: Wallet & { pending_balance?: string }; entries: WalletEntry[]; has_more: boolean }

export function UserCredits({ userID, onLockedChange }: { userID: number; onLockedChange: (locked: boolean) => void }) {
  const [target, setTarget] = useState<ManagedCredits | null>(null)
  const [amount, setAmount] = useState("")
  const [reason, setReason] = useState("")
  const [key, setKey] = useState("")
  const [result, setResult] = useState<WalletEntry | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [loading, setLoading] = useState(true)
  const [reload, setReload] = useState(0)
  const [loadError, setLoadError] = useState("")
  const [error, setError] = useState("")

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setLoadError("")
    api<ManagedCredits>(`/api/v1/admin/wallet/users/${userID}`, { signal: controller.signal })
      .then(data => { if (!controller.signal.aborted) setTarget(data) })
      .catch(reason => { if (!controller.signal.aborted) setLoadError(managementError(reason)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [userID, reload])

  useEffect(() => {
    onLockedChange(submitting || Boolean(key && !result))
    return () => onLockedChange(false)
  }, [submitting, key, result, onLockedChange])

  async function adjust(event: FormEvent) {
    event.preventDefault()
    if (submitting || loading || !target || result) return
    setError("")
    if (!/^-?[1-9][0-9]*$/.test(amount) || !reason.trim()) {
      setError("请输入非零整数积分和调整原因。")
      return
    }
    if (!key) { setKey(crypto.randomUUID()); return }
    setSubmitting(true); setAttempted(true)
    try {
      const entry = await api<WalletEntry>("/api/v1/admin/wallet/adjustments", { method: "POST", body: JSON.stringify({ user_numeric_id: userID, amount, reason: reason.trim(), idempotency_key: key }) })
      setResult(entry)
      setReload(value => value + 1)
    } catch (reason) {
      setError(managementError(reason))
      if (reason instanceof ApiError && reason.status >= 400 && reason.status < 500) setAttempted(false)
    }
    finally { setSubmitting(false) }
  }

  const locked = submitting || Boolean(key && !result)
  return <section aria-label="用户积分" className="flex min-w-0 flex-col gap-4">
    {loadError && <Alert variant="destructive"><AlertCircle /><AlertDescription role="alert">{loadError}<Button variant="link" disabled={loading} onClick={() => setReload(value => value + 1)}>重新加载积分</Button></AlertDescription></Alert>}
    {loading && !target && <Skeleton className="h-36 w-full" aria-label="正在加载积分" />}
    {target && <>
      <div className="managed-user-credit-card">
        <h2>积分卡</h2>
        <WalletAppearance><BalanceCard available={Number(result?.balance_after ?? target.wallet.balance)} pending={Number(target.wallet.pending_balance || "0")} loading={loading} headingLevel={3} /></WalletAppearance>
        <div className="account-member-controls"><UserCreditHistory userID={userID} disabled={locked} /></div>
      </div>
      <form onSubmit={adjust} className="managed-user-credit-adjustment">
        <Card>
          <CardHeader><CardTitle>积分调整</CardTitle>{!target.is_active && <CardDescription>账户已停用</CardDescription>}</CardHeader>
          <CardContent>
            {error && <Alert variant="destructive" className="mb-4"><AlertCircle /><AlertDescription role="alert">{error}</AlertDescription></Alert>}
            {result ? <Alert><Check /><AlertDescription role="status">积分调整已完成：{creditAmount(result.amount)} 积分。流水编号 {result.id}</AlertDescription></Alert> : <FieldGroup>
              <Field><FieldLabel htmlFor={`credits-amount-${userID}`}>积分调整</FieldLabel><Input id={`credits-amount-${userID}`} required inputMode="text" pattern="-?[1-9][0-9]*" maxLength={16} disabled={locked || !target.is_active} value={amount} onChange={event => setAmount(event.target.value)} /><FieldDescription>正数增加积分，负数扣除积分；仅支持非零整数。</FieldDescription></Field>
              <Field><FieldLabel htmlFor={`credits-reason-${userID}`}>调整原因</FieldLabel><Input id={`credits-reason-${userID}`} required maxLength={200} disabled={locked || !target.is_active} value={reason} onChange={event => setReason(event.target.value)} /></Field>
              {key && <Alert><AlertCircle /><AlertDescription>请确认：为用户 {userID} {amount.startsWith("-") ? "扣除" : "增加"} {creditAmount(amount.replace(/^-/, ""))} 积分，原因：{reason.trim()}。若请求结果未确认，请重试本笔操作。</AlertDescription></Alert>}
            </FieldGroup>}
          </CardContent>
          <CardFooter className="flex flex-wrap gap-3">
            {result ? <Button type="button" disabled={loading || Boolean(loadError)} onClick={() => { setResult(null); setKey(""); setAmount(""); setReason(""); setAttempted(false); setError("") }}>继续调整</Button> : <>
              <Button type="submit" disabled={submitting || loading || Boolean(loadError) || !target.is_active || !amount || !reason.trim()}>{submitting && <Spinner data-icon="inline-start" />}{key ? "确认调整" : "核对调整"}</Button>
              {key && !attempted && <Button type="button" variant="outline" disabled={submitting} onClick={() => setKey("")}>返回修改</Button>}
            </>}
          </CardFooter>
        </Card>
      </form>
    </>}
  </section>
}
