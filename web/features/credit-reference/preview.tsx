"use client"
import type { ReactNode } from "react"
import { UserProvider } from "./contexts/user-context"
import "@fontsource-variable/inter"
import "@fontsource/noto-sans-sc/400.css"
import "@fontsource/noto-sans-sc/500.css"
import "@fontsource/noto-sans-sc/600.css"
import "@fontsource/noto-sans-sc/700.css"
import "./theme.css"

export function WalletBalanceProvider({ children }: { children: ReactNode }) {
  return <WalletAppearance><UserProvider>{children}</UserProvider></WalletAppearance>
}

export function WalletAppearance({ children }: { children: ReactNode }) {
  return <div className="credit-reference" style={{ '--font-credit-inter': '"Inter Variable"', '--font-credit-noto': '"Noto Sans SC"' } as React.CSSProperties}>{children}</div>
}
