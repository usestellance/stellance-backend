package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/notifications"
	"github.com/The-True-Hooha/stellance-backend/mail"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stellar/go/amount"
)

const (
	maxPagesPerAccount = 5
	pollLockKey        = 7070001
	defaultGraceDays   = 0
	staleDetectedAfter = 30 * time.Second
)

var (
	ErrNoRecipientWallet   = errors.New("invoice creator has no active stellar wallet")
	ErrInvoiceNotPayable   = errors.New("invoice is not payable")
	ErrTransactionNotFound = errors.New("transaction not found on Stellar")
)

const (
	CodeWalletRequired     = "wallet_required"
	MsgOwnerWalletRequired = "You have not created a wallet yet. Create a Stellar wallet to receive payment for this invoice."
	MsgMerchantNoWallet    = "The merchant has not created a wallet yet, so this invoice cannot receive payment. Please contact the merchant."
	MsgSendWalletRequired  = "Create a Stellar wallet before sending invoices, otherwise nobody can pay them."
)

var ()

type PaymentService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	jwt      *jwt_.JwtTokenServiceConfig
	chain    func(ctx context.Context) (chainClient, string, error)
	now      func() time.Time
	onPaid   func(invoiceID, txHash string)
	onReview func(recipient, txHash string, status VerificationStatus, reason string)
}

func NewPaymentService() *PaymentService {
	s := &PaymentService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
		now:      time.Now,
	}
	s.chain = s.resolveNetwork
	s.onPaid = s.notifyPaid
	s.onReview = s.notifyReview
	return s
}

func graceDays() int {
	if v, err := strconv.Atoi(os.Getenv("PAYMENT_EXPIRY_GRACE_DAYS")); err == nil && v >= 0 {
		return v
	}
	return defaultGraceDays
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const invoiceViewSelect = `
	SELECT id, status::text,
	       COALESCE(recipient_address, (SELECT w.address FROM wallets w
	           WHERE w.user_id = invoice.created_by_id AND w.chain = 'stellar' AND w.is_primary AND w.is_active
	           LIMIT 1), ''),
	       payment_reference, currency::text,
	       total::text, ((due_date + $2::int + 1)::timestamp AT TIME ZONE 'UTC')
	FROM invoice `

// loadInvoice reads the invoice matched by a fixed where-clause: "payment_reference = $1" or "id = $1".
func loadInvoice(ctx context.Context, q querier, where string, key any, lock bool) (*invoiceView, error) {
	query := invoiceViewSelect + "WHERE " + where
	if lock {
		query += " FOR UPDATE"
	}

	var (
		v         invoiceView
		total     string
		expiresAt *time.Time
	)
	err := q.QueryRow(ctx, query, key, graceDays()).Scan(&v.ID, &v.Status, &v.Recipient, &v.Reference, &v.Currency, &total, &expiresAt)
	if err != nil {
		return nil, err
	}
	if v.Stroops, err = amount.ParseInt64(total); err != nil {
		return nil, fmt.Errorf("invoice %s total %q: %w", v.ID, total, err)
	}
	if expiresAt != nil {
		v.ExpiresAt = *expiresAt
	}
	return &v, nil
}

// processPayment records one incoming payment operation and settles it. It returns an error only for
// infrastructure failures, in which case the caller must not advance past this operation.
func (s *PaymentService) processPayment(ctx context.Context, p chainPayment, stage string) error {
	var inv *invoiceView
	if p.Memo != "" {
		v, err := loadInvoice(ctx, s.postgres, "payment_reference = $1", p.Memo, false)
		switch {
		case err == nil:
			inv = v
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("load invoice by reference: %w", err)
		}
	}

	expected := asset{}
	if inv != nil {
		expected = expectedAsset(stage, inv.Currency)
	}
	v := evaluate(p, inv, expected)

	if v.Status == StatusFailed || v.Status == StatusUnmatched {
		var invoiceID *string
		if v.Status == StatusFailed && inv != nil && inv.Recipient == p.To {
			invoiceID = &inv.ID
		}
		return s.insertSettled(ctx, p, invoiceID, v)
	}

	id, err := s.insertDetected(ctx, p, inv.ID)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	return s.finalize(ctx, id, stage)
}

func insertArgs(p chainPayment, invoiceID *string) []any {
	var issuer *string
	if p.Asset.Issuer != "" {
		issuer = &p.Asset.Issuer
	}
	var memo *string
	if p.Memo != "" {
		memo = &p.Memo
	}
	return []any{invoiceID, p.OperationID, p.TxHash, p.From, p.To, p.Asset.Code, issuer,
		amount.StringFromInt64(p.Stroops), memo, p.ClosedAt}
}

const insertPaymentCols = `invoice_id, operation_id, transaction_hash, sender, recipient, asset_code, asset_issuer,
	amount, payment_reference, ledger_closed_at`

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// recordRejectedTransaction puts an incoming payment that did not settle an invoice into the wallet
// owner's history. The ledger status is the on-chain result; the verification result stays on the payment.
func recordRejectedTransaction(ctx context.Context, e execer, p chainPayment, invoiceID *string, status VerificationStatus) error {
	txStatus := "confirmed"
	if status == StatusFailed {
		txStatus = "failed"
	}
	cur := strings.ToLower(p.Asset.Code)
	if cur != "usdc" && cur != "xlm" {
		return nil
	}
	_, err := e.Exec(ctx, `
		INSERT INTO transactions (invoice_id, wallet_id, transaction_hash, user_id, amount, currency, status,
		                          token_type, transaction_type, confirmed_at)
		SELECT $1, w.id, $2, w.user_id, $3::text::numeric, $4::currency_type, $5::transaction_status,
		       $4::currency_type, 'payment', $6
		FROM wallets w WHERE w.address = $7 AND w.chain = 'stellar'
		LIMIT 1
		ON CONFLICT (transaction_hash) DO NOTHING`,
		invoiceID, p.TxHash, amount.StringFromInt64(p.Stroops), cur, txStatus, p.ClosedAt, p.To)
	if err != nil {
		return fmt.Errorf("record %s transaction for payment %s: %w", status, p.OperationID, err)
	}
	return nil
}

func (s *PaymentService) insertSettled(ctx context.Context, p chainPayment, invoiceID *string, v verdict) error {
	tx, err := s.postgres.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		INSERT INTO payments (`+insertPaymentCols+`, verification_status, failure_reason, failure_code, checks, expected)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::text::numeric, $9, $10, $11::payment_verification_status, $12, $13, $14, $15)
		ON CONFLICT (operation_id) DO NOTHING`,
		append(insertArgs(p, invoiceID), string(v.Status), v.Reason, nullIfEmpty(v.Code), v.checksJSON(), v.expectedJSON())...)
	if err != nil {
		return fmt.Errorf("insert %s payment %s: %w", v.Status, p.OperationID, err)
	}
	if tag.RowsAffected() == 1 {
		if err = recordRejectedTransaction(ctx, tx, p, invoiceID, v.Status); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		s.review(p, v)
	}
	return nil
}

func (s *PaymentService) review(p chainPayment, v verdict) {
	if s.onReview != nil && (v.Status == StatusInvalid || v.Status == StatusUnmatched) {
		go s.onReview(p.To, p.TxHash, v.Status, v.Reason)
	}
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (v verdict) checksJSON() []byte {
	b, _ := json.Marshal(v.Checks)
	return b
}

func (v verdict) expectedJSON() []byte {
	if v.Expected == nil {
		return nil
	}
	b, _ := json.Marshal(v.Expected)
	return b
}

// insertDetected stores the payment as 'detected' and flags the invoice. It returns the payment id when
// the payment still needs settling: either newly inserted, or left unfinished by an earlier crash.
func (s *PaymentService) insertDetected(ctx context.Context, p chainPayment, invoiceID string) (string, error) {
	tx, err := s.postgres.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO payments (`+insertPaymentCols+`, verification_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::text::numeric, $9, $10, 'detected')
		ON CONFLICT (operation_id) DO NOTHING
		RETURNING id`, insertArgs(p, &invoiceID)...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			SELECT id FROM payments
			WHERE operation_id = $1 AND verification_status IN ('pending', 'detected')`, p.OperationID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("insert detected payment %s: %w", p.OperationID, err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE invoice SET status = 'payment_detected', updated_at = NOW()
		WHERE id = $1 AND status IN ('sent', 'viewed', 'overdue', 'pending', 'invalid_payment')`, invoiceID); err != nil {
		return "", fmt.Errorf("flag invoice %s as payment_detected: %w", invoiceID, err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	s.invalidateInvoiceCache(ctx, invoiceID)
	return id, nil
}

// finalize settles a detected payment against the current invoice state under row locks.
func (s *PaymentService) finalize(ctx context.Context, paymentID, stage string) error {
	tx, err := s.postgres.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		p         chainPayment
		invoiceID string
		stroops   string
		issuer    *string
		memo      *string
		status    string
	)
	err = tx.QueryRow(ctx, `
		SELECT invoice_id, operation_id, transaction_hash, sender, recipient, asset_code, asset_issuer,
		       amount::text, payment_reference, ledger_closed_at, verification_status::text
		FROM payments WHERE id = $1 FOR UPDATE`, paymentID).Scan(
		&invoiceID, &p.OperationID, &p.TxHash, &p.From, &p.To, &p.Asset.Code, &issuer,
		&stroops, &memo, &p.ClosedAt, &status)
	if err != nil {
		return fmt.Errorf("lock payment %s: %w", paymentID, err)
	}
	if status != string(StatusDetected) && status != string(StatusPending) {
		return nil
	}
	if issuer != nil {
		p.Asset.Issuer = *issuer
	}
	if memo != nil {
		p.Memo = *memo
	}
	p.Successful = true
	if p.Stroops, err = amount.ParseInt64(stroops); err != nil {
		return fmt.Errorf("payment %s amount %q: %w", paymentID, stroops, err)
	}

	inv, err := loadInvoice(ctx, tx, "id = $1", invoiceID, true)
	if err != nil {
		return fmt.Errorf("lock invoice %s: %w", invoiceID, err)
	}
	v := evaluate(p, inv, expectedAsset(stage, inv.Currency))

	if v.Status == StatusVerified {
		tag, err := tx.Exec(ctx, `
			UPDATE invoice
			SET status = 'paid', paid_at = NOW(), payer_wallet_address = $2, updated_at = NOW()
			WHERE id = $1 AND status IN ('sent', 'viewed', 'overdue', 'pending', 'payment_detected', 'invalid_payment')`,
			invoiceID, p.From)
		if err != nil {
			return fmt.Errorf("mark invoice %s paid: %w", invoiceID, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("invoice %s left a payable state while locked", invoiceID)
		}
		if _, err = tx.Exec(ctx, `
			UPDATE payments SET verification_status = 'verified', verified_at = NOW(), checks = $2, expected = $3 WHERE id = $1`,
			paymentID, v.checksJSON(), v.expectedJSON()); err != nil {
			return fmt.Errorf("verify payment %s: %w", paymentID, err)
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO transactions (invoice_id, wallet_id, transaction_hash, user_id, amount, currency, status,
			                          token_type, transaction_type, confirmed_at)
			SELECT i.id, (SELECT w.id FROM wallets w WHERE w.address = $2 AND w.chain = 'stellar'), $3,
			       i.created_by_id, $4::text::numeric, i.currency, 'confirmed', i.currency, 'payment', $5
			FROM invoice i WHERE i.id = $1
			ON CONFLICT (transaction_hash) DO NOTHING`,
			invoiceID, p.To, p.TxHash, stroops, p.ClosedAt); err != nil {
			return fmt.Errorf("record transaction for payment %s: %w", paymentID, err)
		}
	} else {
		if _, err = tx.Exec(ctx, `
			UPDATE payments SET verification_status = $2::payment_verification_status, failure_reason = $3,
			       failure_code = $4, checks = $5, expected = $6
			WHERE id = $1`, paymentID, string(v.Status), v.Reason, nullIfEmpty(v.Code), v.checksJSON(), v.expectedJSON()); err != nil {
			return fmt.Errorf("reject payment %s: %w", paymentID, err)
		}
		if err = recordRejectedTransaction(ctx, tx, p, &invoiceID, v.Status); err != nil {
			return err
		}
		if v.Status == StatusInvalid {
			if _, err = tx.Exec(ctx, `
				UPDATE invoice SET status = 'invalid_payment', updated_at = NOW()
				WHERE id = $1 AND status = 'payment_detected'`, invoiceID); err != nil {
				return fmt.Errorf("flag invoice %s invalid_payment: %w", invoiceID, err)
			}
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.invalidateInvoiceCache(ctx, invoiceID)
	if v.Status == StatusVerified {
		go s.onPaid(invoiceID, p.TxHash)
	} else {
		s.review(p, v)
	}
	return nil
}

func (s *PaymentService) invalidateInvoiceCache(ctx context.Context, invoiceID string) {
	if s.redis == nil {
		return
	}
	if err := s.redis.Del(ctx, "invoice:"+invoiceID).Err(); err != nil {
		s.log.Warn("failed to clear invoice cache", "invoice_id", invoiceID, "error", err)
	}
}

func (s *PaymentService) notifyReview(recipient, txHash string, status VerificationStatus, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var userID string
	if err := s.postgres.QueryRow(ctx, `SELECT user_id FROM wallets WHERE address = $1 AND chain = 'stellar' LIMIT 1`, recipient).Scan(&userID); err != nil {
		return
	}
	title := "Payment Needs Review"
	if status == StatusUnmatched {
		title = "Unmatched Payment Received"
	}
	notifications.NewNotificationService().CreateNewNotification(ctx, notifications.CreateNotificationDto{
		Title:  title,
		UserId: userID,
		Body:   fmt.Sprintf("A payment could not be matched to an invoice (%s). Tx: %s", reason, txHash),
	})
}

func (s *PaymentService) notifyPaid(invoiceID, txHash string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var creatorID, creatorEmail, creatorName, payerEmail, number, total, currency string
	err := s.postgres.QueryRow(ctx, `
		SELECT i.created_by_id, u.email, COALESCE(u.first_name, ''), COALESCE(i.payer_email, ''),
		       i.invoice_number, i.total::text, i.currency::text
		FROM invoice i JOIN users u ON u.id = i.created_by_id WHERE i.id = $1`, invoiceID).
		Scan(&creatorID, &creatorEmail, &creatorName, &payerEmail, &number, &total, &currency)
	if err != nil {
		s.log.Warn("failed to load invoice for payment notification", "invoice_id", invoiceID, "error", err)
		return
	}

	notifications.NewNotificationService().CreateNewNotification(ctx, notifications.CreateNotificationDto{
		Title:  "Invoice Paid",
		UserId: creatorID,
		Body:   fmt.Sprintf("Payment for invoice %s was verified on Stellar. Tx: %s", number, txHash),
	})
	if err := mail.NewMailer().SendPaymentConfirmedEmail(mail.PaymentConfirmedEmailData{
		CreatorEmail:  creatorEmail,
		CreatorName:   creatorName,
		PayerEmail:    payerEmail,
		InvoiceNumber: number,
		Total:         total,
		Currency:      currency,
		TxHash:        txHash,
		DashboardURL:  "https://usestellance.com/dashboard",
	}); err != nil {
		s.log.Warn("failed to send payment confirmed email", "invoice_id", invoiceID, "error", err)
	}
}

// Poll scans every account with an open invoice or an invoice touched in the last 7 days, so late
// duplicate payments to a just-paid invoice are still recorded. One failing account never blocks the others.
func (s *PaymentService) Poll(ctx context.Context) error {
	conn, err := s.postgres.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, pollLockKey).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, pollLockKey)

	chain, stage, err := s.chain(ctx)
	if err != nil {
		return err
	}

	rows, err := conn.Query(ctx, `
		SELECT DISTINCT COALESCE(i.recipient_address, w.address)
		FROM invoice i
		LEFT JOIN wallets w ON w.user_id = i.created_by_id AND w.chain = 'stellar' AND w.is_primary AND w.is_active
		WHERE COALESCE(i.recipient_address, w.address) IS NOT NULL
		  AND (i.status IN ('sent', 'viewed', 'overdue', 'pending', 'payment_detected', 'invalid_payment')
		       OR i.updated_at > NOW() - INTERVAL '7 days')`)
	if err != nil {
		return err
	}
	accounts, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}

	for _, account := range accounts {
		if err := s.pollAccount(ctx, chain, stage, account); err != nil {
			s.log.Error("payment poll failed for account", "account", account, "error", err)
		}
	}
	return nil
}

func (s *PaymentService) pollAccount(ctx context.Context, chain chainClient, stage, account string) error {
	var cursor string
	err := s.postgres.QueryRow(ctx, `SELECT cursor FROM payment_cursors WHERE network = $1 AND account = $2`, stage, account).Scan(&cursor)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	for range maxPagesPerAccount {
		page, err := chain.AccountPayments(ctx, account, cursor)
		if err != nil {
			return err
		}
		for _, p := range page.Payments {
			if err := s.processPayment(ctx, p, stage); err != nil {
				return err
			}
		}
		if page.NextCursor != cursor {
			cursor = page.NextCursor
			if _, err := s.postgres.Exec(ctx, `
				INSERT INTO payment_cursors (network, account, cursor) VALUES ($1, $2, $3)
				ON CONFLICT (network, account) DO UPDATE SET cursor = EXCLUDED.cursor, updated_at = NOW()`,
				stage, account, cursor); err != nil {
				return err
			}
		}
		if page.Fetched < pageLimit {
			return nil
		}
	}
	return nil
}

// RecoverUnfinished settles payments left in 'detected' by a crash between the two settle phases.
func (s *PaymentService) RecoverUnfinished(ctx context.Context) error {
	_, stage, err := s.chain(ctx)
	if err != nil {
		return err
	}
	rows, err := s.postgres.Query(ctx, `
		SELECT id FROM payments
		WHERE verification_status IN ('pending', 'detected') AND detected_at < $1
		ORDER BY detected_at LIMIT 100`, s.now().Add(-staleDetectedAfter))
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.finalize(ctx, id, stage); err != nil {
			s.log.Error("failed to recover detected payment", "payment_id", id, "error", err)
		}
	}
	return nil
}

// ExpireInvoices closes the payment window of unpaid invoices that are past due by the grace period.
func (s *PaymentService) ExpireInvoices(ctx context.Context) error {
	tag, err := s.postgres.Exec(ctx, `
		UPDATE invoice SET status = 'expired', updated_at = NOW()
		WHERE status IN ('sent', 'viewed', 'overdue', 'invalid_payment')
		  AND due_date IS NOT NULL AND due_date + $1::int < (NOW() AT TIME ZONE 'UTC')::date`, graceDays())
	if err != nil {
		return err
	}
	if n := tag.RowsAffected(); n > 0 {
		s.log.Info("expired unpaid invoices", "count", n)
	}
	return nil
}

// VerifyTransaction fetches a transaction by hash and settles every payment in it that targets a
// recipient Stellance knows. It is safe to call repeatedly.
func (s *PaymentService) VerifyTransaction(ctx context.Context, hash string) ([]PaymentRecord, error) {
	chain, stage, err := s.chain(ctx)
	if err != nil {
		return nil, err
	}
	payments, err := chain.TransactionPayments(ctx, hash)
	if err != nil {
		return nil, err
	}

	for _, p := range payments {
		var known bool
		if err := s.postgres.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM invoice WHERE recipient_address = $1)
			    OR EXISTS (SELECT 1 FROM wallets WHERE address = $1)`, p.To).Scan(&known); err != nil {
			return nil, err
		}
		if !known {
			continue
		}
		if err := s.processPayment(ctx, p, stage); err != nil {
			return nil, err
		}
	}
	return s.paymentsByHash(ctx, hash, stage)
}

// EnsurePaymentRequest fixes the invoice recipient to the creator's primary wallet on first use.
func (s *PaymentService) EnsurePaymentRequest(ctx context.Context, invoiceID string) (*PaymentRequest, error) {
	if _, err := s.postgres.Exec(ctx, `
		UPDATE invoice i SET recipient_address = w.address, updated_at = NOW()
		FROM wallets w
		WHERE i.id = $1 AND i.recipient_address IS NULL
		  AND w.user_id = i.created_by_id AND w.chain = 'stellar' AND w.is_primary AND w.is_active
		  AND i.status IN ('sent', 'viewed', 'overdue', 'pending', 'payment_detected', 'invalid_payment')`, invoiceID); err != nil {
		return nil, err
	}

	req, err := s.paymentRequest(ctx, "i.id = $1", invoiceID)
	if err != nil {
		return nil, err
	}
	if !payableInvoiceStatuses[req.InvoiceStatus] {
		return nil, ErrInvoiceNotPayable
	}
	if req.Recipient == "" {
		return nil, ErrNoRecipientWallet
	}
	return req, nil
}
