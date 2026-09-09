"use client"

import { useEffect, useState } from "react"
import { AlertCircle, ChevronRight, CircleCheck, ReceiptText, Search, X } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { creditAmount, walletTime, type WalletEntry } from "@/lib/wallet"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { WalletAppearance } from "@/features/credit-reference/preview"
import { WalletReceiptCard, type ReceiptData } from "@/features/credit-reference/components/common/general/order-receipt"

type EntryPage = { entries: WalletEntry[]; has_more: boolean }
const kinds = { payment: "积分支付", receipt: "积分收款", adjustment: "管理员调整" }

function EntryReceipt({ entry, userID }: { entry: WalletEntry; userID: number }) {
  const data: ReceiptData = {
    number: entry.id, numberLabel: "流水编号", name: entry.description,
    amount: `${entry.amount.startsWith("-") ? "" : "+"}${creditAmount(entry.amount)}`,
    status: "已入账", statusIcon: CircleCheck,
    facts: [["类型", kinds[entry.kind] || entry.kind], ["用户 ID", String(userID)], ["入账后余额", `${creditAmount(entry.balance_after)} 积分`], ["入账时间", walletTime(entry.created_at)]],
    extraFacts: [["关联订单编号", entry.order_id || "—"], ["备注", entry.description || "—"]],
  }
  return <Dialog>
    <DialogTrigger render={<Button variant="ghost" className="credit-history-row" />} aria-label={`查看流水 ${entry.id}`}>
      <span className="min-w-0 flex-1 break-all font-mono text-xs">{entry.id}</span>
      <ChevronRight data-icon="inline-end" aria-hidden="true" />
    </DialogTrigger>
    <DialogContent className="credit-reference wallet-receipt-dialog overflow-y-auto" showCloseButton={false}>
      <WalletAppearance><WalletReceiptCard data={data}
        title={<DialogTitle className="wallet-receipt-title">流水详情</DialogTitle>}
        description={<DialogDescription className="sr-only">积分流水小票，包含变动金额、入账余额和流水编号。</DialogDescription>}
        close={<DialogClose render={<Button variant="ghost" size="icon" className="wallet-receipt-close" />} aria-label="关闭流水详情"><X /></DialogClose>} />
      </WalletAppearance>
    </DialogContent>
  </Dialog>
}

export function UserCreditHistory({ userID, disabled }: { userID: number; disabled: boolean }) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [offset, setOffset] = useState(0)
  const [page, setPage] = useState<EntryPage | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    setLoading(true); setError(""); setPage(null)
    const timer = window.setTimeout(() => {
      const query = new URLSearchParams({ offset: String(offset), q: search.trim() })
      api<EntryPage>(`/api/v1/admin/wallet/users/${userID}?${query}`, { signal: controller.signal })
        .then(value => { if (!controller.signal.aborted) setPage(value) })
        .catch(reason => { if (!controller.signal.aborted) setError(managementError(reason)) })
        .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    }, 200)
    return () => { window.clearTimeout(timer); controller.abort() }
  }, [open, userID, search, offset, revision])

  return <Dialog open={open} onOpenChange={value => { setOpen(value); if (!value) { setSearch(""); setOffset(0); setPage(null) } }}>
    <DialogTrigger render={<Button variant="ghost" size="sm" disabled={disabled} />}><ReceiptText data-icon="inline-start" />查看流水</DialogTrigger>
    <DialogContent className="center-dialog credit-history-dialog">
      <DialogHeader><DialogTitle>积分流水</DialogTitle><DialogDescription>用户 {userID} 的全部流水，点击编号查看详情。</DialogDescription></DialogHeader>
      <FieldGroup><Field>
        <FieldLabel htmlFor={`credit-history-search-${userID}`} className="sr-only">搜索流水编号</FieldLabel>
        <InputGroup>
          <InputGroupAddon><Search aria-hidden="true" /></InputGroupAddon>
          <InputGroupInput id={`credit-history-search-${userID}`} placeholder="搜索流水编号或关联订单编号" maxLength={100} value={search} onChange={event => { setSearch(event.target.value); setOffset(0) }} />
        </InputGroup>
      </Field></FieldGroup>
      <div className="credit-history-results" aria-busy={loading}>
        {error ? <Alert variant="destructive"><AlertCircle /><AlertDescription role="alert">{error}<Button variant="link" onClick={() => setRevision(value => value + 1)}>重试</Button></AlertDescription></Alert>
          : loading ? <div className="flex flex-col gap-3" aria-label="正在加载流水"><Skeleton className="h-10 w-full" /><Skeleton className="h-10 w-full" /><Skeleton className="h-10 w-full" /></div>
          : !page?.entries.length ? <Empty><EmptyHeader><EmptyTitle>{search.trim() ? "未找到匹配流水" : "暂无流水"}</EmptyTitle><EmptyDescription>{search.trim() ? "试试其他编号，或清空搜索查看全部流水。" : "产生积分变动后，流水会显示在这里。"}</EmptyDescription></EmptyHeader></Empty>
          : <ul className="flex flex-col gap-1" aria-label="流水编号">{page.entries.map(entry => <li key={entry.id}><EntryReceipt entry={entry} userID={userID} /></li>)}</ul>}
      </div>
      <DialogFooter className="credit-history-pagination">
        <span className="text-xs text-muted-foreground" aria-live="polite">第 {offset / 20 + 1} 页</span>
        <Button variant="outline" size="sm" disabled={loading || offset === 0} onClick={() => setOffset(value => value - 20)}>上一页</Button>
        <Button variant="outline" size="sm" disabled={loading || Boolean(error) || !page?.has_more} onClick={() => setOffset(value => value + 20)}>下一页</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
