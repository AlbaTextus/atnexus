# Wallet reference UI

The account-center wallet (`/account/wallet`, resolving to
`/account/{userId}/wallet`) uses the reference BalanceMain, BalanceSummary,
BalanceTable and virtualized transaction table. The remaining four activity tabs
and styling are preserved. The reference sidebar, top toolbar and reports were
removed; navigation comes from the existing account shell.

The separate `/balance` page and `/balance/data` endpoint have been deleted.

The summary uses the account identity card's rounded gradient surface and typography.
One card directly displays total credits (available plus pending), available credits
and future credits. It replaces the standalone balance heading and trend chart.
The original seven-day statistics card retains its daily details and refresh action,
followed by recent activity. All balances are visible without hovering or flipping.

The original purple/gray balance donut fills the space to the right of both cards
on wide screens, with segment names and amounts shown on hover or keyboard focus.
On narrower screens it wraps below the cards without shrinking their content.

Each activity's Details button opens a receipt-style dialog: the payment card's
dark header presents the amount and actual order status, while the identity card's
gradient surface holds the transaction facts. A dashed divider and serrated bottom
form the receipt edge. The order number can be copied; additional fields expand
under More information. The activity action column contains only the receipt icon;
dispute and refund-action entries are not shown.

Data is read through `GET /account/wallet/data?resource=user`, `resource=transactions`
and `resource=daily-stats`. The server route forwards the current session to
`GET /api/v1/account/wallet/view` and returns private, uncached JSON. All data now
comes from PostgreSQL; there is no mock-file fallback or preview enable flag.
The existing namespaced UI contract is retained to preserve layout and interactions.
The payment page confirms orders using the current account session.

New payment receipts enter future credits for exactly 24 hours from successful
payment. Due receipts are settled by the Go worker and on wallet access; the tooltip
uses the real scheduled arrival time. Available plus future credits is the total.
Existing settled balances are not frozen again. Administrative adjustments are immediate.

The separate seven-day statistics card reads `resource=daily-stats` from the same endpoint.
It aggregates all fixture transactions independently of the table's pagination:
completed incoming transfers count on their scheduled settlement date, and successful outgoing
payments count on their trade date. Dates use Asia/Shanghai, include today and the
previous six days, and missing days are zero-filled.

Reference color variables are scoped to the wallet content; they do not replace
the account shell's theme. The original Radix components are namespaced here to
avoid overwriting the account center's Base UI components. The license is in
`licenses/linux-do-credit-LICENSE`; see `THIRD_PARTY_NOTICES.md`.
