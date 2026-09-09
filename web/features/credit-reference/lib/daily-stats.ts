import type { Order } from "./services/transaction/types"

export type DailyStats = {
  days: { date: string; income: string; expense: string }[]
  total_income: string
  total_expense: string
}

function shanghaiDate(value: Date): string {
  const parts = new Intl.DateTimeFormat("en", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(value)
  const part = (type: string) => parts.find(p => p.type === type)!.value
  return `${part("year")}-${part("month")}-${part("day")}`
}

// Aggregate every record, independent of the activity table's filter/page.
// Sum whole credits from the same source as the activity records.
export function dailyStats(orders: Order[], userID: string, now = new Date()): DailyStats {
  const today = shanghaiDate(now)
  const midnight = Date.parse(`${today}T00:00:00+08:00`)
  const days = Array.from({ length: 7 }, (_, index) => ({ date: shanghaiDate(new Date(midnight - index * 86400000)), income: 0, expense: 0 }))
  for (const order of orders) {
    if (order.status !== "success" || order.payer_user_id === order.payee_user_id) continue
    if (!/^\d+$/.test(order.amount)) continue
    const amount = Number(order.amount)
    if (!Number.isSafeInteger(amount) || amount <= 0) continue
    const add = (value: string, direction: "income" | "expense") => {
      const time = new Date(value)
      if (!Number.isFinite(time.getTime()) || time > now) return
      const day = days.find(day => day.date === shanghaiDate(time))
      if (day) day[direction] += amount
    }
    if (order.payer_user_id === userID) add(order.trade_time, "expense")
    if (order.payee_user_id === userID && order.payee_transfer_status === "completed") add(order.payee_transfer_at || order.trade_time, "income")
  }
  const format = (amount: number) => String(amount)
  return {
    days: days.map(day => ({ date: day.date, income: format(day.income), expense: format(day.expense) })),
    total_income: format(days.reduce((total, day) => total + day.income, 0)),
    total_expense: format(days.reduce((total, day) => total + day.expense, 0)),
  }
}
