# API conventions

## Pagination

Every list endpoint takes `page` (from 1) and `limit`, and returns:

```json
{ "items": [...], "meta": { "page": 1, "limit": 20, "total": 57, "total_pages": 3 } }
```

Applies to `/payments`, `/admin/payments`, `/invoice`, `/transaction` and `/notification`.
Old parameter names (`page_count`, `count`) and old response keys (`payments`, `invoice`, `data`, `notifications`,
`meta.page_count`, `meta.total_invoice_count`) still work and are deprecated. Remove them once the frontend has moved.
`/invoice/query` takes `page` and `limit` (max 50).

## Amounts

| Kind | Format | Example |
| --- | --- | --- |
| Payment records, payment requests, `expected.amount` | string, 7 decimals, in the asset's own units | `"2.0300000"` |
| Invoice and ledger (`/transaction`) fields | JSON number, in the invoice currency (dollars for USDC) | `2.03` |
| `/transaction/stats` | integer, cents (1/100 of a dollar), with `invoice_count` | `203` |
| `/invoice/stats` | counts only | |

Payment amounts are strings so they never lose precision. Compare them as decimals, never as floats.
`/transaction/stats` returns `expired_amount` and `invalid_payment_amount` next to `overdue_amount`.
`overdue_amount` is kept for compatibility. Invoices past due are now moved to `expired` after the payment window.
