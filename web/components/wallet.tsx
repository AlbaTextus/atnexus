"use client"

import { BalanceMain } from "@/features/credit-reference/components/common/balance/balance-main"
import { WalletBalanceProvider } from "@/features/credit-reference/preview"

export function AccountWallet() {
  return <WalletBalanceProvider><BalanceMain /></WalletBalanceProvider>
}
