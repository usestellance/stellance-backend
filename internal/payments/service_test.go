package payments

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests: need a migrated Postgres. Run with TEST_DATABASE_URL set, otherwise skipped.

func addr(name string) string { return (name + strings.Repeat("A", 56))[:56] }

var vendorAddr = addr("GVENDOR")

type fakeChain struct {
	mu       sync.Mutex
	accounts map[string][]chainPayment
	failFor  map[string]bool
}

func (f *fakeChain) AccountPayments(_ context.Context, account, cursor string) (chainPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failFor[account] {
		return chainPage{}, errors.New("horizon unavailable")
	}
	all := f.accounts[account]
	start, _ := strconv.Atoi(cursor)
	if start > len(all) {
		start = len(all)
	}
	return chainPage{Payments: all[start:], NextCursor: strconv.Itoa(len(all)), Fetched: len(all) - start}, nil
}

func (f *fakeChain) TransactionPayments(_ context.Context, hash string) ([]chainPayment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []chainPayment
	for _, ps := range f.accounts {
		for _, p := range ps {
			if p.TxHash == hash {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

type fixture struct {
	t     *testing.T
	db    *pgxpool.Pool
	svc   *PaymentService
	chain *fakeChain
	user  string
	paid  chan string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	if _, err := db.Exec(ctx, `TRUNCATE payments, payment_cursors, transactions, invoice, wallets, users CASCADE`); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, db: db, chain: &fakeChain{accounts: map[string][]chainPayment{}, failFor: map[string]bool{}}, paid: make(chan string, 16)}
	f.svc = &PaymentService{
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		postgres: db,
		now:      func() time.Time { return time.Now() },
		onPaid:   func(id, _ string) { f.paid <- id },
	}
	f.svc.chain = func(context.Context) (chainClient, string, error) { return f.chain, "testnet", nil }

	if err := db.QueryRow(ctx, `INSERT INTO users (email) VALUES ('vendor@example.com') RETURNING id`).Scan(&f.user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO wallets (user_id, address, private_key, chain, currency, is_primary)
		VALUES ($1, $2, 'x', 'stellar', 'usdc', TRUE)`, f.user, vendorAddr); err != nil {
		t.Fatal(err)
	}
	return f
}

var invoiceSeq int

func (f *fixture) invoice(status, total string, recipient *string) (id, ref string) {
	f.t.Helper()
	invoiceSeq++
	n := strconv.Itoa(invoiceSeq)
	err := f.db.QueryRow(context.Background(), `
		INSERT INTO invoice (invoice_number, invoice_url, created_by_id, sub_total, total, status, payer_name,
		                     payer_email, recipient_address, due_date)
		VALUES ('INV-'||$1, 'url-'||$1, $2, $3::numeric, $3::numeric, $4::invoice_status, 'Payer', 'payer@example.com', $5,
		        CURRENT_DATE + 7)
		RETURNING id, payment_reference`, n, f.user, total, status, recipient).Scan(&id, &ref)
	if err != nil {
		f.t.Fatal(err)
	}
	return id, ref
}

func (f *fixture) vendorInvoice(total string) (string, string) {
	r := vendorAddr
	return f.invoice("sent", total, &r)
}

var opSeq int

func (f *fixture) send(account string, p chainPayment) chainPayment {
	opSeq++
	if p.OperationID == "" {
		p.OperationID = strconv.Itoa(1000 + opSeq)
	}
	if p.TxHash == "" {
		p.TxHash = "hash-" + p.OperationID
	}
	if p.From == "" {
		p.From = "GPAYER"
	}
	if p.To == "" {
		p.To = account
	}
	if p.Asset == (asset{}) {
		p.Asset = asset{Code: "USDC", Issuer: expectedAsset("testnet", "usdc").Issuer}
	}
	p.ClosedAt = time.Now()
	f.chain.accounts[account] = append(f.chain.accounts[account], p)
	return p
}

func (f *fixture) poll() {
	f.t.Helper()
	if err := f.svc.Poll(context.Background()); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) invoiceStatus(id string) string {
	var s string
	if err := f.db.QueryRow(context.Background(), `SELECT status::text FROM invoice WHERE id = $1`, id).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *fixture) paymentStatus(op string) (status string, invoiceID *string) {
	err := f.db.QueryRow(context.Background(),
		`SELECT verification_status::text, invoice_id::text FROM payments WHERE operation_id = $1`, op).Scan(&status, &invoiceID)
	if err != nil {
		f.t.Fatalf("payment for op %s: %v", op, err)
	}
	return
}

func (f *fixture) count(query string, args ...any) int {
	var n int
	if err := f.db.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func TestCorrectPaymentMarksInvoicePaid(t *testing.T) {
	f := newFixture(t)
	id, ref := f.vendorInvoice("100")
	p := f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})

	f.poll()

	if got := f.invoiceStatus(id); got != "paid" {
		t.Fatalf("invoice status = %s, want paid", got)
	}
	if st, inv := f.paymentStatus(p.OperationID); st != "verified" || inv == nil || *inv != id {
		t.Fatalf("payment = %s/%v, want verified for invoice", st, inv)
	}
	if n := f.count(`SELECT COUNT(*) FROM transactions WHERE invoice_id = $1 AND transaction_hash = $2 AND status = 'confirmed'`, id, p.TxHash); n != 1 {
		t.Fatalf("transaction rows = %d, want 1", n)
	}
	var payer *string
	f.db.QueryRow(context.Background(), `SELECT payer_wallet_address FROM invoice WHERE id = $1`, id).Scan(&payer)
	if payer == nil || *payer != "GPAYER" {
		t.Fatalf("payer_wallet_address = %v", payer)
	}
	select {
	case got := <-f.paid:
		if got != id {
			t.Fatalf("notified invoice %s, want %s", got, id)
		}
	case <-time.After(time.Second):
		t.Fatal("no paid notification")
	}
}

func TestRejectedPaymentsNeverPayInvoice(t *testing.T) {
	tests := []struct {
		name       string
		payment    func(ref string) chainPayment
		wantPay    VerificationStatus
		wantInv    string
		wantLinked bool
		wantCode   string
	}{
		{"insufficient", func(ref string) chainPayment {
			return chainPayment{Stroops: 99_9999999, Memo: ref, Successful: true}
		}, StatusInvalid, "invalid_payment", true, CodeInsufficientAmount},
		{"wrong asset", func(ref string) chainPayment {
			return chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true, Asset: asset{Code: "USDC", Issuer: addr("GFAKE")}}
		}, StatusInvalid, "invalid_payment", true, CodeWrongAsset},
		{"wrong memo", func(ref string) chainPayment {
			return chainPayment{Stroops: 100_0000000, Memo: "STL-NOPE", Successful: true}
		}, StatusUnmatched, "sent", false, CodeUnknownReference},
		{"no memo", func(ref string) chainPayment {
			return chainPayment{Stroops: 100_0000000, Successful: true}
		}, StatusUnmatched, "sent", false, CodeMissingReference},
		{"failed transaction", func(ref string) chainPayment {
			return chainPayment{Stroops: 100_0000000, Memo: ref, Successful: false}
		}, StatusFailed, "sent", true, CodeTransactionFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			id, ref := f.vendorInvoice("100")
			p := f.send(vendorAddr, tc.payment(ref))

			f.poll()

			if got := f.invoiceStatus(id); got != tc.wantInv {
				t.Fatalf("invoice status = %s, want %s", got, tc.wantInv)
			}
			st, inv := f.paymentStatus(p.OperationID)
			if VerificationStatus(st) != tc.wantPay {
				t.Fatalf("payment status = %s, want %s", st, tc.wantPay)
			}
			if (inv != nil) != tc.wantLinked {
				t.Fatalf("payment linked to invoice = %v, want %v", inv != nil, tc.wantLinked)
			}
			wantLedger := "confirmed"
			if tc.wantPay == StatusFailed {
				wantLedger = "failed"
			}
			if n := f.count(`SELECT COUNT(*) FROM transactions WHERE status = $1 AND transaction_type = 'payment'`, wantLedger); n != 1 {
				t.Fatalf("history rows with ledger status %s = %d, want 1", wantLedger, n)
			}
			var code string
			if err := f.db.QueryRow(context.Background(),
				`SELECT COALESCE(failure_code, '') FROM payments WHERE operation_id = $1`, p.OperationID).Scan(&code); err != nil {
				t.Fatal(err)
			}
			if code != tc.wantCode {
				t.Fatalf("failure_code = %q, want %q", code, tc.wantCode)
			}
		})
	}
}

func TestPaymentAfterInvalidAttemptStillPays(t *testing.T) {
	f := newFixture(t)
	id, ref := f.vendorInvoice("100")
	f.send(vendorAddr, chainPayment{Stroops: 50_0000000, Memo: ref, Successful: true})
	f.poll()
	if got := f.invoiceStatus(id); got != "invalid_payment" {
		t.Fatalf("after short payment status = %s", got)
	}

	f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})
	f.poll()
	if got := f.invoiceStatus(id); got != "paid" {
		t.Fatalf("after correct payment status = %s, want paid", got)
	}
}

func TestWrongRecipientIsNotAssociated(t *testing.T) {
	f := newFixture(t)
	id, ref := f.vendorInvoice("100")
	other := addr("GOTHER")
	f.invoice("sent", "5", &other)
	p := f.send(other, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})

	f.poll()

	st, inv := f.paymentStatus(p.OperationID)
	if st != "unmatched" || inv != nil {
		t.Fatalf("payment = %s/%v, want unmatched and unlinked", st, inv)
	}
	if got := f.invoiceStatus(id); got != "sent" {
		t.Fatalf("invoice status = %s, want sent", got)
	}
}

func TestDuplicateProcessingDoesNotRepeatEffects(t *testing.T) {
	f := newFixture(t)
	id, ref := f.vendorInvoice("100")
	p := f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})
	f.poll()

	// same operation replayed by a cursor reset and by an explicit re-verification
	f.db.Exec(context.Background(), `DELETE FROM payment_cursors`)
	other := vendorAddr
	f.invoice("sent", "1", &other)
	f.poll()
	if _, err := f.svc.VerifyTransaction(context.Background(), p.TxHash); err != nil {
		t.Fatal(err)
	}

	if n := f.count(`SELECT COUNT(*) FROM payments WHERE operation_id = $1`, p.OperationID); n != 1 {
		t.Fatalf("payment rows = %d, want 1", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM transactions WHERE invoice_id = $1`, id); n != 1 {
		t.Fatalf("transaction rows = %d, want 1", n)
	}
	if len(f.paid) != 1 {
		t.Fatalf("paid notifications = %d, want 1", len(f.paid))
	}
}

func TestSecondPaymentForPaidInvoiceIsInvalid(t *testing.T) {
	f := newFixture(t)
	id, ref := f.vendorInvoice("100")
	f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})
	f.poll()
	second := f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})
	f.poll()

	if st, _ := f.paymentStatus(second.OperationID); st != "invalid" {
		t.Fatalf("second payment = %s, want invalid", st)
	}
	if got := f.invoiceStatus(id); got != "paid" {
		t.Fatalf("invoice status = %s, want paid", got)
	}
	if n := f.count(`SELECT COUNT(*) FROM payments WHERE invoice_id = $1 AND verification_status = 'verified'`, id); n != 1 {
		t.Fatalf("verified payments = %d, want 1", n)
	}
}

func TestConcurrentPaymentsSettleOnce(t *testing.T) {
	f := newFixture(t)
	id, ref := f.vendorInvoice("100")
	var ps []chainPayment
	for range 4 {
		ps = append(ps, f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true}))
	}

	var wg sync.WaitGroup
	for _, p := range ps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.svc.processPayment(context.Background(), p, "testnet"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if n := f.count(`SELECT COUNT(*) FROM payments WHERE invoice_id = $1 AND verification_status = 'verified'`, id); n != 1 {
		t.Fatalf("verified payments = %d, want 1", n)
	}
	if got := f.invoiceStatus(id); got != "paid" {
		t.Fatalf("invoice status = %s, want paid", got)
	}
	if n := f.count(`SELECT COUNT(*) FROM payments WHERE invoice_id = $1 AND verification_status = 'invalid'`, id); n != 3 {
		t.Fatalf("invalid payments = %d, want 3", n)
	}
}

func TestPollIsolatesFailingAccountAndRecovers(t *testing.T) {
	f := newFixture(t)
	broken := addr("GBROKEN")
	f.invoice("sent", "5", &broken)
	id, ref := f.vendorInvoice("100")
	f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})
	f.chain.failFor[broken] = true

	f.poll()
	if got := f.invoiceStatus(id); got != "paid" {
		t.Fatalf("healthy account blocked by failing one: status = %s", got)
	}
	if n := f.count(`SELECT COUNT(*) FROM payment_cursors WHERE account = $1`, broken); n != 0 {
		t.Fatal("cursor advanced for failing account")
	}

	id2, ref2 := f.invoice("sent", "7", &broken)
	f.send(broken, chainPayment{Stroops: 7_0000000, Memo: ref2, Successful: true})
	f.chain.failFor[broken] = false
	f.poll()
	if got := f.invoiceStatus(id2); got != "paid" {
		t.Fatalf("delayed payment not picked up after recovery: status = %s", got)
	}
}

func TestRecoverUnfinishedSettlesStuckDetectedPayment(t *testing.T) {
	f := newFixture(t)
	id, ref := f.vendorInvoice("100")
	p := f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})

	if _, err := f.svc.insertDetected(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	if got := f.invoiceStatus(id); got != "payment_detected" {
		t.Fatalf("status after detection = %s", got)
	}
	f.db.Exec(context.Background(), `UPDATE payments SET detected_at = NOW() - INTERVAL '5 minutes'`)

	if err := f.svc.RecoverUnfinished(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.invoiceStatus(id); got != "paid" {
		t.Fatalf("status after recovery = %s, want paid", got)
	}
}

func TestVerifyTransactionIgnoresUnknownRecipients(t *testing.T) {
	f := newFixture(t)
	stranger := addr("GSTRANGER")
	p := f.send(stranger, chainPayment{Stroops: 1_0000000, Memo: "STL-X", Successful: true})

	if _, err := f.svc.VerifyTransaction(context.Background(), p.TxHash); err != nil {
		t.Fatal(err)
	}
	if n := f.count(`SELECT COUNT(*) FROM payments`); n != 0 {
		t.Fatalf("payments rows = %d, want 0", n)
	}
}

func TestExpireInvoices(t *testing.T) {
	f := newFixture(t)
	old, _ := f.vendorInvoice("100")
	fresh, _ := f.vendorInvoice("100")
	detected, _ := f.vendorInvoice("100")
	f.db.Exec(context.Background(), `UPDATE invoice SET due_date = CURRENT_DATE - 90 WHERE id IN ($1, $2)`, old, detected)
	f.db.Exec(context.Background(), `UPDATE invoice SET status = 'payment_detected' WHERE id = $1`, detected)

	if err := f.svc.ExpireInvoices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.invoiceStatus(old); got != "expired" {
		t.Fatalf("old invoice = %s, want expired", got)
	}
	if got := f.invoiceStatus(fresh); got != "sent" {
		t.Fatalf("fresh invoice = %s, want sent", got)
	}
	if got := f.invoiceStatus(detected); got != "payment_detected" {
		t.Fatalf("in-flight invoice = %s, want payment_detected", got)
	}
}

func TestEnsurePaymentRequest(t *testing.T) {
	f := newFixture(t)
	id, ref := f.invoice("sent", "42.5", nil)

	req, err := f.svc.EnsurePaymentRequest(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if req.Recipient != vendorAddr || req.PaymentReference != ref || req.AssetCode != "USDC" || req.PaymentStatus != "awaiting_payment" {
		t.Fatalf("unexpected request %+v", req)
	}

	draft, _ := f.invoice("draft", "1", nil)
	if _, err := f.svc.EnsurePaymentRequest(context.Background(), draft); !errors.Is(err, ErrInvoiceNotPayable) {
		t.Fatalf("draft err = %v, want ErrInvoiceNotPayable", err)
	}
}

func TestPaymentToClosedInvoiceWithoutRecipientIsInvalidNotUnmatched(t *testing.T) {
	for status, wantReason := range map[string]string{
		"paid":      "invoice is already paid",
		"cancelled": "invoice is cancelled and cannot accept payment",
	} {
		t.Run(status, func(t *testing.T) {
			f := newFixture(t)
			id, ref := f.invoice("sent", "100", nil)
			if _, err := f.db.Exec(context.Background(),
				`UPDATE invoice SET status = $2::invoice_status, paid_at = CASE WHEN $2 = 'paid' THEN NOW() END WHERE id = $1`,
				id, status); err != nil {
				t.Fatal(err)
			}
			p := f.send(vendorAddr, chainPayment{Stroops: 100_0000000, Memo: ref, Successful: true})
			f.poll()

			st, _ := f.paymentStatus(p.OperationID)
			if st != "invalid" {
				t.Fatalf("payment status = %s, want invalid", st)
			}
			var reason string
			if err := f.db.QueryRow(context.Background(),
				`SELECT failure_reason FROM payments WHERE operation_id = $1`, p.OperationID).Scan(&reason); err != nil {
				t.Fatal(err)
			}
			if reason != wantReason {
				t.Fatalf("failure_reason = %q, want %q", reason, wantReason)
			}
		})
	}
}

func TestPaymentRecordCarriesChecksAndIsScopedToMerchant(t *testing.T) {
	f := newFixture(t)
	_, ref := f.vendorInvoice("100")
	p := f.send(vendorAddr, chainPayment{Stroops: 99_0000000, Memo: ref, Successful: true})
	f.poll()

	var id string
	if err := f.db.QueryRow(context.Background(), `SELECT id FROM payments WHERE operation_id = $1`, p.OperationID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	resp := f.svc.GetPayment(context.Background(), f.user, id)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	rec := resp.Data.(PaymentRecord)
	if rec.Amount != "99.0000000" || rec.AssetCode != "USDC" || rec.AssetIssuer == nil || rec.InvoiceNumber == nil {
		t.Fatalf("record = %+v", rec)
	}
	if rec.Checks == nil || rec.Checks.Amount != CheckFail || rec.Checks.Recipient != CheckPass || rec.Checks.Reference != CheckPass || rec.Checks.Success != CheckPass {
		t.Fatalf("checks = %+v", rec.Checks)
	}
	if rec.Expected == nil || rec.Expected.Amount != "100.0000000" || rec.Expected.Recipient != vendorAddr || rec.Expected.PaymentReference != ref {
		t.Fatalf("expected = %+v", rec.Expected)
	}
	if rec.FailureCode == nil || *rec.FailureCode != CodeInsufficientAmount {
		t.Fatalf("failure_code = %v", rec.FailureCode)
	}

	var other string
	if err := f.db.QueryRow(context.Background(), `INSERT INTO users (email) VALUES ('other@example.com') RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if got := f.svc.GetPayment(context.Background(), other, id).StatusCode; got != 404 {
		t.Fatalf("other merchant status = %d, want 404", got)
	}

	list := f.svc.ListPayments(context.Background(), f.user, "", 1, 500).Data.(PaymentList)
	if list.Meta.Total != 1 || list.Meta.Limit != 20 || len(list.Payments) != 1 {
		t.Fatalf("list = %+v", list.Meta)
	}
	admin := f.svc.AdminListPayments(context.Background(), "", "", 1, 20).Data.(PaymentList)
	if admin.Payments[0].MerchantEmail == nil || *admin.Payments[0].MerchantEmail != "vendor@example.com" {
		t.Fatalf("merchant email = %v", admin.Payments[0].MerchantEmail)
	}
}

func TestDuplicateToPaidInvoiceIsInvalidEvenWithWrongRecipient(t *testing.T) {
	inv := &invoiceView{ID: "i", Status: invoicePaid, Recipient: "GA", Reference: "STL-X", Stroops: 5}
	p := chainPayment{Successful: true, Memo: "STL-X", To: "GB", Stroops: 5, Asset: asset{Code: "XLM"}}
	v := evaluate(p, inv, asset{Code: "XLM"})
	if v.Status != StatusInvalid || v.Code != CodeDuplicate {
		t.Fatalf("verdict = %+v", v)
	}
	if v.Checks.Recipient != CheckFail || v.Checks.Invoice != CheckFail || v.Checks.Reference != CheckPass {
		t.Fatalf("checks = %+v", v.Checks)
	}
}

func TestInvoiceWithoutMerchantWalletReportsWalletRequired(t *testing.T) {
	f := newFixture(t)
	if _, err := f.db.Exec(context.Background(), `DELETE FROM wallets`); err != nil {
		t.Fatal(err)
	}
	id, _ := f.invoice("sent", "100", nil)
	var url string
	if err := f.db.QueryRow(context.Background(), `SELECT invoice_url FROM invoice WHERE id = $1`, id).Scan(&url); err != nil {
		t.Fatal(err)
	}

	owner := f.svc.GetInvoicePayments(context.Background(), f.user, id)
	if owner.StatusCode != 409 || owner.Message != MsgOwnerWalletRequired || owner.Error != CodeWalletRequired {
		t.Fatalf("owner response = %+v", owner)
	}
	public := f.svc.GetPublicRequest(context.Background(), url)
	if public.StatusCode != 409 || public.Message != MsgMerchantNoWallet || public.Error != CodeWalletRequired {
		t.Fatalf("public response = %+v", public)
	}
}

func TestDepositToWalletWithoutInvoiceReachesLedger(t *testing.T) {
	f := newFixture(t)
	p := f.send(vendorAddr, chainPayment{Stroops: 7_0000000, Successful: true})
	f.poll()

	if st, _ := f.paymentStatus(p.OperationID); st != "unmatched" {
		t.Fatalf("payment status = %s, want unmatched", st)
	}
	if n := f.count(`SELECT COUNT(*) FROM transactions WHERE transaction_hash = $1 AND user_id = $2 AND wallet_id IS NOT NULL`, p.TxHash, f.user); n != 1 {
		t.Fatalf("ledger rows for the deposit = %d, want 1", n)
	}
}
