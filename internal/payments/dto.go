package payments

import (
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
)

type PaymentRecord struct {
	ID                 string     `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                                                                                  // Payment id
	InvoiceID          *string    `json:"invoice_id,omitempty" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                                                                // Invoice the payment was matched to; null for unmatched payments
	InvoiceNumber      *string    `json:"invoice_number" example:"INV-20261020-0019"`                                                                                         // Number of the matched invoice; null when unmatched
	TransactionHash    string     `json:"transaction_hash" example:"a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"`                                        // Stellar transaction hash
	ExplorerURL        string     `json:"explorer_url" example:"https://stellar.expert/explorer/testnet/tx/a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // Block explorer link
	Sender             string     `json:"sender" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"`                                                          // Paying account
	Recipient          string     `json:"recipient" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"`                                                       // Receiving account
	AssetCode          string     `json:"asset_code" example:"USDC"`
	AssetIssuer        *string    `json:"asset_issuer" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Issuer account; null for XLM
	Amount             string     `json:"amount" example:"2.0300000"`
	PaymentReference   *string    `json:"payment_reference" example:"STL-B9371BD3A173"`
	VerificationStatus string     `json:"verification_status" enums:"pending,detected,verified,invalid,unmatched,failed" example:"verified"`                                                                                                                                     // Outcome of the check; see docs/payment-status-model.md
	FailureCode        *string    `json:"failure_code" enums:"insufficient_amount,amount_mismatch,wrong_asset,wrong_recipient,missing_reference,unknown_reference,transaction_failed,duplicate,invoice_not_payable,expired,invalid_invoice_total" example:"insufficient_amount"` // Machine-readable reason; null when verified
	FailureReason      *string    `json:"failure_reason" example:"insufficient amount: expected 100.0000000, received 99.0000000"`                                                                                                                                               // Human-readable reason; null when verified
	Checks             *Checks    `json:"checks"`                                                                                                                                                                                                                                // Per-check result: pass, fail or skip
	Expected           *Expected  `json:"expected"`                                                                                                                                                                                                                              // What the invoice demanded; null when no invoice matched
	LedgerClosedAt     time.Time  `json:"ledger_closed_at" example:"2026-10-22T14:03:11Z"`                                                                                                                                                                                       // When Stellar closed the ledger containing the payment
	DetectedAt         time.Time  `json:"detected_at" example:"2026-10-22T14:03:25Z"`                                                                                                                                                                                            // When the server first saw the payment
	VerifiedAt         *time.Time `json:"verified_at" example:"2026-10-22T14:03:26Z"`                                                                                                                                                                                            // When the payment was verified; null otherwise
	MerchantEmail      *string    `json:"merchant_email,omitempty" example:"merchant@example.com"`                                                                                                                                                                               // Receiving merchant; only on /admin/payments
}

// PaymentRequest is what a payer needs to pay an invoice and what the owner needs to track it.
type PaymentRequest struct {
	InvoiceID          string          `json:"invoice_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                                                                                    // Invoice id
	InvoiceNumber      string          `json:"invoice_number" example:"INV-20261020-0019"`                                                                                                   // Invoice number
	InvoiceStatus      string          `json:"invoice_status" example:"sent"`                                                                                                                // Invoice status
	PaymentStatus      string          `json:"payment_status" example:"awaiting_payment"`                                                                                                    // Derived: awaiting_payment for sent, viewed, pending and overdue; otherwise the invoice status
	Amount             string          `json:"amount" example:"2.0300000"`                                                                                                                   // Exact amount to send, 7 decimals. Over- and underpayments are rejected
	AssetCode          string          `json:"asset_code" example:"USDC"`                                                                                                                    // Asset to send
	AssetIssuer        *string         `json:"asset_issuer" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"`                                                              // Issuer of the asset; null for XLM
	Recipient          string          `json:"recipient" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"`                                                                 // Account to pay
	PaymentReference   string          `json:"payment_reference" example:"STL-B9371BD3A173"`                                                                                                 // Text memo that must be attached to the payment
	Network            string          `json:"network" example:"testnet"`                                                                                                                    // Stellar network
	ExpiresAt          *time.Time      `json:"expires_at,omitempty" example:"2026-12-01T00:00:00Z"`                                                                                          // Payments closed at or after this time are rejected
	PaymentDetectedAt  *time.Time      `json:"payment_detected_at,omitempty" example:"2026-10-22T14:03:25Z"`                                                                                 // When a payment was first detected
	VerificationStatus *string         `json:"verification_status,omitempty" example:"verified"`                                                                                             // Status of the most relevant payment
	VerifiedAt         *time.Time      `json:"verified_at,omitempty" example:"2026-10-22T14:03:26Z"`                                                                                         // When it was verified
	PaidAt             *time.Time      `json:"paid_at,omitempty" example:"2026-10-22T14:03:26Z"`                                                                                             // When the invoice was marked paid
	TransactionHash    *string         `json:"transaction_hash,omitempty" example:"a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"`                                        // Hash of the verified payment; only set once verified
	ExplorerURL        *string         `json:"explorer_url,omitempty" example:"https://stellar.expert/explorer/testnet/tx/a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // Explorer link for the verified payment
	Payments           []PaymentRecord `json:"payments,omitempty"`                                                                                                                           // All payments recorded for the invoice; owner view only

	currency string
}

type VerifyTransactionDTO struct {
	TransactionHash string `json:"transaction_hash" validate:"required,len=64,hexadecimal" example:"a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // 64-character hex Stellar transaction hash
}

func PaymentStatusFor(invoiceStatus string) string {
	switch invoiceStatus {
	case "paid", "payment_detected", "invalid_payment", "expired", "cancelled", "draft", "refunded":
		return invoiceStatus
	default:
		return "awaiting_payment"
	}
}

type PageMeta = utils.PageMeta

type PaymentList struct {
	Items    []PaymentRecord `json:"items"`
	Payments []PaymentRecord `json:"payments"` // deprecated: use items
	Meta     PageMeta        `json:"meta"`
}
