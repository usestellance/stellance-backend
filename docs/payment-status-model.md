# Payment status model

Three value lists exist. Verification status lives on a payment record. Invoice status and payment status live on the invoice.

## verification_status (payment record)

| Value | Meaning |
| --- | --- |
| `pending` | In flight: accepted by the API, not yet checked against its invoice. Clients must accept it. The poller currently stores in-flight payments as `detected`. |
| `detected` | Payment seen on chain and being checked. Short lived: settled in the same poll pass, and recovered within 30 seconds if the server stops mid-settlement. |
| `verified` | All checks passed. The invoice is paid. |
| `invalid` | Matched an invoice but failed a check (see `failure_code`). |
| `unmatched` | No invoice for the reference, or the recipient is not the invoice recipient. |
| `failed` | The Stellar transaction itself failed. |

## failure_code

`insufficient_amount`, `amount_mismatch` (overpayment), `wrong_asset`, `wrong_recipient`, `missing_reference`, `unknown_reference`, `transaction_failed`, `duplicate` (invoice already paid), `invoice_not_payable`, `expired`, `invalid_invoice_total`. `null` when verified.

## checks

Each of `recipient`, `asset`, `amount`, `reference`, `success`, `invoice` is `pass`, `fail` or `skip`. `skip` means the check could not run, for example every invoice check when no invoice matches the reference. Records stored before this field existed were backfilled by migration 000033 from the stored record and its invoice. `checks` is never null after that migration. `expected` is null only when no invoice matches the reference. For backfilled rows the asset issuer and expected recipient are best effort.

## invoice status

`draft`, `sent`, `viewed`, `pending`, `overdue`, `payment_detected`, `invalid_payment`, `paid`, `expired`, `cancelled`, `refunded`.

`payment_detected` is held only while a payment is being settled, so clients rarely see it.

## payment_status (derived from invoice status)

`awaiting_payment` for `sent`, `viewed`, `pending` and `overdue`. Every other invoice status maps to itself.

## Transitions

- `sent`/`viewed`/`pending`/`overdue` -> `payment_detected` -> `paid` (payment verified)
- `payment_detected` -> `invalid_payment` (payment matched the invoice but failed a check)
- `invalid_payment` -> `paid` (a later correct payment verifies)
- `sent`/`viewed`/`pending`/`overdue`/`invalid_payment` -> `expired` (payment window ended; see `PAYMENT_EXPIRY_GRACE_DAYS`)
- `paid` never changes. A later payment to it is recorded `invalid` with `failure_code: duplicate`, linked to the invoice.

## Wallet history

`/transaction` lists every incoming payment once with type `payment`. Its `status` is the on-chain result: `confirmed` when the Stellar transaction succeeded, `failed` when it did not. The verification result stays on the payment record.
