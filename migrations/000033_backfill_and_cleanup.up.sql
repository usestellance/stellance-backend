-- Older payment records lack per-check results. Rebuild them from the stored record and its invoice.
-- The asset issuer and expected recipient are best effort for rows predating failure codes.
WITH src AS (
    SELECT p.id, p.verification_status, p.failure_code, p.amount, p.asset_code, p.asset_issuer,
           p.recipient, p.payment_reference,
           i.id AS inv_id, i.total, i.currency::text AS currency, i.status::text AS inv_status,
           i.payment_reference AS inv_ref,
           COALESCE(i.recipient_address,
                    (SELECT w.address FROM wallets w
                     WHERE w.user_id = i.created_by_id AND w.chain = 'stellar' AND w.is_primary AND w.is_active
                     LIMIT 1)) AS inv_recipient
    FROM payments p
    LEFT JOIN invoice i ON i.payment_reference = p.payment_reference
    WHERE p.checks IS NULL
)
UPDATE payments p SET
    checks = jsonb_build_object(
        'success',   CASE WHEN s.verification_status = 'failed' THEN 'fail' ELSE 'pass' END,
        'reference', CASE WHEN s.inv_id IS NULL THEN 'fail' ELSE 'pass' END,
        'recipient', CASE WHEN s.inv_id IS NULL THEN 'skip'
                          WHEN s.inv_recipient IS NOT NULL AND s.inv_recipient = s.recipient THEN 'pass' ELSE 'fail' END,
        'asset',     CASE WHEN s.inv_id IS NULL THEN 'skip'
                          WHEN upper(s.currency) = s.asset_code AND s.failure_code IS DISTINCT FROM 'wrong_asset' THEN 'pass'
                          ELSE 'fail' END,
        'amount',    CASE WHEN s.inv_id IS NULL THEN 'skip'
                          WHEN s.total = s.amount THEN 'pass' ELSE 'fail' END,
        'invoice',   CASE WHEN s.inv_id IS NULL THEN 'skip'
                          WHEN s.verification_status = 'verified' THEN 'pass'
                          WHEN s.failure_code IN ('duplicate', 'invoice_not_payable', 'expired') THEN 'fail'
                          ELSE 'pass' END),
    expected = CASE WHEN s.inv_id IS NULL THEN NULL ELSE jsonb_build_object(
        'amount', to_char(s.total, 'FM999999999990.0000000'),
        'asset_code', upper(s.currency),
        'asset_issuer', CASE WHEN upper(s.currency) = 'XLM' OR s.failure_code = 'wrong_asset' THEN NULL ELSE s.asset_issuer END,
        'recipient', COALESCE(s.inv_recipient, ''),
        'payment_reference', s.inv_ref) END
FROM src s WHERE p.id = s.id;

-- Test data: invoice wrongly paid by the old mark-paid behaviour, all-zero ledger hash, test notification.
DELETE FROM transactions
WHERE transaction_hash ~ '^0+$'
   OR invoice_id IN (
        SELECT i.id FROM invoice i
        WHERE i.invoice_number = 'INV-20261020-0019'
          AND NOT EXISTS (SELECT 1 FROM payments p WHERE p.invoice_id = i.id AND p.verification_status = 'verified'));

UPDATE invoice i SET status = 'cancelled', paid_at = NULL, payer_wallet_address = NULL, updated_at = NOW()
WHERE i.invoice_number = 'INV-20261020-0019' AND i.status = 'paid'
  AND NOT EXISTS (SELECT 1 FROM payments p WHERE p.invoice_id = i.id AND p.verification_status = 'verified');

DELETE FROM notifications WHERE body ILIKE '%Amount: 1.00 USDC%';

-- Blank account created through the unverified social sign-in; refuse to delete one that owns invoices.
DELETE FROM users u
WHERE u.id = 'c59c9335-779b-4ee3-826f-16249a59e2d7'
  AND u.first_name IS NULL AND u.last_name IS NULL
  AND NOT EXISTS (SELECT 1 FROM invoice i WHERE i.created_by_id = u.id);
