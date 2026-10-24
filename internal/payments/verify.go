package payments

import (
	"fmt"
	"time"

	"github.com/stellar/go/amount"
)

type VerificationStatus string

const (
	StatusPending   VerificationStatus = "pending"
	StatusDetected  VerificationStatus = "detected"
	StatusVerified  VerificationStatus = "verified"
	StatusInvalid   VerificationStatus = "invalid"
	StatusUnmatched VerificationStatus = "unmatched"
	StatusFailed    VerificationStatus = "failed"
)

const (
	invoiceDetected = "payment_detected"
	invoiceInvalid  = "invalid_payment"
	invoicePaid     = "paid"
)

// payableInvoiceStatuses are the invoice states in which an incoming payment may complete the invoice.
var payableInvoiceStatuses = map[string]bool{
	"sent":          true,
	"viewed":        true,
	"overdue":       true,
	"pending":       true,
	invoiceDetected: true,
	invoiceInvalid:  true,
}

type asset struct {
	Code   string
	Issuer string
}

func (a asset) String() string {
	if a.Issuer == "" {
		return a.Code
	}
	return a.Code + ":" + a.Issuer
}

// chainPayment is one incoming payment operation, amounts in stroops (1e-7).
type chainPayment struct {
	OperationID string
	TxHash      string
	From        string
	To          string
	Asset       asset
	Stroops     int64
	Memo        string
	Successful  bool
	ClosedAt    time.Time
}

type invoiceView struct {
	ID        string
	Status    string
	Recipient string
	Reference string
	Currency  string
	Stroops   int64
	// ExpiresAt is exclusive: a payment closed at or after it is late. Judged on ledger close time, not processing time.
	ExpiresAt time.Time
}

const (
	CheckPass = "pass"
	CheckFail = "fail"
	CheckSkip = "skip"
)

const (
	CodeInsufficientAmount = "insufficient_amount"
	CodeAmountMismatch     = "amount_mismatch"
	CodeWrongAsset         = "wrong_asset"
	CodeWrongRecipient     = "wrong_recipient"
	CodeMissingReference   = "missing_reference"
	CodeUnknownReference   = "unknown_reference"
	CodeTransactionFailed  = "transaction_failed"
	CodeDuplicate          = "duplicate"
	CodeInvoiceNotPayable  = "invoice_not_payable"
	CodeExpired            = "expired"
	CodeInvalidInvoice     = "invalid_invoice_total"
)

// Expected is what the invoice demanded, in the same units as the payment record.
type Expected struct {
	Amount           string  `json:"amount" example:"100.0000000"`                                                    // Exact amount, 7 decimals
	AssetCode        string  `json:"asset_code" example:"USDC"`                                                       // Asset code
	AssetIssuer      *string `json:"asset_issuer" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Issuer; null for XLM
	Recipient        string  `json:"recipient" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"`    // Required recipient
	PaymentReference string  `json:"payment_reference" example:"STL-B9371BD3A173"`                                    // Required memo
}

type Checks struct {
	Recipient string `json:"recipient" enums:"pass,fail,skip"`
	Asset     string `json:"asset" enums:"pass,fail,skip"`
	Amount    string `json:"amount" enums:"pass,fail,skip"`
	Reference string `json:"reference" enums:"pass,fail,skip"`
	Success   string `json:"success" enums:"pass,fail,skip"`
	Invoice   string `json:"invoice" enums:"pass,fail,skip"`
}

type verdict struct {
	Status   VerificationStatus
	Reason   string
	Code     string
	Checks   Checks
	Expected *Expected
}

func passFail(ok bool) string {
	if ok {
		return CheckPass
	}
	return CheckFail
}

func newExpected(inv *invoiceView, expected asset) *Expected {
	if inv == nil {
		return nil
	}
	e := &Expected{
		Amount:           amount.StringFromInt64(inv.Stroops),
		AssetCode:        expected.Code,
		Recipient:        inv.Recipient,
		PaymentReference: inv.Reference,
	}
	if expected.Issuer != "" {
		e.AssetIssuer = &expected.Issuer
	}
	return e
}

func computeChecks(p chainPayment, inv *invoiceView, expected asset) Checks {
	c := Checks{
		Success:   passFail(p.Successful),
		Recipient: CheckSkip, Asset: CheckSkip, Amount: CheckSkip, Invoice: CheckSkip,
		Reference: CheckFail,
	}
	if inv == nil {
		return c
	}
	c.Reference = CheckPass
	c.Recipient = passFail(inv.Recipient != "" && p.To == inv.Recipient)
	c.Asset = passFail(p.Asset == expected)
	c.Amount = passFail(inv.Stroops > 0 && p.Stroops == inv.Stroops)
	switch {
	case !payableInvoiceStatuses[inv.Status]:
		c.Invoice = CheckFail
	case !inv.ExpiresAt.IsZero() && !p.ClosedAt.Before(inv.ExpiresAt):
		c.Invoice = CheckFail
	default:
		c.Invoice = CheckPass
	}
	return c
}

// evaluate is the single rule set deciding what an on-chain payment means for an invoice.
// inv is nil when no invoice carries the payment's memo. An already-settled invoice is judged
// a duplicate before anything else about the payment is questioned.
func evaluate(p chainPayment, inv *invoiceView, expected asset) verdict {
	v := verdict{Checks: computeChecks(p, inv, expected), Expected: newExpected(inv, expected)}
	reject := func(st VerificationStatus, code, reason string) verdict {
		v.Status, v.Code, v.Reason = st, code, reason
		return v
	}
	switch {
	case !p.Successful:
		return reject(StatusFailed, CodeTransactionFailed, "transaction failed on Stellar")
	case p.Memo == "":
		return reject(StatusUnmatched, CodeMissingReference, "payment has no text memo reference")
	case inv == nil:
		return reject(StatusUnmatched, CodeUnknownReference, "no invoice matches the payment reference")
	case inv.Status == invoicePaid:
		return reject(StatusInvalid, CodeDuplicate, "invoice is already paid")
	case inv.Recipient == "" || p.To != inv.Recipient:
		return reject(StatusUnmatched, CodeWrongRecipient, "payment recipient is not the invoice recipient")
	case !payableInvoiceStatuses[inv.Status]:
		return reject(StatusInvalid, CodeInvoiceNotPayable, fmt.Sprintf("invoice is %s and cannot accept payment", inv.Status))
	case !inv.ExpiresAt.IsZero() && !p.ClosedAt.Before(inv.ExpiresAt):
		return reject(StatusInvalid, CodeExpired, "invoice payment window has expired")
	case inv.Stroops <= 0:
		return reject(StatusInvalid, CodeInvalidInvoice, "invoice total is zero")
	case p.Asset != expected:
		return reject(StatusInvalid, CodeWrongAsset, fmt.Sprintf("expected asset %s, received %s", expected, p.Asset))
	case p.Stroops < inv.Stroops:
		return reject(StatusInvalid, CodeInsufficientAmount, fmt.Sprintf("insufficient amount: expected %s, received %s", amount.StringFromInt64(inv.Stroops), amount.StringFromInt64(p.Stroops)))
	case p.Stroops > inv.Stroops:
		return reject(StatusInvalid, CodeAmountMismatch, fmt.Sprintf("amount mismatch: expected %s, received %s", amount.StringFromInt64(inv.Stroops), amount.StringFromInt64(p.Stroops)))
	}
	v.Status = StatusVerified
	return v
}
