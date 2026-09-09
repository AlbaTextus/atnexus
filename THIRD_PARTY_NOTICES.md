# Third-party code

The wallet balance layout, balance summary, transaction table, payment checkout
layout, and table UI components are adapted from
[LINUX DO Credit](https://github.com/linux-do/credit), Copyright 2025 linux.do,
under the Apache License 2.0. The license is included in
`licenses/linux-do-credit-LICENSE`.

Adaptations connect these components to ALBATEXTUS account and wallet APIs,
use this application's theme and installed UI primitives, confirm orders with
the current account session, and remove unsupported community, reports, fees, and promotional features.

Source paths: `frontend/components/common/balance/`,
`frontend/components/common/general/table-data.tsx`,
`frontend/components/common/pay/paying/`, and
`frontend/components/ui/table.tsx`.

The account-center wallet additionally reuses the original balance page, full
virtualized transaction table, transaction context, number animation and their
supporting Radix components in `web/features/credit-reference`. Imports are
namespaced; an authenticated wallet adapter connects the UI to the Go service. The reference sidebar,
header and report section have been removed, and the account shell provides
navigation. See that directory's README for the explicit adaptations.

The wallet trend is adapted from `frontend/components/common/home/data-panel.tsx`
and reuses `frontend/components/ui/chart.tsx`, with the wallet daily-statistics
adapter, loading-error retry, and zero-valued tooltip entries.
